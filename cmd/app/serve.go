package main

import (
	"github.com/spf13/cobra"
)

// newServeCmd returns the `serve` subcommand. The same RunE is also wired as
// the root's no-arg fallback in newRootCmd, so `./graph-go` and
// `./graph-go serve` produce identical behavior.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "serve",
		Short:        "Run the HTTP server with auto-discovery and live updates.",
		Long:         "serve starts the graph-go HTTP server on :8080 (or $PORT). It runs Docker and Kubernetes auto-discovery, applies any YAML-declared services, and streams real-time health updates over /websocket.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			logLevel, _ := cmd.Flags().GetString("log-level")
			logFormat, _ := cmd.Flags().GetString("log-format")
			return runServe(configPath, logLevel, logFormat)
		},
	}
}
