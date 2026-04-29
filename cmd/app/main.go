package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/config"
	"github.com/guilherme-grimm/graph-go/internal/logging"
	"github.com/guilherme-grimm/graph-go/internal/server"
)

// Version is injected at build time via -ldflags "-X main.Version=..."
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		configPath  string
		logLevel    string
		logFormat   string
		healthCheck bool
	)

	cmd := &cobra.Command{
		Use:          "graph-go",
		Short:        "See your infrastructure — zero config.",
		Long:         "graph-go auto-discovers databases, caches, storage, and services, and serves a live topology graph.",
		Version:      Version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if healthCheck {
				return runHealthCheck()
			}
			return runServe(configPath, logLevel, logFormat)
		},
	}

	// Persistent: inherited by every subcommand so the surface stays uniform.
	cmd.PersistentFlags().StringVarP(&configPath, "config", "c", defaultConfigPath(), "path to config file (optional — auto-discovery by default)")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", envOr("LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	cmd.PersistentFlags().StringVar(&logFormat, "log-format", envOr("LOG_FORMAT", "console"), "log format: console|json")
	// Local: --health-check is a Docker HEALTHCHECK shim, not a user surface.
	cmd.Flags().BoolVar(&healthCheck, "health-check", false, "hit the local /health endpoint and exit 0/1 (for Docker HEALTHCHECK)")

	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newScanCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

func runServe(configPath, logLevel, logFormat string) error {
	logger, err := logging.New(logLevel, logFormat)
	if err != nil {
		return err
	}
	defer logger.Sync() //nolint:errcheck // stderr sync may fail on some TTYs

	logger.Infow("graph-go starting", "version", Version)

	cfg, err := config.Load(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Infow("no config file found — running in auto-discovery mode", "path", configPath)
		} else {
			logger.Warnw("failed to load config — running in auto-discovery mode", "path", configPath, "err", err)
		}
		cfg = &config.Config{}
	}

	applyServerEnv(&cfg.Server)

	srv, cleanup := server.NewServer(cfg, logger)
	done := make(chan bool, 1)

	go gracefulShutdown(srv, cleanup, logger, done)

	logger.Infow("http server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Errorw("http server error", "err", err)
		return err
	}

	<-done
	logger.Infow("graceful shutdown complete")
	return nil
}

func gracefulShutdown(apiServer *http.Server, cleanup func(), logger *zap.SugaredLogger, done chan<- bool) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	logger.Infow("shutdown requested; press Ctrl+C again to force")
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		logger.Warnw("server forced to shutdown with error", "err", err)
	}

	cleanup()
	done <- true
}

// runHealthCheck hits the local /health endpoint and exits with status 0 on
// 2xx, 1 otherwise. Used by the Docker HEALTHCHECK directive instead of curl.
func runHealthCheck() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/health", port)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check got status %d", resp.StatusCode)
	}
	return nil
}

func defaultConfigPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	return "conf/config.yaml"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// applyServerEnv lets env vars override YAML config. Only populates zero fields
// so an explicit YAML value always wins over env.
func applyServerEnv(s *config.ServerConfig) {
	if s.Port == 0 {
		if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil && p > 0 {
			s.Port = p
		}
	}
	if len(s.AllowedOrigins) == 0 {
		if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
			parts := strings.Split(v, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			s.AllowedOrigins = parts
		}
	}
}
