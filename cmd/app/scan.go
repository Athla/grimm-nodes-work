package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilherme-grimm/graph-go/internal/config"
	"github.com/guilherme-grimm/graph-go/internal/logging"
	"github.com/guilherme-grimm/graph-go/internal/server"
)

func newScanCmd() *cobra.Command {
	var (
		format     string
		withHealth bool
		pretty     bool
	)

	cmd := &cobra.Command{
		Use:          "scan",
		Short:        "Run discovery once and emit the graph to stdout.",
		Long:         "scan runs one discovery pass against Docker, Kubernetes, and any YAML-declared services, then writes the discovered graph to stdout and exits. Logs go to stderr — combine with --log-level=warn (or LOG_LEVEL=warn) for quieter stderr when piping.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			logLevel, _ := cmd.Flags().GetString("log-level")
			logFormat, _ := cmd.Flags().GetString("log-format")
			return runScan(configPath, logLevel, logFormat, format, withHealth, pretty)
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "output format (supported: json)")
	cmd.Flags().BoolVar(&withHealth, "health", false, "run one health sweep and merge adapter status onto nodes")
	cmd.Flags().BoolVar(&pretty, "pretty", false, "indent output for humans (default: compact)")

	return cmd
}

func runScan(configPath, logLevel, logFormat, format string, withHealth, pretty bool) error {
	if format != "json" {
		return fmt.Errorf("unsupported format %q (supported: json)", format)
	}

	logger, err := logging.New(logLevel, logFormat)
	if err != nil {
		return err
	}
	defer logger.Sync() //nolint:errcheck // stderr sync may fail on some TTYs

	cfg, err := config.Load(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debugw("no config file — auto-discovery mode", "path", configPath)
		} else {
			logger.Warnw("config load failed — auto-discovery mode", "path", configPath, "err", err)
		}
		cfg = &config.Config{}
	}

	reg, _, cleanup := server.BuildRegistry(cfg, logger)
	defer cleanup()

	if err := server.WriteGraphJSON(os.Stdout, reg, withHealth, pretty); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	return nil
}
