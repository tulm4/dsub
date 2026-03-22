package ueid

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// TestDeconcealMSIN_ProfileA tests ECIES Profile A (X25519) de-concealment.
//
// 3GPP: TS 33.501 Annex C.3 — Profile A test
func TestDeconcealMSIN_ProfileA(t *testing.T) {
	// Generate a key pair for the home network
	hnPrivKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate X25519 key: %v", err)
	}
	hnPubKey := hnPrivKey.PublicKey()

	// Simulate UE-side encryption
	plainMSIN := []byte("0123456789")

	// Generate UE ephemeral key pair
	uePrivKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate UE ephemeral key: %v", err)
	}
	uePubKey := uePrivKey.PublicKey()

	// UE computes shared secret using HN public key
	sharedSecret, err := uePrivKey.ECDH(hnPubKey)
	if err != nil {
		t.Fatalf("UE ECDH: %v", err)
	}

	// Derive keys using the same KDF
	encKey, icb, macKey, err := deriveKeys(sharedSecret, uePubKey.Bytes())
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}

	// Encrypt MSIN with AES-128-CTR
	encryptedMSIN, err := aesCTRDecrypt(encKey, icb, plainMSIN)
	if err != nil {
		t.Fatalf("AES-CTR encrypt: %v", err)
	}

	// Compute MAC
	mac := computeMAC(macKey, encryptedMSIN)

	// Build cipher data: encrypted MSIN + MAC
	cipherText := append(encryptedMSIN, mac...)

	// Deconceal using the home network private key
	decrypted, err := DeconcealMSIN(ProfileA, hnPrivKey.Bytes(), uePubKey.Bytes(), cipherText)
	if err != nil {
		t.Fatalf("DeconcealMSIN failed: %v", err)
	}

	if string(decrypted) != string(plainMSIN) {
		t.Errorf("decrypted MSIN mismatch: got %q, want %q", string(decrypted), string(plainMSIN))
	}
}

// TestDeconcealMSIN_ProfileB tests ECIES Profile B (secp256r1/P-256) de-concealment.
//
// 3GPP: TS 33.501 Annex C.4 — Profile B test
func TestDeconcealMSIN_ProfileB(t *testing.T) {
	curve := elliptic.P256()

	// Generate home network key pair
	hnPrivKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate P-256 key: %v", err)
	}
	hnPrivBytes := hnPrivKey.D.Bytes()
	if len(hnPrivBytes) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(hnPrivBytes):], hnPrivBytes)
		hnPrivBytes = padded
	}

	// Generate UE ephemeral key pair
	uePrivKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate UE ephemeral key: %v", err)
	}
	uePubKeyCompressed := elliptic.MarshalCompressed(curve, uePrivKey.PublicKey.X, uePrivKey.PublicKey.Y)

	// UE computes shared secret: uePriv * hnPub
	sx, _ := curve.ScalarMult(hnPrivKey.PublicKey.X, hnPrivKey.PublicKey.Y, uePrivKey.D.Bytes())
	sharedSecret := sx.Bytes()
	if len(sharedSecret) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(sharedSecret):], sharedSecret)
		sharedSecret = padded
	}

	// Derive keys
	encKey, icb, macKey, err := deriveKeys(sharedSecret, uePubKeyCompressed)
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}

	plainMSIN := []byte("9876543210")

	// Encrypt
	encryptedMSIN, err := aesCTRDecrypt(encKey, icb, plainMSIN)
	if err != nil {
		t.Fatalf("AES-CTR encrypt: %v", err)
	}

	// MAC
	mac := computeMAC(macKey, encryptedMSIN)
	cipherText := append(encryptedMSIN, mac...)

	// Deconceal
	decrypted, err := DeconcealMSIN(ProfileB, hnPrivBytes, uePubKeyCompressed, cipherText)
	if err != nil {
		t.Fatalf("DeconcealMSIN failed: %v", err)
	}

	if string(decrypted) != string(plainMSIN) {
		t.Errorf("decrypted MSIN mismatch: got %q, want %q", string(decrypted), string(plainMSIN))
	}
}

