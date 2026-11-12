package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024 // 64 MB
	argonTime        = 1
	argonThreads     = 4
	argonKeyLen      = 32
	argonSaltLen     = 16
	argonVersion     = argon2.Version
	argonTypePrefix  = "$argon2id$"
)

var (
	ErrInvalidHash         = errors.New("invalid argon2id hash format")
	ErrIncompatibleVersion = errors.New("incompatible argon2 version")
)

// HashPassword hashes a raw password/token string using Argon2id.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("%sv=%d$m=%d,t=%d,p=%d$%s$%s",
		argonTypePrefix, argonVersion, argonMemory, argonTime, argonThreads, b64Salt, b64Hash)

	return encoded, nil
}

// ComparePasswordAndHash verifies a plaintext password against an encoded Argon2id hash.
func ComparePasswordAndHash(password, encodedHash string) (bool, error) {
	if !strings.HasPrefix(encodedHash, argonTypePrefix) {
		return false, ErrInvalidHash
	}

	parts := strings.Split(encodedHash[len(argonTypePrefix):], "$")
	if len(parts) != 4 {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[0], "v=%d", &version); err != nil || version != argonVersion {
		return false, ErrIncompatibleVersion
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[1], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, ErrInvalidHash
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, ErrInvalidHash
	}

	comparisonHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	if subtle.ConstantTimeCompare(comparisonHash, expectedHash) == 1 {
		return true, nil
	}

	return false, nil
}

// GenerateSecureToken creates a random token with prefix (e.g. andv_...).
func GenerateSecureToken(prefix string) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	if prefix != "" {
		return prefix + "_" + token, nil
	}
	return token, nil
}
