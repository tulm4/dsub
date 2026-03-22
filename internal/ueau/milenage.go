package ueau

// Milenage authentication algorithm implementation.
//
// Based on: docs/service-decomposition.md §2.1 (udm-ueau — auth vector generation)
// 3GPP: TS 35.206 — Specification of the Milenage Algorithm Set
// 3GPP: TS 35.207 — Test Data for the Milenage Algorithm Set
// 3GPP: TS 33.501 §6.1.3 — 5G-AKA authentication procedure

import (
	"crypto/aes"
	"fmt"
)

// Milenage rotation constants (r1–r5) per TS 35.206 §3.
const (
	r1 = 64
	r2 = 0
	r3 = 32
	r4 = 64
	r5 = 96
)

// Milenage additive constants (c1–c5) per TS 35.206 §3.
// Each is a 128-bit value; only the last byte differs.
var (
	c1 = [16]byte{}
	c2 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	c3 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	c4 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4}
	c5 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8}
)

// ComputeOPc derives OPc from K and OP per TS 35.206 §2.2:
//
//	OPc = OP ⊕ AES_K(OP)
func ComputeOPc(k, op []byte) ([]byte, error) {
	if len(k) != 16 {
		return nil, fmt.Errorf("milenage: K must be 16 bytes, got %d", len(k))
	}
	if len(op) != 16 {
		return nil, fmt.Errorf("milenage: OP must be 16 bytes, got %d", len(op))
	}

	cipher, err := aes.NewCipher(k)
	if err != nil {
		return nil, fmt.Errorf("milenage: AES cipher: %w", err)
	}

	encrypted := make([]byte, 16)
	cipher.Encrypt(encrypted, op)

	opc := make([]byte, 16)
	xorBytes(opc, op, encrypted)
	return opc, nil
}

