package ueid

// ECIES de-concealment for SUCI identifiers.
//
// Based on: docs/security.md §4.2 (SUCI Encryption Scheme — ECIES Profile A/B)
// 3GPP: TS 33.501 §6.12, Annex C — ECIES Profile A (X25519) and Profile B (secp256r1)
//
// Profile A: X25519 key agreement, ANSI-X9.63-KDF(SHA-256), AES-128-CTR, HMAC-SHA-256 (8 bytes)
// Profile B: secp256r1 (ECDH), ANSI-X9.63-KDF(SHA-256), AES-128-CTR, HMAC-SHA-256 (8 bytes)

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
)

const (
	// ProfileA uses X25519 key agreement per TS 33.501 Annex C.3.
	ProfileA = "A"
	// ProfileB uses secp256r1 (P-256) ECDH key agreement per TS 33.501 Annex C.4.
	ProfileB = "B"

	// macLen is the truncated MAC length (8 bytes / 64 bits).
	macLen = 8
	// aes128KeyLen is the AES-128 key length.
	aes128KeyLen = 16
	// icbLen is the Initial Counter Block length for AES-128-CTR.
	icbLen = 16
)

// DeconcealMSIN decrypts the encrypted MSIN from a SUCI using the home network
// private key and the ephemeral public key sent by the UE.
//
// Parameters:
//   - profileType: "A" (X25519) or "B" (secp256r1)
//   - privateKey: home network private key bytes (32 bytes for both profiles)
//   - ephemeralPubKey: UE's ephemeral public key (32 bytes for A, 33 bytes compressed for B)
//   - cipherText: encrypted MSIN bytes concatenated with 8-byte MAC tag
//
// Returns the plaintext MSIN bytes.
//
// Based on: docs/security.md §4.4 (SUCI Deconceal Process Security)
// 3GPP: TS 33.501 Annex C — ECIES-based SUCI de-concealment
func DeconcealMSIN(profileType string, privateKey, ephemeralPubKey, cipherText []byte) ([]byte, error) {
	if len(cipherText) <= macLen {
		return nil, fmt.Errorf("ecies: ciphertext too short: need >%d bytes, got %d", macLen, len(cipherText))
	}

	// Split cipherText into encrypted MSIN and MAC tag
	encryptedMSIN := cipherText[:len(cipherText)-macLen]
	macTag := cipherText[len(cipherText)-macLen:]

	// Compute shared secret via key agreement
	sharedSecret, err := computeSharedSecret(profileType, privateKey, ephemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("ecies: key agreement failed: %w", err)
	}

	// Derive encryption key (16 bytes), ICB (16 bytes), and MAC key (32 bytes) via KDF
	encKey, icb, macKey, err := deriveKeys(sharedSecret, ephemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("ecies: key derivation failed: %w", err)
	}

	// Verify MAC tag
	expectedMAC := computeMAC(macKey, encryptedMSIN)
	if !hmac.Equal(macTag, expectedMAC) {
		return nil, fmt.Errorf("ecies: MAC verification failed")
	}

	// Decrypt MSIN using AES-128-CTR
	plaintext, err := aesCTRDecrypt(encKey, icb, encryptedMSIN)
	if err != nil {
		return nil, fmt.Errorf("ecies: AES-CTR decryption failed: %w", err)
	}

	return plaintext, nil
}

// computeSharedSecret performs the key agreement for the given profile.
func computeSharedSecret(profileType string, privateKey, ephemeralPubKey []byte) ([]byte, error) {
	switch profileType {
	case ProfileA:
		return computeSharedSecretX25519(privateKey, ephemeralPubKey)
	case ProfileB:
		return computeSharedSecretP256(privateKey, ephemeralPubKey)
	default:
		return nil, fmt.Errorf("unsupported profile type: %s", profileType)
	}
}

// computeSharedSecretX25519 performs X25519 Diffie-Hellman key agreement.
//
// 3GPP: TS 33.501 Annex C.3.4 — Profile A key agreement
func computeSharedSecretX25519(privateKey, ephemeralPubKey []byte) ([]byte, error) {
	if len(privateKey) != 32 {
		return nil, fmt.Errorf("X25519 private key must be 32 bytes, got %d", len(privateKey))
	}
	if len(ephemeralPubKey) != 32 {
		return nil, fmt.Errorf("X25519 ephemeral public key must be 32 bytes, got %d", len(ephemeralPubKey))
	}

	privKey, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid X25519 private key: %w", err)
	}

	pubKey, err := ecdh.X25519().NewPublicKey(ephemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid X25519 public key: %w", err)
	}

	shared, err := privKey.ECDH(pubKey)
	if err != nil {
		return nil, fmt.Errorf("X25519 ECDH failed: %w", err)
	}

	return shared, nil
}

