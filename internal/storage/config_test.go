package storage

import "testing"

func TestConfigFromEnvOverridePath(t *testing.T) {
	t.Setenv("GIDDYUP_DB_PATH", "/tmp/giddyup-custom.db")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv() unexpected error: %v", err)
	}
	if cfg.Mode != ModeSecure {
		t.Fatalf("cfg.Mode = %q, want %q", cfg.Mode, ModeSecure)
	}
	if cfg.Path != "/tmp/giddyup-custom.db" {
		t.Fatalf("cfg.Path = %q, want %q", cfg.Path, "/tmp/giddyup-custom.db")
	}
}