// GenerateAuthVector computes the full set of Milenage outputs (f1–f5)
// and constructs the AUTN parameter.
//
// Parameters:
//   - k: 128-bit subscriber key
//   - opc: 128-bit derived operator key (OPc)
//   - sqn: 48-bit sequence number (6 bytes)
//   - amf: 16-bit authentication management field (2 bytes)
//   - rand: 128-bit random challenge (16 bytes)
//
// Returns MilenageOutput containing MAC-A, XRES, CK, IK, AK, and AUTN.
//
// 3GPP: TS 35.206 §3 — Milenage algorithm specification
func GenerateAuthVector(k, opc, sqn, amf, rand []byte) (*MilenageOutput, error) {
	if len(k) != 16 {
		return nil, fmt.Errorf("milenage: K must be 16 bytes, got %d", len(k))
	}
	if len(opc) != 16 {
		return nil, fmt.Errorf("milenage: OPc must be 16 bytes, got %d", len(opc))
	}
	if len(sqn) != 6 {
		return nil, fmt.Errorf("milenage: SQN must be 6 bytes, got %d", len(sqn))
	}
	if len(amf) != 2 {
		return nil, fmt.Errorf("milenage: AMF must be 2 bytes, got %d", len(amf))
	}
	if len(rand) != 16 {
		return nil, fmt.Errorf("milenage: RAND must be 16 bytes, got %d", len(rand))
	}

	cipher, err := aes.NewCipher(k)
	if err != nil {
		return nil, fmt.Errorf("milenage: AES cipher: %w", err)
	}

	// Step 1: TEMP = AES_K(RAND ⊕ OPc)
	var temp [16]byte
	var inp [16]byte
	xorBytes(inp[:], rand, opc)
	cipher.Encrypt(temp[:], inp[:])

	// Step 2: f1 — compute OUT1 for MAC-A
	// IN1 is constructed per TS 35.206 §4.1:
	//   IN1[0..47]    = SQN (bits 0–47)
	//   IN1[48..63]   = AMF (bits 0–15)
	//   IN1[64..111]  = SQN (bits 0–47)
	//   IN1[112..127] = AMF (bits 0–15)
	var in1 [16]byte
	copy(in1[0:6], sqn)
	copy(in1[6:8], amf)
	copy(in1[8:14], sqn)
	copy(in1[14:16], amf)

	// OUT1 = AES_K(TEMP ⊕ rot(IN1 ⊕ OPc, r1) ⊕ c1) ⊕ OPc
	// Note: f1 rotates (IN1 ⊕ OPc), unlike f2–f5 which rotate (TEMP ⊕ OPc).
	var in1XorOPc [16]byte
	xorBytes(in1XorOPc[:], in1[:], opc)

	var rotated [16]byte
	rotateLeft(rotated[:], in1XorOPc[:], r1)
	var out1In [16]byte
	xorBytesThree(out1In[:], temp[:], rotated[:], c1[:])
	var out1 [16]byte
	cipher.Encrypt(out1[:], out1In[:])
	xorBytesInPlace(out1[:], opc)

	var tempXorOPc [16]byte
	xorBytes(tempXorOPc[:], temp[:], opc)

	// f1 = OUT1[0..63] → first 8 bytes → MAC-A
	mac := make([]byte, 8)
	copy(mac, out1[0:8])

	// Step 3: f2 and f5 share the same OUT2 block per TS 35.206 §4.1
	// OUT2 = AES_K(rot(TEMP ⊕ OPc, r2) ⊕ c2) ⊕ OPc
	var rot2 [16]byte
	rotateLeft(rot2[:], tempXorOPc[:], r2)
	var out2In [16]byte
	xorBytes(out2In[:], rot2[:], c2[:])
	var out2 [16]byte
	cipher.Encrypt(out2[:], out2In[:])
	xorBytesInPlace(out2[:], opc)

	// f2 = OUT2[64..127] → bytes 8–15 → XRES
	xres := make([]byte, 8)
	copy(xres, out2[8:16])

	// f5 = OUT2[0..47] → bytes 0–5 → AK
	ak := make([]byte, 6)
	copy(ak, out2[0:6])

	// Step 4: f3 — compute OUT3 for CK
	// OUT3 = AES_K(rot(TEMP ⊕ OPc, r3) ⊕ c3) ⊕ OPc
	var rot3 [16]byte
	rotateLeft(rot3[:], tempXorOPc[:], r3)
	var out3In [16]byte
	xorBytes(out3In[:], rot3[:], c3[:])
	var out3 [16]byte
	cipher.Encrypt(out3[:], out3In[:])
	xorBytesInPlace(out3[:], opc)

	// f3 = OUT3[0..127] → all 16 bytes → CK
	ck := make([]byte, 16)
	copy(ck, out3[:])

	// Step 5: f4 — compute OUT4 for IK
	// OUT4 = AES_K(rot(TEMP ⊕ OPc, r4) ⊕ c4) ⊕ OPc
	var rot4 [16]byte
	rotateLeft(rot4[:], tempXorOPc[:], r4)
	var out4In [16]byte
	xorBytes(out4In[:], rot4[:], c4[:])
	var out4 [16]byte
	cipher.Encrypt(out4[:], out4In[:])
	xorBytesInPlace(out4[:], opc)

	// f4 = OUT4[0..127] → all 16 bytes → IK
	ik := make([]byte, 16)
	copy(ik, out4[:])

	// AUTN = (SQN ⊕ AK) || AMF || MAC-A
	autn := make([]byte, 16)
	for i := 0; i < 6; i++ {
		autn[i] = sqn[i] ^ ak[i]
	}
	copy(autn[6:8], amf)
	copy(autn[8:16], mac)

	return &MilenageOutput{
		MAC:  mac,
		XRES: xres,
		CK:   ck,
		IK:   ik,
		AK:   ak,
		AUTN: autn,
	}, nil
}

// xorBytes computes dst = a ⊕ b. All slices must be the same length.
func xorBytes(dst, a, b []byte) {
	for i := range dst {
		dst[i] = a[i] ^ b[i]
	}
}

// xorBytesInPlace computes dst = dst ⊕ b.
func xorBytesInPlace(dst, b []byte) {
	for i := range dst {
		dst[i] ^= b[i]
	}
}

// xorBytesThree computes dst = a ⊕ b ⊕ c. All slices must be the same length.
func xorBytesThree(dst, a, b, c []byte) {
	for i := range dst {
		dst[i] = a[i] ^ b[i] ^ c[i]
	}
}

// rotateLeft performs a bitwise left rotation of a 128-bit (16-byte) value by n bits.
// Per TS 35.206, rotation is a cyclic left shift of the full 128-bit block.
func rotateLeft(dst, src []byte, n int) {
	n = n % 128
	if n == 0 {
		copy(dst, src)
		return
	}

	byteShift := n / 8
	bitShift := uint(n % 8)

	for i := 0; i < 16; i++ {
		srcIdx := (i + byteShift) % 16
		nextIdx := (i + byteShift + 1) % 16
		if bitShift == 0 {
			dst[i] = src[srcIdx]
		} else {
			dst[i] = (src[srcIdx] << bitShift) | (src[nextIdx] >> (8 - bitShift))
		}
	}
}
