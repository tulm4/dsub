package ueid

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	svcerrors "github.com/tulm4/dsub/internal/common/errors"
)

// mockRow implements pgx.Row for testing.
type mockRow struct {
	scanFunc func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

// mockDB implements the DB interface for testing.
type mockDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFunc(ctx, sql, args...)
}

// mockHSM implements HSMDecrypter for testing using SoftwareHSM.
type mockHSM struct {
	keys map[string][]byte
}

func (m *mockHSM) DeconcealMSIN(hsmKeyRef, profileType string, ephemeralPubKey, cipherText []byte) ([]byte, error) {
	privKey, ok := m.keys[hsmKeyRef]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", hsmKeyRef)
	}
	return DeconcealMSIN(profileType, privKey, ephemeralPubKey, cipherText)
}

// newTestHSM creates a test HSM with a single key.
func newTestHSM() *mockHSM {
	return &mockHSM{keys: make(map[string][]byte)}
}

// TestNewService verifies service construction.
func TestNewService(t *testing.T) {
	db := &mockDB{}
	hsm := newTestHSM()
	svc := NewService(db, hsm)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.db != db {
		t.Fatal("NewService did not set db field")
	}
}

// TestResolveSUCI verifies the ResolveSUCI method delegates to Deconceal.
func TestResolveSUCI(t *testing.T) {
	hsm := newTestHSM()
	svc := NewService(&mockDB{}, hsm)
	supi, err := svc.ResolveSUCI(context.Background(), "suci-0-001-01-0000-0-0-0123456789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if supi != "imsi-001010123456789" {
		t.Errorf("SUPI mismatch: got %q, want %q", supi, "imsi-001010123456789")
	}
}

// TestDeconceal_NilRequest tests rejection of nil request.
func TestDeconceal_NilRequest(t *testing.T) {
	svc := NewService(&mockDB{}, newTestHSM())
	_, err := svc.Deconceal(context.Background(), nil)
	assertProblemStatus(t, err, 400)
}

// TestDeconceal_EmptySUCI tests rejection of empty SUCI.
func TestDeconceal_EmptySUCI(t *testing.T) {
	svc := NewService(&mockDB{}, newTestHSM())
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{})
	assertProblemStatus(t, err, 400)
}

// TestDeconceal_InvalidSUCIFormat tests rejection of malformed SUCI.
func TestDeconceal_InvalidSUCIFormat(t *testing.T) {
	svc := NewService(&mockDB{}, newTestHSM())
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "not-a-valid-suci",
	})
	assertProblemStatus(t, err, 400)
}

// TestDeconceal_NullScheme tests that scheme_id=0 (null scheme) returns plaintext SUPI.
func TestDeconceal_NullScheme(t *testing.T) {
	svc := NewService(&mockDB{}, newTestHSM())
	resp, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "suci-0-001-01-0000-0-0-0123456789",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "imsi-001010123456789"
	if resp.Supi != expected {
		t.Errorf("SUPI mismatch: got %q, want %q", resp.Supi, expected)
	}
}

// TestDeconceal_NullScheme_InvalidSUPI tests SUPI validation for null scheme.
func TestDeconceal_NullScheme_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{}, newTestHSM())
	// EncryptedMSIN contains hex characters not valid for IMSI digits
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "suci-0-001-01-0000-0-0-abcdef",
	})
	assertProblemStatus(t, err, 400)
}

// TestDeconceal_ProfileNotFound tests handling of unknown HN key ID.
func TestDeconceal_ProfileNotFound(t *testing.T) {
	db := &mockDB{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(_ ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	}
	svc := NewService(db, newTestHSM())

	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		// scheme_id=1 (Profile A), hn_key_id=99
		Suci: "suci-0-001-01-0000-1-99-" + hex.EncodeToString(make([]byte, 50)),
	})
	assertProblemStatus(t, err, 404)
}

