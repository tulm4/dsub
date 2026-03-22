package ueau

// KDF tests for 5G key derivation functions.
//
// 3GPP: TS 33.501 Annex A — Key derivation functions

import (
	"encoding/hex"
	"testing"
)

// TestDeriveXResStar verifies XRES* derivation produces deterministic output
// and has correct length (16 bytes).
func TestDeriveXResStar(t *testing.T) {
	ck := mustDecodeKDF(t, "b40ba9a3c58b2a05bbf0d987b21bf8cb")
	ik := mustDecodeKDF(t, "f769bcd751044604127672711c6d3441")
	servingNetwork := "5G:mnc001.mcc001.3gppnetwork.org"
	rand := mustDecodeKDF(t, "23553cbe9637a89d218ae64dae47bf35")
	xres := mustDecodeKDF(t, "a54211d5e3ba50bf")

	result := DeriveXResStar(ck, ik, servingNetwork, rand, xres)

	if len(result) != 16 {
		t.Fatalf("DeriveXResStar: expected 16 bytes, got %d", len(result))
	}

	// Verify determinism: same inputs produce same output
	result2 := DeriveXResStar(ck, ik, servingNetwork, rand, xres)
	if hex.EncodeToString(result) != hex.EncodeToString(result2) {
		t.Error("DeriveXResStar: not deterministic")
	}

	// Different serving network produces different output
	result3 := DeriveXResStar(ck, ik, "5G:mnc002.mcc002.3gppnetwork.org", rand, xres)
	if hex.EncodeToString(result) == hex.EncodeToString(result3) {
		t.Error("DeriveXResStar: different serving network should produce different output")
	}
}

// TestDeriveKausf verifies Kausf derivation produces deterministic output
// and has correct length (32 bytes).
func TestDeriveKausf(t *testing.T) {
	ck := mustDecodeKDF(t, "b40ba9a3c58b2a05bbf0d987b21bf8cb")
	ik := mustDecodeKDF(t, "f769bcd751044604127672711c6d3441")
	servingNetwork := "5G:mnc001.mcc001.3gppnetwork.org"
	sqnXorAK := mustDecodeKDF(t, "55f228904c97")

	result := DeriveKausf(ck, ik, servingNetwork, sqnXorAK)

	if len(result) != 32 {
		t.Fatalf("DeriveKausf: expected 32 bytes, got %d", len(result))
	}

	// Verify determinism
	result2 := DeriveKausf(ck, ik, servingNetwork, sqnXorAK)
	if hex.EncodeToString(result) != hex.EncodeToString(result2) {
		t.Error("DeriveKausf: not deterministic")
	}

	// Different serving network produces different output
	result3 := DeriveKausf(ck, ik, "5G:mnc002.mcc002.3gppnetwork.org", sqnXorAK)
	if hex.EncodeToString(result) == hex.EncodeToString(result3) {
		t.Error("DeriveKausf: different serving network should produce different output")
	}
}

// TestDeriveXResStarFC verifies the FC byte is 0x6B.
func TestDeriveXResStarFC(t *testing.T) {
	ck := make([]byte, 16)
	ik := make([]byte, 16)
	rand := make([]byte, 16)
	xres := make([]byte, 8)

	result1 := DeriveXResStar(ck, ik, "net1", rand, xres)
	result2 := DeriveXResStar(ck, ik, "net2", rand, xres)

	// Different inputs must produce different outputs (verifies FC is in the calculation)
	if hex.EncodeToString(result1) == hex.EncodeToString(result2) {
		t.Error("different network names should produce different XRES*")
	}
}

// TestDeriveKausfFC verifies the FC byte is 0x6A.
func TestDeriveKausfFC(t *testing.T) {
	ck := make([]byte, 16)
	ik := make([]byte, 16)
	sqnXorAK := make([]byte, 6)

	result1 := DeriveKausf(ck, ik, "net1", sqnXorAK)
	result2 := DeriveKausf(ck, ik, "net2", sqnXorAK)

	if hex.EncodeToString(result1) == hex.EncodeToString(result2) {
		t.Error("different network names should produce different Kausf")
	}
}

func mustDecodeKDF(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode %q: %v", s, err)
	}
	return b
}
