package main

import (
	"strings"
	"testing"
)

func TestScanSubcommandRegistered(t *testing.T) {
	root := newRootCmd()
	if findSubcommand(root, "scan") == nil {
		t.Fatal("scan subcommand not registered on root")
	}
}

func TestScanInheritsPersistentFlags(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "config",     flag: "config"},
		{name: "log-level",  flag: "log-level"},
		{name: "log-format", flag: "log-format"},
	}
	root := newRootCmd()
	scan := findSubcommand(root, "scan")
	if scan == nil {
		t.Fatal("scan subcommand not registered")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if scan.InheritedFlags().Lookup(tt.flag) == nil {
				t.Errorf("scan does not inherit --%s", tt.flag)
			}
		})
	}
}

// TestScanDoesNotRedeclareInheritedFlags catches the easy regression where a
// future contributor re-adds --config or --log-level locally on scan and
// shadows the root persistent flags.
func TestScanDoesNotRedeclareInheritedFlags(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "config not local",     flag: "config"},
		{name: "log-level not local",  flag: "log-level"},
		{name: "log-format not local", flag: "log-format"},
	}
	root := newRootCmd()
	scan := findSubcommand(root, "scan")
	if scan == nil {
		t.Fatal("scan subcommand not registered")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if f := scan.LocalFlags().Lookup(tt.flag); f != nil {
				t.Errorf("scan redeclared --%s as local flag (should inherit from root)", tt.flag)
			}
		})
	}
}

func TestScanLocalFlags(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		wantDef   string
		wantUsage string
	}{
		{name: "format",  flag: "format",  wantDef: "json",  wantUsage: "json"},
		{name: "pretty",  flag: "pretty",  wantDef: "false", wantUsage: "indent"},
		{name: "health",  flag: "health",  wantDef: "false", wantUsage: "health"},
	}
	root := newRootCmd()
	scan := findSubcommand(root, "scan")
	if scan == nil {
		t.Fatal("scan subcommand not registered")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := scan.LocalFlags().Lookup(tt.flag)
			if f == nil {
				t.Fatalf("scan missing local flag --%s", tt.flag)
			}
			if f.DefValue != tt.wantDef {
				t.Errorf("--%s default = %q, want %q", tt.flag, f.DefValue, tt.wantDef)
			}
			if !strings.Contains(strings.ToLower(f.Usage), strings.ToLower(tt.wantUsage)) {
				t.Errorf("--%s usage missing %q: %q", tt.flag, tt.wantUsage, f.Usage)
			}
		})
	}
}

func TestScanRejectsUnsupportedFormat(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErrSub string
	}{
		{name: "yaml format rejected", args: []string{"scan", "--format=yaml"}, wantErrSub: "unsupported format"},
		{name: "xml format rejected",  args: []string{"scan", "--format=xml"},  wantErrSub: "unsupported format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeCommand(newRootCmd(), tt.args...)
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tt.args)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
			}
		})
	}
}

func TestScanHelpOutput(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInOut string
	}{
		{name: "scan --help shows --format",  args: []string{"scan", "--help"}, wantInOut: "--format"},
		{name: "scan --help shows --pretty",  args: []string{"scan", "--help"}, wantInOut: "--pretty"},
		{name: "scan --help shows --health",  args: []string{"scan", "--help"}, wantInOut: "--health"},
		{name: "root --help lists scan cmd",  args: []string{"--help"},          wantInOut: "scan "},
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
