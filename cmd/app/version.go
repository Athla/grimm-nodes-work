package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Commit and BuildDate are injected at build time via -ldflags.
// See Makefile's build-backend target.
var (
	Commit    = ""
	BuildDate = ""
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), versionString(Version, Commit, BuildDate))
		},
	}
}

func versionString(version, commit, date string) string {
	if version == "" {
		version = "(devel)"
	}
	if commit == "" {
		commit = "(unknown)"
	}
	if date == "" {
		date = "(unknown)"
	}
	return fmt.Sprintf("graph-go %s\ncommit: %s\nbuilt:  %s", version, commit, date)
}
