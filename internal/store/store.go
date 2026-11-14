package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/t0ggl/andvari/internal/auth"
)

const MaxTokensPerUser = 20

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrMaxTokensReached = errors.New("maximum of 20 active tokens allowed; revoke an existing token to create a new one")
	ErrTokenNotFound    = errors.New("token not found")
	ErrInvalidUsername  = errors.New("invalid username")
	ErrEmptyTokenLabel  = errors.New("token label cannot be empty")
)

type TokenInfo struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Hash       string     `json:"hash"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type UserPreferences struct {
	FirstLoginCompleted bool `json:"first_login_completed"`
}

type UserMeta struct {
	Username    string          `json:"username"`
	CreatedAt   time.Time       `json:"created_at"`
	Tokens      []TokenInfo     `json:"tokens"`
	Preferences UserPreferences `json:"preferences"`
	QuotaBytes  *int64          `json:"quota_bytes,omitempty"`
}

type Store struct {
	configDir         string
	storageDir        string
	defaultQuotaBytes int64
	startupSalt       []byte
	mu                sync.RWMutex

	// In-memory cache for user metadata: username -> cachedMeta
	userCache map[string]*cachedMeta
	// In-memory token verification cache: hmac(startupSalt, username+":"+token) -> tokenCacheEntry
	tokenCache map[string]*tokenCacheEntry
	// In-memory cache for storage usage: username -> cachedUsage
	usageCache map[string]*cachedUsage
}

type cachedMeta struct {
	modTime time.Time
	meta    *UserMeta
}

type cachedUsage struct {
	bytes int64
}

type tokenCacheEntry struct {
	tokenID    string
	username   string
	validUntil time.Time
}

func NewStore(configDir, storageDir string, defaultQuotaBytes ...int64) (*Store, error) {
	usersConfigDir := filepath.Join(configDir, "users")
	if err := os.MkdirAll(usersConfigDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create config dir %s: %w", usersConfigDir, err)
	}

	usersStorageDir := filepath.Join(storageDir, "users")
	if err := os.MkdirAll(usersStorageDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create storage dir %s: %w", usersStorageDir, err)
	}

	// Generate random 32-byte salt on application startup for fast HMAC token caching
	startupSalt := make([]byte, 32)
	if _, err := rand.Read(startupSalt); err != nil {
		return nil, fmt.Errorf("failed to generate random startup salt: %w", err)
	}

	var quota int64
	if len(defaultQuotaBytes) > 0 {
		quota = defaultQuotaBytes[0]
	}

	return &Store{
		configDir:         configDir,
		storageDir:        storageDir,
		defaultQuotaBytes: quota,
		startupSalt:       startupSalt,
		userCache:         make(map[string]*cachedMeta),
		tokenCache:        make(map[string]*tokenCacheEntry),
		usageCache:        make(map[string]*cachedUsage),
	}, nil
}

func (s *Store) userFilePath(username string) (string, error) {
	if strings.ContainsAny(username, "/\\\x00") || strings.Contains(username, "..") {
		return "", ErrInvalidUsername
	}
	clean := filepath.Clean(username)
	if clean == "." || clean == "/" || clean == ".." || filepath.Base(clean) != clean {
		return "", ErrInvalidUsername
	}
	return filepath.Join(s.configDir, "users", clean+".json"), nil
}

func (s *Store) UserStoragePath(username string) (string, error) {
	if strings.ContainsAny(username, "/\\\x00") || strings.Contains(username, "..") {
		return "", ErrInvalidUsername
	}
	clean := filepath.Clean(username)
	if clean == "." || clean == "/" || clean == ".." || filepath.Base(clean) != clean {
		return "", ErrInvalidUsername
	}
	return filepath.Join(s.storageDir, "users", clean), nil
}

func (s *Store) EnsureUserProvisioned(username string) (*UserMeta, string, error) {
	userDir, err := s.UserStoragePath(username)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(userDir, 0750); err != nil {
		return nil, "", fmt.Errorf("failed to create user storage dir: %w", err)
	}

	user, err := s.GetUser(username)
	if errors.Is(err, ErrUserNotFound) {
		// New user! Provision initial token
		rawToken, err := auth.GenerateSecureToken("andv")
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate token: %w", err)
		}

		hash, err := auth.HashPassword(rawToken)
		if err != nil {
			return nil, "", fmt.Errorf("failed to hash token: %w", err)
		}

		user = &UserMeta{
			Username:  username,
			CreatedAt: time.Now().UTC(),
			Tokens: []TokenInfo{
				{
					ID:        uuid.NewString(),
					Label:     "Token 1",
					Hash:      hash,
					CreatedAt: time.Now().UTC(),
				},
			},
			Preferences: UserPreferences{
				FirstLoginCompleted: false,
			},
		}

		if err := s.saveUser(user); err != nil {
			return nil, "", fmt.Errorf("failed to save initial user: %w", err)
		}

		return user, rawToken, nil
	} else if err != nil {
		return nil, "", err
	}

	return user, "", nil
}

func (s *Store) GetUser(username string) (*UserMeta, error) {
	filePath, err := s.userFilePath(username)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	s.mu.RLock()
	cached, ok := s.userCache[username]
	if ok && cached.modTime.Equal(stat.ModTime()) {
		metaCopy := *cached.meta
		s.mu.RUnlock()
		return &metaCopy, nil
	}
	s.mu.RUnlock()

	// Read from disk
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var meta UserMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse user json: %w", err)
	}

	s.mu.Lock()
	s.userCache[username] = &cachedMeta{
		modTime: stat.ModTime(),
		meta:    &meta,
	}
	s.mu.Unlock()

	return &meta, nil
}

func (s *Store) saveUser(user *UserMeta) error {
	filePath, err := s.userFilePath(user.Username)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d.%d", filePath, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	// Update memory cache
	stat, _ := os.Stat(filePath)
	s.mu.Lock()
	if stat != nil {
		s.userCache[user.Username] = &cachedMeta{
			modTime: stat.ModTime(),
			meta:    user,
		}
	}
	// Invalidate fast token cache entries for this user
	for key, entry := range s.tokenCache {
		if entry.username == user.Username {
			delete(s.tokenCache, key)
		}
	}
	s.mu.Unlock()

	return nil
}

func (s *Store) AddToken(username, label string) (string, *TokenInfo, error) {
	user, err := s.GetUser(username)
	if err != nil {
		return "", nil, err
	}

	if len(user.Tokens) >= MaxTokensPerUser {
		return "", nil, ErrMaxTokensReached
	}

	if label == "" {
		label = fmt.Sprintf("Token %d", len(user.Tokens)+1)
	}

	rawToken, err := auth.GenerateSecureToken("andv")
	if err != nil {
		return "", nil, err
	}

	hash, err := auth.HashPassword(rawToken)
	if err != nil {
		return "", nil, err
	}

	newToken := TokenInfo{
		ID:        uuid.NewString(),
		Label:     label,
		Hash:      hash,
		CreatedAt: time.Now().UTC(),
	}

	user.Tokens = append(user.Tokens, newToken)

	if err := s.saveUser(user); err != nil {
		return "", nil, err
	}

	return rawToken, &newToken, nil
}

func (s *Store) RevokeToken(username, tokenID string) error {
	user, err := s.GetUser(username)
	if err != nil {
		return err
	}

	found := false
	newTokens := make([]TokenInfo, 0, len(user.Tokens))
	for _, tok := range user.Tokens {
		if tok.ID == tokenID {
			found = true
			continue
		}
		newTokens = append(newTokens, tok)
	}

	if !found {
		return ErrTokenNotFound
	}

	user.Tokens = newTokens
	return s.saveUser(user)
}

func (s *Store) RenameToken(username, tokenID, newLabel string) (*TokenInfo, error) {
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		return nil, ErrEmptyTokenLabel
	}

	user, err := s.GetUser(username)
	if err != nil {
		return nil, err
	}

	var updatedToken *TokenInfo
	for i := range user.Tokens {
		if user.Tokens[i].ID == tokenID {
			user.Tokens[i].Label = newLabel
			updatedToken = &user.Tokens[i]
			break
		}
	}

	if updatedToken == nil {
		return nil, ErrTokenNotFound
	}

	if err := s.saveUser(user); err != nil {
		return nil, err
	}

	tokenCopy := *updatedToken
	return &tokenCopy, nil
}

func (s *Store) SetFirstLoginCompleted(username string) error {
	user, err := s.GetUser(username)
	if err != nil {
		return err
	}
	user.Preferences.FirstLoginCompleted = true
	return s.saveUser(user)
}

func (s *Store) ListUsers() ([]string, error) {
	usersDir := filepath.Join(s.configDir, "users")
	entries, err := os.ReadDir(usersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var users []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			name := entry.Name()
			username := name[:len(name)-5]
			users = append(users, username)
		}
	}
	return users, nil
}

// AuthenticateToken verifies the username and raw token:
// 1. Checks in-memory fast HMAC cache with startupSalt (sub-microsecond).
// 2. On miss, loads user config, runs Argon2id verification, and caches successful results for 10 minutes.
// 3. Verifies mtime against disk to detect cross-pod changes.
func (s *Store) AuthenticateToken(username, rawToken string) (bool, string, error) {
	filePath, err := s.userFilePath(username)
	if err != nil {
		return false, "", err
	}

	cacheKey := s.computeHMACKey(username, rawToken)

	stat, statErr := os.Stat(filePath)

	s.mu.RLock()
	cached, hasCachedMeta := s.userCache[username]
	entry, hasTokenEntry := s.tokenCache[cacheKey]
	s.mu.RUnlock()

	// If mtime on disk changed since we cached user meta, evict cached tokens for this user
	if hasCachedMeta && statErr == nil && !cached.modTime.Equal(stat.ModTime()) {
		s.mu.Lock()
		delete(s.userCache, username)
		for k, e := range s.tokenCache {
			if e.username == username {
				delete(s.tokenCache, k)
			}
		}
		s.mu.Unlock()
		hasTokenEntry = false
	}

	if hasTokenEntry && time.Now().Before(entry.validUntil) {
		return true, entry.tokenID, nil
	}

	// Cache miss: verify against Argon2id hashes
	user, err := s.GetUser(username)
	if err != nil {
		return false, "", err
	}

	var matchedTokenID string
	for _, tok := range user.Tokens {
		match, err := auth.ComparePasswordAndHash(rawToken, tok.Hash)
		if err == nil && match {
			matchedTokenID = tok.ID
			break
		}
	}

	// If miss, retry once with fresh disk read in case of concurrent write
	if matchedTokenID == "" {
		if data, err := os.ReadFile(filePath); err == nil {
			var freshMeta UserMeta
			if json.Unmarshal(data, &freshMeta) == nil {
				for _, tok := range freshMeta.Tokens {
					match, err := auth.ComparePasswordAndHash(rawToken, tok.Hash)
					if err == nil && match {
						matchedTokenID = tok.ID
						break
					}
				}
			}
		}
	}

	if matchedTokenID == "" {
		return false, "", nil
	}

	// Cache successful verification for 10 minutes
	s.mu.Lock()
	s.tokenCache[cacheKey] = &tokenCacheEntry{
		tokenID:    matchedTokenID,
		username:   username,
		validUntil: time.Now().Add(10 * time.Minute),
	}
	s.mu.Unlock()

	return true, matchedTokenID, nil
}

func (s *Store) computeHMACKey(username, rawToken string) string {
	mac := hmac.New(sha256.New, s.startupSalt)
	mac.Write([]byte(username))
	mac.Write([]byte(":"))
	mac.Write([]byte(rawToken))
	return hex.EncodeToString(mac.Sum(nil))
}

// SetDefaultQuota updates the fallback quota in bytes (0 = unlimited).
func (s *Store) SetDefaultQuota(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultQuotaBytes = bytes
}

// DefaultQuota returns the default quota in bytes (0 = unlimited).
func (s *Store) DefaultQuota() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultQuotaBytes
}

// ConfigDir returns the configured config directory path.
func (s *Store) ConfigDir() string {
	return s.configDir
}

// StorageDir returns the configured storage directory path.
func (s *Store) StorageDir() string {
	return s.storageDir
}

// CheckHealth verifies that the critical config and storage directories exist and are writable.
func (s *Store) CheckHealth() error {
	if err := checkDirWritable(s.configDir); err != nil {
		return fmt.Errorf("config directory not writable: %w", err)
	}
	if err := checkDirWritable(s.storageDir); err != nil {
		return fmt.Errorf("storage directory not writable: %w", err)
	}
	return nil
}

func checkDirWritable(dir string) error {
	if dir == "" {
		return errors.New("directory path is empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	tmpFile, err := os.CreateTemp(dir, ".healthcheck-*")
	if err != nil {
		return fmt.Errorf("directory %s is not writable: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(tmpName)
	return nil
}

// GetUserStorageUsage calculates or returns cached disk space used by a user in bytes.
// Usage is calculated from disk once on initial access and maintained in-memory via delta updates.
func (s *Store) GetUserStorageUsage(username string) (int64, error) {
	s.mu.RLock()
	cached, ok := s.usageCache[username]
	if ok {
		bytes := cached.bytes
		s.mu.RUnlock()
		return bytes, nil
	}
	s.mu.RUnlock()

	userDir, err := s.UserStoragePath(username)
	if err != nil {
		return 0, err
	}

	var totalSize int64
	_ = filepath.WalkDir(userDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				totalSize += fi.Size()
			}
		}
		return nil
	})

	s.mu.Lock()
	s.usageCache[username] = &cachedUsage{
		bytes: totalSize,
	}
	s.mu.Unlock()

	return totalSize, nil
}

// InvalidateUsage clears the cached storage usage for a user.
func (s *Store) InvalidateUsage(username string) {
	s.mu.Lock()
	delete(s.usageCache, username)
	s.mu.Unlock()
}

// AddUsageDelta adjusts the in-memory cached storage usage by delta bytes without walking the disk.
func (s *Store) AddUsageDelta(username string, delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.usageCache[username]; ok {
		cached.bytes += delta
		if cached.bytes < 0 {
			cached.bytes = 0
		}
	}
}

// GetUserQuotaLimit returns the effective quota limit in bytes without computing current usage (0 = unlimited).
func (s *Store) GetUserQuotaLimit(username string) (int64, error) {
	user, err := s.GetUser(username)
	if err != nil {
		return 0, err
	}
	if user.QuotaBytes != nil {
		return *user.QuotaBytes, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultQuotaBytes, nil
}

// GetEffectiveQuota returns the user's used bytes and their effective quota limit in bytes (0 = unlimited).
func (s *Store) GetEffectiveQuota(username string) (usedBytes int64, quotaBytes int64, err error) {
	user, err := s.GetUser(username)
	if err != nil {
		return 0, 0, err
	}

	usedBytes, err = s.GetUserStorageUsage(username)
	if err != nil {
		return 0, 0, err
	}

	s.mu.RLock()
	quotaBytes = s.defaultQuotaBytes
	s.mu.RUnlock()

	if user.QuotaBytes != nil {
		quotaBytes = *user.QuotaBytes
	}

	return usedBytes, quotaBytes, nil
}

// SetUserQuota sets or overrides a user's quota in bytes. If quotaBytes is nil, resets to default.
func (s *Store) SetUserQuota(username string, quotaBytes *int64) error {
	if quotaBytes != nil && *quotaBytes < 0 {
		return errors.New("quota cannot be negative")
	}

	user, err := s.GetUser(username)
	if err != nil {
		return err
	}

	user.QuotaBytes = quotaBytes
	return s.saveUser(user)
}

