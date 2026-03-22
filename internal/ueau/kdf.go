package ueau

// 5G Key Derivation Functions per TS 33.501 Annex A.
//
// Based on: docs/service-decomposition.md §2.1 (udm-ueau — 5G key derivation)
// 3GPP: TS 33.501 Annex A — Key derivation functions
// 3GPP: TS 33.220 Annex B — KDF based on HMAC-SHA-256

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// DeriveXResStar computes XRES* from CK, IK, serving network name, RAND, and XRES.
//
// XRES* = KDF(CK||IK, FC=0x6B || servingNetworkName || RAND || XRES)
//
// The KDF uses HMAC-SHA-256 with key = CK||IK.
// The input S is constructed as:
//
//	FC (1 byte) || P0 || L0 || P1 || L1 || P2 || L2
//
// where P0=servingNetworkName, P1=RAND, P2=XRES, and L is length in 2 bytes big-endian.
//
// Returns the last 16 bytes of the 32-byte HMAC output per TS 33.501 A.4.
//
// 3GPP: TS 33.501 Annex A.4 — XRES* derivation
func DeriveXResStar(ck, ik []byte, servingNetworkName string, rand, xres []byte) []byte {
	key := make([]byte, 0, len(ck)+len(ik))
	key = append(key, ck...)
	key = append(key, ik...)

	snBytes := []byte(servingNetworkName)

	// Build KDF input S: FC, serving network name, RAND, XRES with length prefixes.
	s := make([]byte, 0, 1+len(snBytes)+2+len(rand)+2+len(xres)+2)
	s = append(s, 0x6B) // FC
	s = append(s, snBytes...)
	s = appendLength(s, len(snBytes))
	s = append(s, rand...)
	s = appendLength(s, len(rand))
	s = append(s, xres...)
	s = appendLength(s, len(xres))

	mac := hmac.New(sha256.New, key)
	mac.Write(s)
	hash := mac.Sum(nil)

	// Return last 16 bytes per TS 33.501 A.4
	return hash[16:32]
}

// DeriveKausf computes Kausf from CK, IK, serving network name, and SQN⊕AK.
//
// Kausf = KDF(CK||IK, FC=0x6A || servingNetworkName || SQN⊕AK)
//
// The KDF uses HMAC-SHA-256 with key = CK||IK.
//
// 3GPP: TS 33.501 Annex A.2 — Kausf derivation
func DeriveKausf(ck, ik []byte, servingNetworkName string, sqnXorAK []byte) []byte {
	key := make([]byte, 0, len(ck)+len(ik))
	key = append(key, ck...)
	key = append(key, ik...)

	snBytes := []byte(servingNetworkName)

	// Build KDF input S: FC, serving network name, SQN⊕AK with length prefixes.
	s := make([]byte, 0, 1+len(snBytes)+2+len(sqnXorAK)+2)
	s = append(s, 0x6A) // FC
	s = append(s, snBytes...)
	s = appendLength(s, len(snBytes))
	s = append(s, sqnXorAK...)
	s = appendLength(s, len(sqnXorAK))

	mac := hmac.New(sha256.New, key)
	mac.Write(s)
	return mac.Sum(nil)
}

// appendLength appends a 2-byte big-endian length to s.
func appendLength(s []byte, length int) []byte {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], uint16(length))
	return append(s, buf[:]...)
}