// TestDeconceal_ProfileA_Success tests a successful Profile A SUCI de-concealment.
func TestDeconceal_ProfileA_Success(t *testing.T) {
	// Generate a home network X25519 key pair
	hnPrivKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Generate UE ephemeral key pair
	uePrivKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate UE key: %v", err)
	}
	uePubKey := uePrivKey.PublicKey()

	// UE-side encryption
	sharedSecret, _ := uePrivKey.ECDH(hnPrivKey.PublicKey())
	encKey, icb, macKey, _ := deriveKeys(sharedSecret, uePubKey.Bytes())
	plainMSIN := []byte("0123456789")
	encryptedMSIN, _ := aesCTRDecrypt(encKey, icb, plainMSIN)
	mac := computeMAC(macKey, encryptedMSIN)

	// Build the encrypted MSIN field: ephemeral_pub_key || encrypted_msin || mac
	cipherData := append(uePubKey.Bytes(), encryptedMSIN...)
	cipherData = append(cipherData, mac...)
	encMSINHex := hex.EncodeToString(cipherData)

	// SUCI: suci-0-<MCC>-<MNC>-<routing_id>-<scheme_id>-<hn_key_id>-<encrypted_MSIN>
	suci := fmt.Sprintf("suci-0-001-01-0000-1-1-%s", encMSINHex)

	hsmKeyRef := "hsm:hn-key-1"
	hsm := &mockHSM{keys: map[string][]byte{hsmKeyRef: hnPrivKey.Bytes()}}

	db := &mockDB{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(dest ...any) error {
					*(dest[0].(*int)) = 1                               // hn_key_id
					*(dest[1].(*string)) = "A"                          // profile_type
					*(dest[2].(*[]byte)) = hnPrivKey.PublicKey().Bytes() // public_key
					*(dest[3].(*string)) = hsmKeyRef                    // hsm_key_ref
					*(dest[4].(*bool)) = true                           // is_active
					return nil
				},
			}
		},
	}

	svc := NewService(db, hsm)
	resp, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{Suci: suci})
	if err != nil {
		t.Fatalf("Deconceal failed: %v", err)
	}

	expectedSUPI := "imsi-001010123456789"
	if resp.Supi != expectedSUPI {
		t.Errorf("SUPI mismatch: got %q, want %q", resp.Supi, expectedSUPI)
	}
}

// TestDeconceal_InactiveKey tests rejection of inactive SUCI profiles.
func TestDeconceal_InactiveKey(t *testing.T) {
	db := &mockDB{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(dest ...any) error {
					*(dest[0].(*int)) = 1
					*(dest[1].(*string)) = "A"
					*(dest[2].(*[]byte)) = make([]byte, 32)
					*(dest[3].(*string)) = "hsm:inactive"
					*(dest[4].(*bool)) = false // inactive
					return nil
				},
			}
		},
	}

	svc := NewService(db, newTestHSM())
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "suci-0-001-01-0000-1-1-" + hex.EncodeToString(make([]byte, 50)),
	})
	assertProblemStatus(t, err, 404)
}

// TestDeconceal_ProfileTypeMismatch tests rejection when scheme_id doesn't match stored profile.
func TestDeconceal_ProfileTypeMismatch(t *testing.T) {
	db := &mockDB{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(dest ...any) error {
					*(dest[0].(*int)) = 1
					*(dest[1].(*string)) = "B"              // Profile B key
					*(dest[2].(*[]byte)) = make([]byte, 33)
					*(dest[3].(*string)) = "hsm:mismatch"
					*(dest[4].(*bool)) = true
					return nil
				},
			}
		},
	}

	svc := NewService(db, newTestHSM())
	// scheme_id=1 means Profile A, but key is Profile B
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "suci-0-001-01-0000-1-1-" + hex.EncodeToString(make([]byte, 50)),
	})
	assertProblemStatus(t, err, 400)
}

// TestDeconceal_DBError tests handling of database errors.
func TestDeconceal_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(_ ...any) error {
					return fmt.Errorf("connection refused")
				},
			}
		},
	}

	svc := NewService(db, newTestHSM())
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "suci-0-001-01-0000-1-1-" + hex.EncodeToString(make([]byte, 50)),
	})
	assertProblemStatus(t, err, 500)
}

// TestDeconceal_HSMFailure tests that ECIES failures return 400, not 500.
func TestDeconceal_HSMFailure(t *testing.T) {
	hsmKeyRef := "hsm:bad-key"
	hsm := &mockHSM{keys: map[string][]byte{hsmKeyRef: make([]byte, 32)}}

	db := &mockDB{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFunc: func(dest ...any) error {
					*(dest[0].(*int)) = 1
					*(dest[1].(*string)) = "A"
					*(dest[2].(*[]byte)) = make([]byte, 32)
					*(dest[3].(*string)) = hsmKeyRef
					*(dest[4].(*bool)) = true
					return nil
				},
			}
		},
	}

	svc := NewService(db, hsm)
	_, err := svc.Deconceal(context.Background(), &SuciDeconcealRequest{
		Suci: "suci-0-001-01-0000-1-1-" + hex.EncodeToString(make([]byte, 50)),
	})
	// ECIES failures should map to 400 (bad cipher text), not 500
	assertProblemStatus(t, err, 400)
}

// assertProblemStatus checks that the error is a ProblemDetails with the expected HTTP status.
func assertProblemStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with status %d, got nil", wantStatus)
	}
	pd, ok := err.(*svcerrors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != wantStatus {
		t.Errorf("ProblemDetails.Status = %d, want %d (detail: %s)", pd.Status, wantStatus, pd.Detail)
	}
}