// computeSharedSecretP256 performs ECDH key agreement on secp256r1 (P-256).
//
// 3GPP: TS 33.501 Annex C.4.4 — Profile B key agreement
func computeSharedSecretP256(privateKey, ephemeralPubKey []byte) ([]byte, error) {
	if len(privateKey) != 32 {
		return nil, fmt.Errorf("P-256 private key must be 32 bytes, got %d", len(privateKey))
	}
	if len(ephemeralPubKey) != 33 {
		return nil, fmt.Errorf("P-256 compressed public key must be 33 bytes, got %d", len(ephemeralPubKey))
	}

	curve := elliptic.P256()

	// Decompress the ephemeral public key
	x, y := elliptic.UnmarshalCompressed(curve, ephemeralPubKey)
	if x == nil {
		return nil, fmt.Errorf("invalid compressed P-256 public key")
	}

	// Perform ECDH: shared = privateKey * ephemeralPubKey
	d := new(big.Int).SetBytes(privateKey)
	sx, _ := curve.ScalarMult(x, y, d.Bytes())
	if sx == nil {
		return nil, fmt.Errorf("P-256 ECDH failed")
	}

	// Return the x-coordinate as 32 bytes (zero-padded)
	shared := sx.Bytes()
	if len(shared) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(shared):], shared)
		shared = padded
	}
	return shared, nil
}

// deriveKeys uses ANSI-X9.63-KDF with SHA-256 to derive:
//   - encKey: 16 bytes (AES-128 key)
//   - icb: 16 bytes (Initial Counter Block for AES-128-CTR)
//   - macKey: 32 bytes (HMAC-SHA-256 key)
//
// 3GPP: TS 33.501 Annex C — ANSI-X9.63-KDF with SHA-256
func deriveKeys(sharedSecret, ephemeralPubKey []byte) (encKey, icb, macKey []byte, err error) {
	// Total key material needed: 16 (enc) + 16 (icb) + 32 (mac) = 64 bytes
	// SHA-256 outputs 32 bytes, so we need 2 iterations of the KDF.
	totalLen := aes128KeyLen + icbLen + 32 // 64 bytes

	keyMaterial := ansiX963KDF(sharedSecret, ephemeralPubKey, totalLen)

	encKey = keyMaterial[:aes128KeyLen]
	icb = keyMaterial[aes128KeyLen : aes128KeyLen+icbLen]
	macKey = keyMaterial[aes128KeyLen+icbLen:]

	return encKey, icb, macKey, nil
}

// ansiX963KDF implements ANSI-X9.63-KDF using SHA-256.
//
// KDF(Z, SharedInfo) = Hash(Z || counter || SharedInfo)
// where counter is a 4-byte big-endian integer starting at 1.
//
// 3GPP: TS 33.501 Annex C — Key derivation function
func ansiX963KDF(sharedSecret, sharedInfo []byte, keyLen int) []byte {
	var result []byte
	counter := uint32(1)

	for len(result) < keyLen {
		h := sha256.New()
		h.Write(sharedSecret)
		var ctrBytes [4]byte
		binary.BigEndian.PutUint32(ctrBytes[:], counter)
		h.Write(ctrBytes[:])
		h.Write(sharedInfo)
		result = append(result, h.Sum(nil)...)
		counter++
	}

	return result[:keyLen]
}

// computeMAC computes HMAC-SHA-256 truncated to 8 bytes (64 bits).
//
// 3GPP: TS 33.501 Annex C — MAC computation
func computeMAC(macKey, data []byte) []byte {
	mac := hmac.New(sha256.New, macKey)
	mac.Write(data)
	fullMAC := mac.Sum(nil)
	return fullMAC[:macLen]
}

// aesCTRDecrypt decrypts data using AES-128-CTR.
func aesCTRDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	plaintext := make([]byte, len(ciphertext))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}
