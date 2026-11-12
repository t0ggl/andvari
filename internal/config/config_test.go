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

	// Test overrides
	os.Setenv("CONFIG_PATH", "/custom/config")
	os.Setenv("STORAGE_PATH", "/custom/storage")
	os.Setenv("OIDC_ADMIN_GROUP", "superadmins")
	os.Setenv("SESSION_SECRET", "custom-secret-key-12345678901234")

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
}
