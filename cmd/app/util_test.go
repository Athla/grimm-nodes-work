package main

import "testing"

func TestDefaultConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{name: "unset env falls back to default", envValue: "", want: "conf/config.yaml"},
		{name: "set env wins", envValue: "/etc/graph-go/foo.yaml", want: "/etc/graph-go/foo.yaml"},
		{name: "empty env falls back", envValue: "", want: "conf/config.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CONFIG_PATH", tt.envValue)
			if got := defaultConfigPath(); got != tt.want {
				t.Errorf("defaultConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnvOr(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue string
		fallback string
		want     string
	}{
		{name: "unset returns fallback", key: "GRAPHGO_TEST_ENVOR", envValue: "", fallback: "fallback", want: "fallback"},
		{name: "set returns env value", key: "GRAPHGO_TEST_ENVOR", envValue: "from-env", fallback: "fallback", want: "from-env"},
		{name: "empty env returns fallback", key: "GRAPHGO_TEST_ENVOR", envValue: "", fallback: "fallback", want: "fallback"},
		{name: "whitespace value is preserved", key: "GRAPHGO_TEST_ENVOR", envValue: " spaced ", fallback: "fallback", want: " spaced "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			if got := envOr(tt.key, tt.fallback); got != tt.want {
				t.Errorf("envOr(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}
