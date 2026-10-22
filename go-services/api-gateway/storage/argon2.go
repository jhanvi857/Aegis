package storage

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2Params = Argon2Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  1,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// GenerateRandomBytes generates n cryptographically secure random bytes
func GenerateRandomBytes(n uint32) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("failed to read random bytes: %w", err)
	}
	return b, nil
}

// GenerateSalt returns a hex-encoded random salt of the default length
func GenerateSalt() (string, error) {
	b, err := GenerateRandomBytes(DefaultArgon2Params.SaltLength)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashPassword generates an argon2id hash given a password and hex-encoded salt
func HashPassword(password string, saltHex string) (string, error) {
	saltBytes, err := hex.DecodeString(saltHex)
	if err != nil {
		return "", errors.New("invalid salt encoding")
	}

	hash := argon2.IDKey(
		[]byte(password),
		saltBytes,
		DefaultArgon2Params.Iterations,
		DefaultArgon2Params.Memory,
		DefaultArgon2Params.Parallelism,
		DefaultArgon2Params.KeyLength,
	)

	return hex.EncodeToString(hash), nil
}

// VerifyPassword verifies that a password matches the stored argon2id hash
func VerifyPassword(password string, saltHex string, expectedHashHex string) bool {
	computedHashHex, err := HashPassword(password, saltHex)
	if err != nil {
		return false
	}

	expectedBytes, err1 := hex.DecodeString(expectedHashHex)
	computedBytes, err2 := hex.DecodeString(computedHashHex)
	if err1 != nil || err2 != nil {
		return false
	}

	return subtle.ConstantTimeCompare(expectedBytes, computedBytes) == 1
}
