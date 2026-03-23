package crypto

import (
	"bytes"
	"testing"
)

// ---------------------------------------------------------------------------
// ColumnEncryptor tests
// ---------------------------------------------------------------------------

func TestNewColumnEncryptor_ValidKey(t *testing.T) {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := NewColumnEncryptor(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc == nil {
		t.Fatal("expected non-nil encryptor")
	}
}

func TestNewColumnEncryptor_InvalidKeySize(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{name: "too short (16 bytes)", keySize: 16},
		{name: "too short (24 bytes)", keySize: 24},
		{name: "too long (48 bytes)", keySize: 48},
		{name: "empty key", keySize: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			_, err := NewColumnEncryptor(key)
			if err != ErrInvalidKeySize {
				t.Errorf("expected ErrInvalidKeySize, got %v", err)
			}
		})
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	enc, err := NewColumnEncryptor(key)
	if err != nil {
		t.Fatalf("NewColumnEncryptor: %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{name: "K key (16 bytes)", plaintext: bytes.Repeat([]byte{0xAB}, 16)},
		{name: "OPc key (16 bytes)", plaintext: bytes.Repeat([]byte{0xCD}, 16)},
		{name: "empty data", plaintext: []byte{}},
		{name: "single byte", plaintext: []byte{0xFF}},
		{name: "large payload", plaintext: bytes.Repeat([]byte{0x42}, 4096)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			// Ciphertext should be different from plaintext.
			if len(tt.plaintext) > 0 && bytes.Equal(ciphertext, tt.plaintext) {
				t.Error("ciphertext should differ from plaintext")
			}

			// Ciphertext should include nonce + GCM tag overhead.
			minSize := NonceSize + 16 // nonce + tag
			if len(ciphertext) < minSize {
				t.Errorf("ciphertext too short: %d bytes, expected at least %d", len(ciphertext), minSize)
			}

			decrypted, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("decrypted data does not match original\ngot:  %x\nwant: %x", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncrypt_ProducesUniqueCiphertext(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	enc, err := NewColumnEncryptor(key)
	if err != nil {
		t.Fatalf("NewColumnEncryptor: %v", err)
	}

	plaintext := []byte("authentication-key-material")

	ct1, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt: %v", err)
	}

	ct2, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt: %v", err)
	}

	// Two encryptions of the same plaintext should produce different ciphertext
	// due to random nonces.
	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of the same plaintext should produce different ciphertext (unique nonces)")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	enc1, _ := NewColumnEncryptor(key1)
	enc2, _ := NewColumnEncryptor(key2)

	plaintext := []byte("secret-K-material")
	ciphertext, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypting with a different key should fail.
	_, err = enc2.Decrypt(ciphertext)
	if err != ErrDecryptionFailed {
		t.Errorf("expected ErrDecryptionFailed, got %v", err)
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewColumnEncryptor(key)

	plaintext := []byte("secret-OPc-material")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Tamper with the ciphertext (flip a byte in the middle).
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)/2] ^= 0xFF

	_, err = enc.Decrypt(tampered)
	if err != ErrDecryptionFailed {
		t.Errorf("expected ErrDecryptionFailed for tampered data, got %v", err)
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewColumnEncryptor(key)

	// Too short to contain nonce + tag.
	_, err := enc.Decrypt([]byte{1, 2, 3})
	if err != ErrCiphertextShort {
		t.Errorf("expected ErrCiphertextShort, got %v", err)
	}

	// Exactly nonce size should still be too short (no tag).
	_, err = enc.Decrypt(make([]byte, NonceSize))
	if err != ErrCiphertextShort {
		t.Errorf("expected ErrCiphertextShort for nonce-only data, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GenerateKey tests
// ---------------------------------------------------------------------------

func TestGenerateKey_Size(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != KeySize {
		t.Errorf("key size = %d, want %d", len(key), KeySize)
	}
}

func TestGenerateKey_Unique(t *testing.T) {
	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if bytes.Equal(key1, key2) {
		t.Error("two generated keys should not be equal")
	}
}

// ---------------------------------------------------------------------------
// ZeroKey tests
// ---------------------------------------------------------------------------

func TestZeroKey(t *testing.T) {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}

	ZeroKey(key)

	for i, b := range key {
		if b != 0 {
			t.Errorf("byte at index %d should be 0 after zeroization, got %d", i, b)
		}
	}
}

func TestZeroKey_EmptySlice(t *testing.T) {
	// Should not panic.
	ZeroKey(nil)
	ZeroKey([]byte{})
}

// ---------------------------------------------------------------------------
// Key copy isolation test
// ---------------------------------------------------------------------------

func TestColumnEncryptor_KeyIsolation(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewColumnEncryptor(key)

	plaintext := []byte("test-data")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Mutate the original key — should not affect the encryptor.
	for i := range key {
		key[i] = 0
	}

	// Decrypt should still work because the encryptor holds an internal copy.
	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt after key mutation: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted data does not match after external key mutation")
	}
}
