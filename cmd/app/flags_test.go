package main

import "testing"

// TestRootPersistentFlags asserts that --config, --log-level, and --log-format
// are declared on the root command's PersistentFlags so subcommands inherit
// them. --health-check stays local to root (Docker HEALTHCHECK shim only).
func TestRootPersistentFlags(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		wantShort string
		wantDef   string
	}{
		{name: "config",     flag: "config",     wantShort: "c", wantDef: "conf/config.yaml"},
		{name: "log-level",  flag: "log-level",  wantShort: "",  wantDef: "info"},
		{name: "log-format", flag: "log-format", wantShort: "",  wantDef: "console"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear envs so flag construction reflects pure defaults.
			t.Setenv("CONFIG_PATH", "")
			t.Setenv("LOG_LEVEL", "")
			t.Setenv("LOG_FORMAT", "")

			cmd := newRootCmd()
			f := cmd.PersistentFlags().Lookup(tt.flag)
			if f == nil {
				t.Fatalf("flag %q not found in PersistentFlags()", tt.flag)
			}
			if f.Shorthand != tt.wantShort {
				t.Errorf("flag %q shorthand = %q, want %q", tt.flag, f.Shorthand, tt.wantShort)
			}
			if f.DefValue != tt.wantDef {
				t.Errorf("flag %q default = %q, want %q", tt.flag, f.DefValue, tt.wantDef)
			}
		})
	}
}

// TestRootHealthCheckIsLocal asserts --health-check is a root-local flag, not
// persistent — it's a Docker HEALTHCHECK shim and should not pollute child
// command help output.
func TestRootHealthCheckIsLocal(t *testing.T) {
	cmd := newRootCmd()
	if f := cmd.PersistentFlags().Lookup("health-check"); f != nil {
		t.Error("--health-check should be a local flag, found in PersistentFlags()")
	}
	if f := cmd.LocalFlags().Lookup("health-check"); f == nil {
		t.Error("--health-check missing from LocalFlags()")
	}
}

// TestRootFlagEnvDefaults asserts env vars feed the persistent flag defaults.
func TestRootFlagEnvDefaults(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		flag    string
		wantDef string
	}{
		{name: "CONFIG_PATH feeds --config",  env: map[string]string{"CONFIG_PATH": "/env/x.yaml"},   flag: "config",     wantDef: "/env/x.yaml"},
		{name: "LOG_LEVEL feeds --log-level", env: map[string]string{"LOG_LEVEL": "debug"},           flag: "log-level",  wantDef: "debug"},
		{name: "LOG_FORMAT feeds --log-format", env: map[string]string{"LOG_FORMAT": "json"},         flag: "log-format", wantDef: "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cmd := newRootCmd()
			f := cmd.PersistentFlags().Lookup(tt.flag)
			if f == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if f.DefValue != tt.wantDef {
				t.Errorf("flag %q default = %q, want %q", tt.flag, f.DefValue, tt.wantDef)
			}
		})
	}
}