// TestDeconcealMSIN_InvalidMAC verifies that a wrong MAC tag causes failure.
func TestDeconcealMSIN_InvalidMAC(t *testing.T) {
	hnPrivKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	hnPubKey := hnPrivKey.PublicKey()

	uePrivKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate UE key: %v", err)
	}
	uePubKey := uePrivKey.PublicKey()

	sharedSecret, _ := uePrivKey.ECDH(hnPubKey)
	encKey, icb, _, _ := deriveKeys(sharedSecret, uePubKey.Bytes())

	plainMSIN := []byte("0123456789")
	encryptedMSIN, _ := aesCTRDecrypt(encKey, icb, plainMSIN)

	// Wrong MAC (all zeros)
	badMAC := make([]byte, macLen)
	cipherText := append(encryptedMSIN, badMAC...)

	_, err = DeconcealMSIN(ProfileA, hnPrivKey.Bytes(), uePubKey.Bytes(), cipherText)
	if err == nil {
		t.Error("expected MAC verification failure, got nil error")
	}
}

// TestDeconcealMSIN_CipherTextTooShort verifies rejection of too-short cipher data.
func TestDeconcealMSIN_CipherTextTooShort(t *testing.T) {
	_, err := DeconcealMSIN(ProfileA, make([]byte, 32), make([]byte, 32), make([]byte, macLen))
	if err == nil {
		t.Error("expected error for short ciphertext, got nil")
	}
}

// TestDeconcealMSIN_UnsupportedProfile verifies rejection of unknown profile types.
func TestDeconcealMSIN_UnsupportedProfile(t *testing.T) {
	_, err := DeconcealMSIN("C", make([]byte, 32), make([]byte, 32), make([]byte, 20))
	if err == nil {
		t.Error("expected error for unsupported profile, got nil")
	}
}

// TestAnsiX963KDF verifies the KDF produces deterministic output.
func TestAnsiX963KDF(t *testing.T) {
	secret := []byte("test-shared-secret")
	info := []byte("test-shared-info")

	result1 := ansiX963KDF(secret, info, 64)
	result2 := ansiX963KDF(secret, info, 64)

	if hex.EncodeToString(result1) != hex.EncodeToString(result2) {
		t.Error("KDF is not deterministic")
	}

	if len(result1) != 64 {
		t.Errorf("KDF output length: got %d, want 64", len(result1))
	}
}

// TestSplitCipherData_ProfileA tests cipher data splitting for Profile A.
func TestSplitCipherData_ProfileA(t *testing.T) {
	// 32 (key) + 10 (encrypted) + 8 (mac) = 50 bytes
	data := make([]byte, 50)
	for i := range data {
		data[i] = byte(i)
	}

	eph, ct, err := splitCipherData(ProfileA, data)
	if err != nil {
		t.Fatalf("splitCipherData: %v", err)
	}
	if len(eph) != 32 {
		t.Errorf("ephemeral key length: got %d, want 32", len(eph))
	}
	if len(ct) != 18 { // 10 + 8
		t.Errorf("ciphertext length: got %d, want 18", len(ct))
	}
}

// TestSplitCipherData_ProfileB tests cipher data splitting for Profile B.
func TestSplitCipherData_ProfileB(t *testing.T) {
	// 33 (key) + 10 (encrypted) + 8 (mac) = 51 bytes
	data := make([]byte, 51)
	eph, ct, err := splitCipherData(ProfileB, data)
	if err != nil {
		t.Fatalf("splitCipherData: %v", err)
	}
	if len(eph) != 33 {
		t.Errorf("ephemeral key length: got %d, want 33", len(eph))
	}
	if len(ct) != 18 {
		t.Errorf("ciphertext length: got %d, want 18", len(ct))
	}
}

// TestSplitCipherData_TooShort verifies rejection of cipher data that's too short.
func TestSplitCipherData_TooShort(t *testing.T) {
	_, _, err := splitCipherData(ProfileA, make([]byte, 40))
	if err == nil {
		t.Error("expected error for short data, got nil")
	}
}

// TestSchemeIDToProfileType verifies scheme ID mapping.
func TestSchemeIDToProfileType(t *testing.T) {
	tests := []struct {
		schemeID    int
		wantProfile string
		wantErr     bool
	}{
		{1, ProfileA, false},
		{2, ProfileB, false},
		{0, "", true},
		{3, "", true},
	}
	for _, tt := range tests {
		got, err := schemeIDToProfileType(tt.schemeID)
		if (err != nil) != tt.wantErr {
			t.Errorf("schemeIDToProfileType(%d): err=%v, wantErr=%v", tt.schemeID, err, tt.wantErr)
		}
		if got != tt.wantProfile {
			t.Errorf("schemeIDToProfileType(%d): got %q, want %q", tt.schemeID, got, tt.wantProfile)
		}
	}
}
