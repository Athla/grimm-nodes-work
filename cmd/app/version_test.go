package main

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		date    string
		want    string
	}{
		{
			name:    "all fields set",
			version: "v0.1.0",
			commit:  "abc1234",
			date:    "2026-04-29",
			want:    "graph-go v0.1.0\ncommit: abc1234\nbuilt:  2026-04-29",
		},
		{
			name:    "missing commit",
			version: "v0.1.0",
			commit:  "",
			date:    "2026-04-29",
			want:    "graph-go v0.1.0\ncommit: (unknown)\nbuilt:  2026-04-29",
		},
		{
			name:    "all empty falls back to devel",
			version: "",
			commit:  "",
			date:    "",
			want:    "graph-go (devel)\ncommit: (unknown)\nbuilt:  (unknown)",
		},
		{
			name:    "only version set",
			version: "v0.2.0",
			commit:  "",
			date:    "",
			want:    "graph-go v0.2.0\ncommit: (unknown)\nbuilt:  (unknown)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionString(tt.version, tt.commit, tt.date)
			if got != tt.want {
				t.Errorf("versionString(%q, %q, %q) =\n%q\nwant:\n%q", tt.version, tt.commit, tt.date, got, tt.want)
			}
		})
	}
}

func TestVersionSubcommandRegistered(t *testing.T) {
	root := newRootCmd()
	if findSubcommand(root, "version") == nil {
		t.Fatal("version subcommand not registered on root")
	}
}

func TestVersionCommandOutput(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInOut []string
	}{
		{
			name:      "version cmd prints all three lines",
			args:      []string{"version"},
			wantInOut: []string{"graph-go ", "commit:", "built:"},
		},
		{
			name:      "root --help lists version",
			args:      []string{"--help"},
			wantInOut: []string{"version"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := executeCommand(newRootCmd(), tt.args...)
			if err != nil {
				t.Fatalf("executeCommand: %v", err)
			}
			for _, want := range tt.wantInOut {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, out)
				}
			}
		})
	}
}
