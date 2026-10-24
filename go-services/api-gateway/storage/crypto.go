package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

var (
	cryptoKeyOnce sync.Once
	cryptoKey     []byte
)

func getEncryptionKey() []byte {
	cryptoKeyOnce.Do(func() {
		raw := os.Getenv("AEGIS_ENCRYPTION_KEY")
		if raw == "" {
			raw = "aegis-production-data-model-secret-key-salt-2026"
		}
		h := sha256.Sum256([]byte(raw))
		cryptoKey = h[:]
	})
	return cryptoKey
}

// EncryptCredential encrypts sensitive credentials using AES-256-GCM
func EncryptCredential(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	// If already encrypted prefix, avoid double encryption
	if strings.HasPrefix(plaintext, "enc::") {
		return plaintext, nil
	}

	key := getEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc::" + hex.EncodeToString(sealed), nil
}

// DecryptCredential decrypts AES-256-GCM encrypted credentials
func DecryptCredential(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if !strings.HasPrefix(encoded, "enc::") {
		return encoded, nil
	}

	hexData := strings.TrimPrefix(encoded, "enc::")
	cipherData, err := hex.DecodeString(hexData)
	if err != nil {
		return "", err
	}

	key := getEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(cipherData) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := cipherData[:nonceSize], cipherData[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plain), nil
}

// RedactTelemetrySource returns a copy of TelemetrySource with sensitive credentials stripped
func RedactTelemetrySource(src *TelemetrySource) *TelemetrySource {
	if src == nil {
		return nil
	}
	cp := *src
	cp.AuthToken = ""
	cp.AuthPassword = ""
	return &cp
}
