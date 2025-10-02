package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/woodleighschool/JCDSSync/internal/config"
	"github.com/woodleighschool/JCDSSync/internal/jamf"
	"github.com/woodleighschool/JCDSSync/internal/sync"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		slog.Error("Command execution failed", "error", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jcdssync",
		Short: "JCDS Synchronisation Tool",
		Long:  "JCDSSync syncs JCDS files to a local path.",

		SilenceUsage:  true, // Don't show usage on errors
		SilenceErrors: true, // We handle errors manually
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync()
		},
	}

	// Setup Viper for automatic env binding
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	flags := cmd.Flags()
	flags.String("schedule", "", "cron schedule expression for automatic sync (e.g., '0 2 * * *' for daily at 2 AM)")
	flags.String("local-folder", "/packages", "local directory path where JCDS files will be stored")
	flags.String("log-level", "info", "logging level: debug, info, warn, error")
	flags.String("instance-domain", "", "Jamf Pro instance domain (can also use INSTANCE_DOMAIN env var)")
	flags.String("client-id", "", "OAuth2 client ID (can also use CLIENT_ID env var)")
	flags.String("client-secret", "", "OAuth2 client secret (can also use CLIENT_SECRET env var)")

	if err := viper.BindPFlag("instance_domain", flags.Lookup("instance-domain")); err != nil {
		panic(fmt.Sprintf("failed to bind instance-domain flag: %v", err))
	}
	if err := viper.BindPFlag("client_id", flags.Lookup("client-id")); err != nil {
		panic(fmt.Sprintf("failed to bind client-id flag: %v", err))
	}
	if err := viper.BindPFlag("client_secret", flags.Lookup("client-secret")); err != nil {
		panic(fmt.Sprintf("failed to bind client-secret flag: %v", err))
	}
	if err := viper.BindPFlag("local_folder", flags.Lookup("local-folder")); err != nil {
		panic(fmt.Sprintf("failed to bind local-folder flag: %v", err))
	}
	if err := viper.BindPFlag("sync_schedule", flags.Lookup("schedule")); err != nil {
		panic(fmt.Sprintf("failed to bind schedule flag: %v", err))
	}
	if err := viper.BindPFlag("log_level", flags.Lookup("log-level")); err != nil {
		panic(fmt.Sprintf("failed to bind log-level flag: %v", err))
	}

	cmd.AddCommand(newVersionCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("jcdssync %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", date)
		},
	}
}

func runSync() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger := setupLogging(cfg)

	logger.Info("JCDSSync starting",
		"version", version,
		"log_level", cfg.LogLevel,
		"local_folder", cfg.LocalFolder,
		"oneshot_mode", cfg.IsOneshot())

	jamfClient, err := jamf.NewClient(cfg, cfg.LogLevel, logger)
	if err != nil {
		return fmt.Errorf("failed to create Jamf client: %w", err)
	}
	defer func() {
		if closeErr := jamfClient.Close(); closeErr != nil {
			logger.Warn("failed to close Jamf client", "error", closeErr)
		}
	}()

	syncService := sync.NewService(jamfClient, cfg.LocalFolder, logger)

	if cfg.IsOneshot() {
		logger.Info("Running sync once (oneshot mode)")
		return syncService.Sync()
	}

	logger.Info("Setting up scheduled sync", "schedule", cfg.SyncSchedule)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	c := cron.New()

	_, err = c.AddFunc(cfg.SyncSchedule, func() {
		logger.Info("Starting scheduled sync")
		if err := syncService.Sync(); err != nil {
			logger.Error("Scheduled sync failed", "error", err)
		} else {
			logger.Info("Scheduled sync completed successfully")
		}
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	c.Start()
	defer c.Stop()

	logger.Info("Scheduler started, waiting for signals...")

	<-ctx.Done()
	logger.Info("Shutdown signal received, stopping...")

	return nil
}

func setupLogging(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: cfg.GetLogLevel(),
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	slog.SetDefault(logger)

	return logger
}
