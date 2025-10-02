package sync

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/woodleighschool/JCDSSync/internal/jamf"
)

// FileCache represents the cached JCDS state
type FileCache struct {
	Timestamp time.Time               `json:"timestamp"`
	Files     map[string]jamf.Package `json:"files"` // filename -> Package
}

// Service handles the sync logic
type Service struct {
	jamfClient  jamf.Client
	localFolder string
	logger      *slog.Logger
	cacheFile   string
}

// NewService creates a new sync service
func NewService(jamfClient jamf.Client, localFolder string, logger *slog.Logger) *Service {
	cacheFile := filepath.Join(localFolder, ".jcdssync_cache.json")
	return &Service{
		jamfClient:  jamfClient,
		localFolder: localFolder,
		logger:      logger,
		cacheFile:   cacheFile,
	}
}

// Sync performs the complete sync process
func (s *Service) Sync() error {
	s.logger.Info("Starting sync process")

	if err := os.MkdirAll(s.localFolder, 0755); err != nil {
		return fmt.Errorf("failed to create local folder %s: %w", s.localFolder, err)
	}

	previousCache, err := s.loadCache()
	if err != nil {
		s.logger.Debug("Failed to load previous cache", "error", err)
		previousCache = &FileCache{Files: make(map[string]jamf.Package)}
	}

	files, err := s.jamfClient.GetPackages()
	if err != nil {
		return fmt.Errorf("failed to fetch files: %w", err)
	}

	s.logger.Info("Retrieved files from JCDS", "count", len(files))

	currentCache := &FileCache{
		Timestamp: time.Now(),
		Files:     make(map[string]jamf.Package),
	}
	for _, file := range files {
		currentCache.Files[file.FileName] = file
	}

	var downloadsNeeded []jamf.DownloadRequest
	for _, file := range files {
		needsDownload, reason := s.fileNeedsDownload(file, previousCache)
		if needsDownload {
			downloadsNeeded = append(downloadsNeeded, jamf.DownloadRequest{
				FileName:  file.FileName,
				LocalPath: filepath.Join(s.localFolder, file.FileName),
			})
			s.logger.Info("File queued for download", "filename", file.FileName, "reason", reason)
		} else {
			s.logger.Debug("File up to date", "filename", file.FileName)
		}
	}

	if len(downloadsNeeded) > 0 {
		s.logger.Info("Starting batch download", "files_to_download", len(downloadsNeeded))
		if err := s.jamfClient.DownloadPackages(downloadsNeeded); err != nil {
			s.logger.Error("Batch download failed, falling back to individual downloads", "error", err)
			for _, download := range downloadsNeeded {
				if err := s.jamfClient.DownloadPackage(download.FileName, download.LocalPath); err != nil {
					s.logger.Error("Failed to download file", "filename", download.FileName, "error", err)
				} else {
					s.logger.Info("Successfully downloaded file", "filename", download.FileName)
				}
			}
		} else {
			s.logger.Info("Batch download completed successfully", "files_downloaded", len(downloadsNeeded))
		}
	}

	if err := s.cleanupWithCache(currentCache.Files, previousCache.Files); err != nil {
		s.logger.Warn("Failed to clean up some outdated files", "error", err)
	}

	if err := s.saveCache(currentCache); err != nil {
		s.logger.Warn("Failed to save cache", "error", err)
	}

	s.logger.Info("Sync complete")
	return nil
}

// fileNeedsDownload determines if a file needs to be downloaded and why
func (s *Service) fileNeedsDownload(file jamf.Package, previousCache *FileCache) (bool, string) {
	localFilePath := filepath.Join(s.localFolder, file.FileName)

	_, err := os.Stat(localFilePath)
	fileExists := err == nil

	if previousFile, exists := previousCache.Files[file.FileName]; exists {
		if previousFile.MD5 == file.MD5 {
			if fileExists {
				return false, "unchanged and file exists"
			} else {
				return true, "unchanged but local file missing"
			}
		} else {
			return true, fmt.Sprintf("updated in JCDS (old: %s, new: %s)", previousFile.MD5, file.MD5)
		}
	} else {
		if fileExists {
			return true, "new file, local file will be replaced"
		} else {
			return true, "new file"
		}
	}
}

// cleanupWithCache removes local files using cache-based comparison
func (s *Service) cleanupWithCache(currentFiles, previousFiles map[string]jamf.Package) error {
	entries, err := os.ReadDir(s.localFolder)
	if err != nil {
		return fmt.Errorf("failed to read local folder %s: %w", s.localFolder, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		_, existsInCurrent := currentFiles[entry.Name()]
		_, existedInPrevious := previousFiles[entry.Name()]

		if !existsInCurrent {
			if existedInPrevious {
				filePath := filepath.Join(s.localFolder, entry.Name())
				s.logger.Info("Deleting file removed from JCDS", "filename", entry.Name())

				if err := os.Remove(filePath); err != nil {
					s.logger.Error("Failed to delete removed file",
						"filename", entry.Name(), "error", err)
				}
			} else {
				s.logger.Warn("Found local file not managed by JCDSSync, leaving untouched", "filename", entry.Name())
			}
		}
	}

	return nil
}

// loadCache loads the previous file cache from disk
func (s *Service) loadCache() (*FileCache, error) {
	data, err := os.ReadFile(s.cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cache FileCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache: %w", err)
	}

	s.logger.Debug("Loaded cache", "files", len(cache.Files), "timestamp", cache.Timestamp)
	return &cache, nil
}

// saveCache saves the current file cache to disk
func (s *Service) saveCache(cache *FileCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(s.cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	s.logger.Debug("Saved cache", "files", len(cache.Files), "file", s.cacheFile)
	return nil
}
