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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/t0ggl/andvari/internal/auth"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrMaxTokensReached = errors.New("maximum of 2 active tokens allowed; revoke an existing token to create a new one")
	ErrTokenNotFound    = errors.New("token not found")
	ErrInvalidUsername  = errors.New("invalid username")
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
}

type Store struct {
	configDir   string
	storageDir  string
	startupSalt []byte
	mu          sync.RWMutex

	// In-memory cache for user metadata: username -> cachedMeta
	userCache map[string]*cachedMeta
	// In-memory token verification cache: hmac(startupSalt, username+":"+token) -> tokenCacheEntry
	tokenCache map[string]*tokenCacheEntry
}

type cachedMeta struct {
	modTime time.Time
	meta    *UserMeta
}

type tokenCacheEntry struct {
	tokenID    string
	username   string
	validUntil time.Time
}

func NewStore(configDir, storageDir string) (*Store, error) {
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

	return &Store{
		configDir:   configDir,
		storageDir:  storageDir,
		startupSalt: startupSalt,
		userCache:   make(map[string]*cachedMeta),
		tokenCache:  make(map[string]*tokenCacheEntry),
	}, nil
}

func (s *Store) userFilePath(username string) (string, error) {
	clean := filepath.Clean(username)
	if clean == "." || clean == "/" || clean == ".." || filepath.Base(clean) != clean {
		return "", ErrInvalidUsername
	}
	return filepath.Join(s.configDir, "users", clean+".json"), nil
}

func (s *Store) UserStoragePath(username string) (string, error) {
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

	if len(user.Tokens) >= 2 {
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
