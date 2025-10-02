package jamf

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
	"github.com/woodleighschool/JCDSSync/internal/config"
)

// Package represents a Jamf file with metadata
type Package struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FileName string `json:"fileName"`
	MD5      string `json:"md5"`
}

// Client interface abstracts Jamf Pro operations
type Client interface {
	GetPackages() ([]Package, error)
	DownloadPackage(fileName, localPath string) error
	DownloadPackages(downloads []DownloadRequest) error
	Close() error
}

// DownloadRequest represents a file download request
type DownloadRequest struct {
	FileName  string
	LocalPath string
}

// jamfClient implements the Client interface using the Jamf Pro SDK
type jamfClient struct {
	client *jamfpro.Client
	logger *slog.Logger
}

// NewClient creates a new Jamf client using the provided configuration
func NewClient(cfg *config.Config, logLevel string, logger *slog.Logger) (Client, error) {
	if err := os.Setenv("INSTANCE_DOMAIN", cfg.InstanceDomain); err != nil {
		return nil, fmt.Errorf("failed to set INSTANCE_DOMAIN environment variable: %w", err)
	}
	if err := os.Setenv("CLIENT_ID", cfg.ClientID); err != nil {
		return nil, fmt.Errorf("failed to set CLIENT_ID environment variable: %w", err)
	}
	if err := os.Setenv("CLIENT_SECRET", cfg.ClientSecret); err != nil {
		return nil, fmt.Errorf("failed to set CLIENT_SECRET environment variable: %w", err)
	}
	if err := os.Setenv("AUTH_METHOD", cfg.AuthMethod); err != nil {
		return nil, fmt.Errorf("failed to set AUTH_METHOD environment variable: %w", err)
	}
	if err := os.Setenv("TOKEN_REFRESH_BUFFER_PERIOD_SECONDS", cfg.TokenRefreshBufferPeriod); err != nil {
		return nil, fmt.Errorf("failed to set TOKEN_REFRESH_BUFFER_PERIOD_SECONDS environment variable: %w", err)
	}
	if err := os.Setenv("TOKEN_BUFFER_PERIOD_SECONDS", cfg.TokenBufferPeriod); err != nil {
		return nil, fmt.Errorf("failed to set TOKEN_BUFFER_PERIOD_SECONDS environment variable: %w", err)
	}

	// Set log level to fatal to shut up SDK
	if err := os.Setenv("LOG_LEVEL", "fatal"); err != nil {
		return nil, fmt.Errorf("failed to set LOG_LEVEL environment variable: %w", err)
	}

	client, err := jamfpro.BuildClientWithEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to initialise Jamf Pro client: %w", err)
	}

	return &jamfClient{
		client: client,
		logger: logger,
	}, nil
}

// GetPackages retrieves all files from JCDS
func (j *jamfClient) GetPackages() ([]Package, error) {
	j.logger.Debug("Calling JCDS2 packages API")

	resp, err := j.client.GetJCDS2Packages()
	if err != nil {
		j.logger.Error("JCDS2 packages API call failed", "error", err)
		return nil, fmt.Errorf("failed to fetch files: %w", err)
	}

	j.logger.Debug("JCDS2 API response received", "package_count", len(resp))

	files := make([]Package, len(resp))
	for i, file := range resp {
		files[i] = Package{
			ID:       0, // The JCDS2 response doesn't include ID
			Name:     file.FileName,
			FileName: file.FileName,
			MD5:      file.MD5,
		}
	}

	return files, nil
}

