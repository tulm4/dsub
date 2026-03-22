package ueau

// Milenage algorithm tests using 3GPP TS 35.207 test vectors.
//
// 3GPP: TS 35.207 — Test Data for the MILENAGE Algorithm Set
// Test Set 1 from TS 35.207 §4.3

import (
	"encoding/hex"
	"testing"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode %q: %v", s, err)
	}
	return b
}

// TestMilenageTestSet1 verifies Milenage against TS 35.207 Test Set 1.
func TestMilenageTestSet1(t *testing.T) {
	k := mustDecode(t, "465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustDecode(t, "cdc202d5123e20f62b6d676ac72cb318")
	rand := mustDecode(t, "23553cbe9637a89d218ae64dae47bf35")
	sqn := mustDecode(t, "ff9bb4d0b607")
	amf := mustDecode(t, "b9b9")

	// Expected outputs from TS 35.207 §4.3 Test Set 1
	expectedF1 := "4a9ffac354dfafb3"  // MAC-A
	expectedF2 := "a54211d5e3ba50bf"  // XRES
	expectedF3 := "b40ba9a3c58b2a05bbf0d987b21bf8cb" // CK
	expectedF4 := "f769bcd751044604127672711c6d3441" // IK
	expectedF5 := "aa689c648370"      // AK

	// First compute OPc from K and OP
	opc, err := ComputeOPc(k, op)
	if err != nil {
		t.Fatalf("ComputeOPc: %v", err)
	}

	// Verify OPc matches expected value from TS 35.207
	expectedOPc := "cd63cb71954a9f4e48a5994e37a02baf"
	if got := hex.EncodeToString(opc); got != expectedOPc {
		t.Errorf("OPc mismatch:\n  got:  %s\n  want: %s", got, expectedOPc)
	}

	// Generate auth vector
	av, err := GenerateAuthVector(k, opc, sqn, amf, rand)
	if err != nil {
		t.Fatalf("GenerateAuthVector: %v", err)
	}

	// Verify f1 (MAC-A)
	if got := hex.EncodeToString(av.MAC); got != expectedF1 {
		t.Errorf("f1 (MAC-A) mismatch:\n  got:  %s\n  want: %s", got, expectedF1)
	}

	// Verify f2 (XRES)
	if got := hex.EncodeToString(av.XRES); got != expectedF2 {
		t.Errorf("f2 (XRES) mismatch:\n  got:  %s\n  want: %s", got, expectedF2)
	}

	// Verify f3 (CK)
	if got := hex.EncodeToString(av.CK); got != expectedF3 {
		t.Errorf("f3 (CK) mismatch:\n  got:  %s\n  want: %s", got, expectedF3)
	}

	// Verify f4 (IK)
	if got := hex.EncodeToString(av.IK); got != expectedF4 {
		t.Errorf("f4 (IK) mismatch:\n  got:  %s\n  want: %s", got, expectedF4)
	}

	// Verify f5 (AK)
	if got := hex.EncodeToString(av.AK); got != expectedF5 {
		t.Errorf("f5 (AK) mismatch:\n  got:  %s\n  want: %s", got, expectedF5)
	}
}

// TestComputeOPc verifies OPc derivation.
func TestComputeOPc(t *testing.T) {
	k := mustDecode(t, "465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustDecode(t, "cdc202d5123e20f62b6d676ac72cb318")
	expected := "cd63cb71954a9f4e48a5994e37a02baf"

	opc, err := ComputeOPc(k, op)
	if err != nil {
		t.Fatalf("ComputeOPc: %v", err)
	}

	if got := hex.EncodeToString(opc); got != expected {
		t.Errorf("OPc mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

// TestComputeOPcInvalidInputs tests error handling for bad inputs.
func TestComputeOPcInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		k    []byte
		op   []byte
	}{
		{"short K", make([]byte, 8), make([]byte, 16)},
		{"long K", make([]byte, 32), make([]byte, 16)},
		{"short OP", make([]byte, 16), make([]byte, 8)},
		{"long OP", make([]byte, 16), make([]byte, 32)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ComputeOPc(tc.k, tc.op)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestGenerateAuthVectorInvalidInputs tests error handling for bad inputs.
func TestGenerateAuthVectorInvalidInputs(t *testing.T) {
	valid16 := make([]byte, 16)
	valid6 := make([]byte, 6)
	valid2 := make([]byte, 2)

	tests := []struct {
		name string
		k    []byte
		opc  []byte
		sqn  []byte
		amf  []byte
		rand []byte
	}{
		{"short K", make([]byte, 8), valid16, valid6, valid2, valid16},
		{"short OPc", valid16, make([]byte, 8), valid6, valid2, valid16},
		{"short SQN", valid16, valid16, make([]byte, 4), valid2, valid16},
		{"short AMF", valid16, valid16, valid6, make([]byte, 1), valid16},
		{"short RAND", valid16, valid16, valid6, valid2, make([]byte, 8)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GenerateAuthVector(tc.k, tc.opc, tc.sqn, tc.amf, tc.rand)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestRotateLeft verifies the 128-bit cyclic left rotation.
func TestRotateLeft(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		n        int
		expected []byte
	}{
		{
			"rotate 0 bits",
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			0,
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		},
		{
			"rotate 8 bits (1 byte)",
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			8,
			[]byte{0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x01},
		},
		{
			"rotate 64 bits (8 bytes)",
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			64,
			[]byte{0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := make([]byte, 16)
			rotateLeft(dst, tc.input, tc.n)
			if hex.EncodeToString(dst) != hex.EncodeToString(tc.expected) {
				t.Errorf("rotate(%d) mismatch:\n  got:  %s\n  want: %s",
					tc.n, hex.EncodeToString(dst), hex.EncodeToString(tc.expected))
			}
		})
	}
}

// TestAUTNConstruction verifies that AUTN = (SQN ⊕ AK) || AMF || MAC-A.
func TestAUTNConstruction(t *testing.T) {
	k := mustDecode(t, "465b5ce8b199b49faa5f0a2ee238a6bc")
	opc := mustDecode(t, "cd63cb71954a9f4e48a5994e37a02baf")
	sqn := mustDecode(t, "ff9bb4d0b607")
	amf := mustDecode(t, "b9b9")
	rand := mustDecode(t, "23553cbe9637a89d218ae64dae47bf35")

	av, err := GenerateAuthVector(k, opc, sqn, amf, rand)
	if err != nil {
		t.Fatalf("GenerateAuthVector: %v", err)
	}

	// Verify AUTN structure: first 6 bytes = SQN ⊕ AK, next 2 = AMF, last 8 = MAC-A
	for i := 0; i < 6; i++ {
		expected := sqn[i] ^ av.AK[i]
		if av.AUTN[i] != expected {
			t.Errorf("AUTN[%d]: got %02x, want %02x (SQN⊕AK)", i, av.AUTN[i], expected)
		}
	}

	if av.AUTN[6] != amf[0] || av.AUTN[7] != amf[1] {
		t.Errorf("AUTN AMF: got %02x%02x, want %02x%02x", av.AUTN[6], av.AUTN[7], amf[0], amf[1])
	}

	for i := 0; i < 8; i++ {
		if av.AUTN[8+i] != av.MAC[i] {
			t.Errorf("AUTN MAC[%d]: got %02x, want %02x", i, av.AUTN[8+i], av.MAC[i])
		}
	}
}
