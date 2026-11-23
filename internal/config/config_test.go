package config

import (
	"os"
	"testing"
)

func TestConfigLoadDefaultsAndOverrides(t *testing.T) {
	os.Unsetenv("CONFIG_PATH")
	os.Unsetenv("STORAGE_PATH")
	os.Unsetenv("LISTEN_ADDR")
	os.Unsetenv("OIDC_ADMIN_GROUP")

	cfg := Load()
	if cfg.ConfigPath != "/config" {
		t.Fatalf("expected /config, got %s", cfg.ConfigPath)
	}
	if cfg.StoragePath != "/data" {
		t.Fatalf("expected /data, got %s", cfg.StoragePath)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("expected :8080, got %s", cfg.ListenAddr)
	}
	if cfg.OIDCAdminGroup != "admin" {
		t.Fatalf("expected admin, got %s", cfg.OIDCAdminGroup)
	}
	if len(cfg.SessionSecret) != 32 {
		t.Fatalf("expected 32 byte random secret, got %d", len(cfg.SessionSecret))
	}
	if cfg.UsersConfigDir() != "/config/users" {
		t.Fatalf("expected /config/users, got %s", cfg.UsersConfigDir())
	}
	if cfg.UserStorageDir("alice") != "/data/users/alice" {
		t.Fatalf("expected /data/users/alice, got %s", cfg.UserStorageDir("alice"))
	}

	if cfg.DefaultQuotaBytes != 0 {
		t.Fatalf("expected default quota 0 (unlimited), got %d", cfg.DefaultQuotaBytes)
	}

	// Test overrides
	os.Setenv("CONFIG_PATH", "/custom/config")
	os.Setenv("STORAGE_PATH", "/custom/storage")
	os.Setenv("OIDC_ADMIN_GROUP", "superadmins")
	os.Setenv("SESSION_SECRET", "custom-secret-key-12345678901234")
	os.Setenv("DEFAULT_QUOTA", "5GB")

	cfg2 := Load()
	if cfg2.ConfigPath != "/custom/config" {
		t.Fatalf("expected /custom/config, got %s", cfg2.ConfigPath)
	}
	if cfg2.StoragePath != "/custom/storage" {
		t.Fatalf("expected /custom/storage, got %s", cfg2.StoragePath)
	}
	if cfg2.OIDCAdminGroup != "superadmins" {
		t.Fatalf("expected superadmins, got %s", cfg2.OIDCAdminGroup)
	}
	if string(cfg2.SessionSecret) != "custom-secret-key-12345678901234" {
		t.Fatalf("expected custom secret")
	}
	if cfg2.DefaultQuotaBytes != 5*1024*1024*1024 {
		t.Fatalf("expected 5GB quota bytes, got %d", cfg2.DefaultQuotaBytes)
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasErr   bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"unlimited", 0, false},
		{"NONE", 0, false},
		{"-1", 0, false},
		{"1024", 1024, false},
		{"500B", 500, false},
		{"10KB", 10 * 1024, false},
		{"10KiB", 10 * 1024, false},
		{"10K", 10 * 1024, false},
		{"256MB", 256 * 1024 * 1024, false},
		{"256MiB", 256 * 1024 * 1024, false},
		{"256M", 256 * 1024 * 1024, false},
		{"2GB", 2 * 1024 * 1024 * 1024, false},
		{"2GiB", 2 * 1024 * 1024 * 1024, false},
		{"2G", 2 * 1024 * 1024 * 1024, false},
		{"1.5GB", int64(1.5 * 1024 * 1024 * 1024), false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"invalid-size", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseBytes(tt.input)
		if (err != nil) != tt.hasErr {
			t.Errorf("ParseBytes(%q) error = %v, expected error: %v", tt.input, err, tt.hasErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("ParseBytes(%q) = %d, expected %d", tt.input, got, tt.expected)
		}
	}
}

func TestLoadVersion(t *testing.T) {
	if got := loadVersion("/nonexistent/version/file"); got != "dev" {
		t.Fatalf("expected default version 'dev', got %q", got)
	}

	tmp, err := os.CreateTemp(t.TempDir(), "version-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = tmp.Close()

	if got := loadVersion(tmp.Name()); got != "dev" {
		t.Fatalf("expected 'dev' for empty version file, got %q", got)
	}

	if err := os.WriteFile(tmp.Name(), []byte("v0.10.0\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := loadVersion(tmp.Name()); got != "v0.10.0" {
		t.Fatalf("expected 'v0.10.0', got %q", got)
	}
}
