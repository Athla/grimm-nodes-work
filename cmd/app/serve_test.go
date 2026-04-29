package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func findSubcommand(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestServeSubcommandRegistered(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		wantInStr string
	}{
		{name: "Use is 'serve'",        field: "use",   wantInStr: "serve"},
		{name: "Short mentions HTTP",   field: "short", wantInStr: "HTTP"},
	}
	root := newRootCmd()
	cmd := findSubcommand(root, "serve")
	if cmd == nil {
		t.Fatal("serve subcommand not registered on root")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			switch tt.field {
			case "use":
				got = cmd.Use
			case "short":
				got = cmd.Short
			}
			if !strings.Contains(got, tt.wantInStr) {
				t.Errorf("serve.%s = %q, expected to contain %q", tt.field, got, tt.wantInStr)
			}
		})
	}
}

func TestServeInheritsPersistentFlags(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "config",     flag: "config"},
		{name: "log-level",  flag: "log-level"},
		{name: "log-format", flag: "log-format"},
	}
	root := newRootCmd()
	serveCmd := findSubcommand(root, "serve")
	if serveCmd == nil {
		t.Fatal("serve subcommand not registered")
	}
	// InheritedFlags() requires the parent chain to be set, which AddCommand does.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if serveCmd.InheritedFlags().Lookup(tt.flag) == nil {
				t.Errorf("serve does not inherit --%s", tt.flag)
			}
		})
	}
}

// TestRootHasNoArgFallback asserts that ./graph-go with no subcommand still
// has a RunE wired (so it doesn't just print help). This preserves the
// pre-CLI-milestone behavior where the binary boots the server by default.
func TestRootHasNoArgFallback(t *testing.T) {
	root := newRootCmd()
	if root.RunE == nil && root.Run == nil {
		t.Fatal("root has no RunE/Run — ./graph-go with no args would print help instead of running serve")
	}
}

// TestServeHelpOutput exercises --help via the cobra runtime, asserting the
// help text mentions serve-relevant content. --help short-circuits before RunE
// fires, so this is safe (no HTTP server actually starts).
func TestServeHelpOutput(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInOut string
	}{
		{name: "serve --help mentions HTTP", args: []string{"serve", "--help"}, wantInOut: "HTTP"},
		{name: "root --help lists serve",    args: []string{"--help"},          wantInOut: "serve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := executeCommand(newRootCmd(), tt.args...)
			if err != nil {
				t.Fatalf("executeCommand: %v", err)
			}
			if !strings.Contains(out, tt.wantInOut) {
				t.Errorf("output missing %q\nfull output:\n%s", tt.wantInOut, out)
			}
		})
	}
}
