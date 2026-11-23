package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Version is the application version loaded from /version at startup (defaulting to "dev").
var Version = loadVersion("/version")

func loadVersion(path string) string {
	// nosemgrep: go.lang.security.audit.os-readfile-variable-path.os-readfile-variable-path -- path is internal constant "/version" or unit test temp file
	if b, err := os.ReadFile(path); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return v
		}
	}
	return "dev"
}

type Config struct {
	ConfigPath        string
	StoragePath       string
	ListenAddr        string
	OIDCIssuerURL     string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCRedirectURL   string
	OIDCScopes        []string
	OIDCAdminGroup    string
	OIDCUsernameClaim string
	SessionSecret     []byte
	DevMode           bool
	DefaultQuotaBytes int64
	WorkerDir         string
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

// ParseBytes parses a human-readable size string (e.g. "10GB", "500MB", "1.5GiB", "1048576").
// Returns 0 if empty, "0", or "unlimited".
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" || s == "0" || s == "UNLIMITED" || s == "NONE" || s == "-1" {
		return 0, nil
	}

	clean := s
	if strings.HasSuffix(clean, "IB") {
		clean = strings.TrimSuffix(clean, "IB")
	} else if strings.HasSuffix(clean, "B") && len(clean) > 1 && !strings.HasSuffix(clean, "KB") && !strings.HasSuffix(clean, "MB") && !strings.HasSuffix(clean, "GB") && !strings.HasSuffix(clean, "TB") {
		clean = strings.TrimSuffix(clean, "B")
	} else if strings.HasSuffix(clean, "B") {
		clean = strings.TrimSuffix(clean, "B")
	}

	var multiplier int64 = 1
	clean = strings.TrimSpace(clean)
	if strings.HasSuffix(clean, "K") {
		multiplier = 1024
		clean = strings.TrimSuffix(clean, "K")
	} else if strings.HasSuffix(clean, "M") {
		multiplier = 1024 * 1024
		clean = strings.TrimSuffix(clean, "M")
	} else if strings.HasSuffix(clean, "G") {
		multiplier = 1024 * 1024 * 1024
		clean = strings.TrimSuffix(clean, "G")
	} else if strings.HasSuffix(clean, "T") {
		multiplier = 1024 * 1024 * 1024 * 1024
		clean = strings.TrimSuffix(clean, "T")
	}

	clean = strings.TrimSpace(clean)
	val, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
	}
	if val <= 0 {
		return 0, nil
	}

	return int64(val * float64(multiplier)), nil
}

func Load() *Config {
	scopesStr := getEnv("OIDC_SCOPES", "openid profile email groups")
	scopes := strings.Fields(scopesStr)

	secretStr := os.Getenv("SESSION_SECRET")
	var sessionSecret []byte
	if secretStr != "" {
		sessionSecret = []byte(secretStr)
	} else {
		// Generate random 32-byte secret if not provided
		randomBytes := make([]byte, 32)
		_, _ = rand.Read(randomBytes)
		sessionSecret = randomBytes
	}

	devMode := strings.ToLower(os.Getenv("DEV_MODE")) == "true" || strings.ToLower(os.Getenv("DEV_MODE")) == "1"

	defaultQuotaStr := getEnv("DEFAULT_QUOTA", "")
	defaultQuotaBytes, _ := ParseBytes(defaultQuotaStr)

	return &Config{
		ConfigPath:        getEnv("CONFIG_PATH", "/config"),
		StoragePath:       getEnv("STORAGE_PATH", "/data"),
		ListenAddr:        getEnv("LISTEN_ADDR", ":8080"),
		OIDCIssuerURL:     getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:      getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:  getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURL:   getEnv("OIDC_REDIRECT_URL", ""),
		OIDCScopes:        scopes,
		OIDCAdminGroup:    getEnv("OIDC_ADMIN_GROUP", "admin"),
		OIDCUsernameClaim: getEnv("OIDC_USERNAME_CLAIM", "preferred_username"),
		SessionSecret:     sessionSecret,
		DevMode:           devMode,
		DefaultQuotaBytes: defaultQuotaBytes,
		WorkerDir:         getEnv("WORKER_DIR", "/tmp/andvari-workers"),
	}
}

func (c *Config) UsersConfigDir() string {
	return c.ConfigPath + "/users"
}

func (c *Config) UserStorageDir(username string) string {
	return c.StoragePath + "/users/" + username
}