// DownloadPackage downloads a file from JCDS
func (j *jamfClient) DownloadPackage(fileName, localPath string) error {
	j.logger.Debug("Getting download URI", "filename", fileName)

	downloadResp, err := j.client.GetJCDS2PackageURIByName(fileName)
	if err != nil {
		j.logger.Error("Failed to get download URI", "filename", fileName, "error", err)
		return fmt.Errorf("failed to get download URI for %s: %w", fileName, err)
	}

	j.logger.Debug("Download URI obtained", "filename", fileName, "uri", downloadResp.URI)

	httpClient := &http.Client{
		Timeout: 10 * time.Minute,
	}

	j.logger.Debug("Making HTTP request", "filename", fileName)
	resp, err := httpClient.Get(downloadResp.URI)
	if err != nil {
		j.logger.Error("HTTP request failed", "filename", fileName, "error", err)
		return fmt.Errorf("failed to download %s: %w", fileName, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			j.logger.Warn("failed to close response body", "filename", fileName, "error", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		j.logger.Error("HTTP request returned error status", "filename", fileName, "status_code", resp.StatusCode, "status", resp.Status)
		return fmt.Errorf("download failed for %s with status %d", fileName, resp.StatusCode)
	}

	j.logger.Debug("Creating local file", "filename", fileName, "local_path", localPath)
	out, err := os.Create(localPath)
	if err != nil {
		j.logger.Error("Failed to create local file", "filename", fileName, "local_path", localPath, "error", err)
		return fmt.Errorf("failed to create local file %s: %w", localPath, err)
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			j.logger.Warn("failed to close file", "filename", fileName, "error", closeErr)
		}
	}()

	j.logger.Debug("Writing file data", "filename", fileName)
	bytesWritten, err := io.Copy(out, resp.Body)
	if err != nil {
		j.logger.Error("Failed to write file data", "filename", fileName, "local_path", localPath, "error", err)
		return fmt.Errorf("failed to write file %s: %w", localPath, err)
	}

	j.logger.Debug("File write completed", "filename", fileName, "bytes_written", bytesWritten)
	return nil
}

// DownloadPackages downloads multiple files efficiently by reusing HTTP connections
func (j *jamfClient) DownloadPackages(downloads []DownloadRequest) error {
	if len(downloads) == 0 {
		j.logger.Debug("No downloads in batch request")
		return nil
	}

	j.logger.Debug("Initialising batch download", "count", len(downloads))

	httpClient := &http.Client{
		Timeout: 10 * time.Minute,
	}

	successCount := 0
	for i, download := range downloads {
		j.logger.Debug("Processing batch download", "filename", download.FileName, "progress", fmt.Sprintf("%d/%d", i+1, len(downloads)))

		if err := j.downloadPackageWithClient(httpClient, download.FileName, download.LocalPath); err != nil {
			j.logger.Error("Batch download item failed", "filename", download.FileName, "progress", fmt.Sprintf("%d/%d", i+1, len(downloads)), "error", err)
			return fmt.Errorf("failed to download %s: %w", download.FileName, err)
		}

		successCount++
		j.logger.Debug("Batch download item completed", "filename", download.FileName, "progress", fmt.Sprintf("%d/%d", i+1, len(downloads)))
	}

	j.logger.Debug("Batch download operation finished", "total_files", len(downloads), "successful_downloads", successCount)
	return nil
}

// downloadPackageWithClient downloads a file using the provided HTTP client
func (j *jamfClient) downloadPackageWithClient(httpClient *http.Client, fileName, localPath string) error {
	j.logger.Debug("Getting download URI for batch item", "filename", fileName)

	downloadResp, err := j.client.GetJCDS2PackageURIByName(fileName)
	if err != nil {
		j.logger.Error("Failed to get download URI for batch item", "filename", fileName, "error", err)
		return fmt.Errorf("failed to get download URI for %s: %w", fileName, err)
	}

	j.logger.Debug("Making HTTP request for batch item", "filename", fileName, "uri", downloadResp.URI)

	resp, err := httpClient.Get(downloadResp.URI)
	if err != nil {
		j.logger.Error("HTTP request failed for batch item", "filename", fileName, "error", err)
		return fmt.Errorf("failed to download %s: %w", fileName, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			j.logger.Warn("failed to close response body for batch item", "filename", fileName, "error", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		j.logger.Error("HTTP request returned error status for batch item", "filename", fileName, "status_code", resp.StatusCode, "status", resp.Status)
		return fmt.Errorf("download failed for %s with status %d", fileName, resp.StatusCode)
	}

	j.logger.Debug("Creating local file for batch item", "filename", fileName, "local_path", localPath)
	// Create the file
	out, err := os.Create(localPath)
	if err != nil {
		j.logger.Error("Failed to create local file for batch item", "filename", fileName, "local_path", localPath, "error", err)
		return fmt.Errorf("failed to create local file %s: %w", localPath, err)
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			j.logger.Warn("failed to close file for batch item", "filename", fileName, "error", closeErr)
		}
	}()

	j.logger.Debug("Writing file data for batch item", "filename", fileName)
	bytesWritten, err := io.Copy(out, resp.Body)
	if err != nil {
		j.logger.Error("Failed to write file data for batch item", "filename", fileName, "local_path", localPath, "error", err)
		return fmt.Errorf("failed to write file %s: %w", localPath, err)
	}

	j.logger.Debug("Batch item write completed", "filename", fileName, "bytes_written", bytesWritten)
	return nil
}

func (j *jamfClient) Close() error {
	return nil
}
