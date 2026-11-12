package config

import (
	"crypto/rand"
	"os"
	"strings"
)

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
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
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
	}
}

func (c *Config) UsersConfigDir() string {
	return c.ConfigPath + "/users"
}

func (c *Config) UserStorageDir(username string) string {
	return c.StoragePath + "/users/" + username
}
