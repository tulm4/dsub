// Package crypto provides column-level encryption utilities for the 5G UDM
// network function, specifically for protecting authentication credentials
// (K, OPc) stored in YugabyteDB.
//
// Based on: docs/security.md §6.3 (Column-Level Encryption)
// Based on: docs/security.md §5 (Authentication Credential Security)
// 3GPP: TS 33.501 §6.1 — Key storage and protection
//
// Algorithm: AES-256-GCM with per-value unique nonce.
// Key hierarchy: KEK (Key Encryption Key) wraps per-row DEKs (Data Encryption Keys).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Errors returned by encryption operations.
var (
	ErrInvalidKeySize   = errors.New("crypto: key must be 32 bytes (AES-256)")
	ErrCiphertextShort  = errors.New("crypto: ciphertext too short")
	ErrDecryptionFailed = errors.New("crypto: decryption failed (authentication tag mismatch)")
)

// KeySize is the required key size for AES-256 in bytes.
const KeySize = 32

// NonceSize is the standard GCM nonce size in bytes.
const NonceSize = 12

// ColumnEncryptor provides AES-256-GCM encryption and decryption for
// database column values. It uses a KEK (Key Encryption Key) for all
// operations.
//
// Based on: docs/security.md §6.3 (Column-Level Encryption)
// 3GPP: TS 33.501 §6.1 — Authentication credential protection
type ColumnEncryptor struct {
	kek []byte // Key Encryption Key (32 bytes)
}

// NewColumnEncryptor creates a new encryptor with the given KEK.
// The key must be exactly 32 bytes (AES-256).
//
// Based on: docs/security.md §6.3 (Key Hierarchy)
func NewColumnEncryptor(kek []byte) (*ColumnEncryptor, error) {
	if len(kek) != KeySize {
		return nil, ErrInvalidKeySize
	}
	// Copy the key to prevent external mutation.
	keyCopy := make([]byte, KeySize)
	copy(keyCopy, kek)
	return &ColumnEncryptor{kek: keyCopy}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a random nonce.
// The nonce is prepended to the ciphertext so the result is self-contained:
//
//	result = nonce (12 bytes) || ciphertext || GCM tag (16 bytes)
//
// Based on: docs/security.md §6.3 (AES-256-GCM encryption)
func (e *ColumnEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.kek)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends the ciphertext and GCM tag to nonce.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt. It expects the format:
//
//	input = nonce (12 bytes) || ciphertext || GCM tag (16 bytes)
//
// Returns the original plaintext, or an error if the ciphertext is too short,
// corrupted, or the authentication tag does not match (tampering detected).
//
// Based on: docs/security.md §6.3 (AES-256-GCM decryption)
func (e *ColumnEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.kek)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize+gcm.Overhead() {
		return nil, ErrCiphertextShort
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// GenerateKey generates a cryptographically secure random 32-byte key
// suitable for use as a KEK or DEK.
//
// Based on: docs/security.md §6.3 (Key Generation)
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: generate key: %w", err)
	}
	return key, nil
}

// ZeroKey overwrites a key slice with zeros. Call this when the key is no
// longer needed to minimize the window of key exposure in memory.
//
// Based on: docs/security.md §5.1 (Key Material Zeroization)
// 3GPP: TS 33.501 §6.1.3 — Key lifetime management
func ZeroKey(key []byte) {
	for i := range key {
		key[i] = 0
	}
}
