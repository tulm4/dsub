package ueau

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	svcerrors "github.com/tulm4/dsub/internal/common/errors"
)

// mockRow implements pgx.Row for testing.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

// mockDB implements the DB interface for testing.
type mockDB struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

// scanString safely assigns a string to a scan destination.
func scanString(dest any, val string) {
	if p, ok := dest.(*string); ok {
		*p = val
	}
}

// scanBytes safely assigns a byte slice to a scan destination.
func scanBytes(dest any, val []byte) {
	if p, ok := dest.(*[]byte); ok {
		*p = val
	}
}

// validAuthCredentialsMockRow returns a mockRow that populates valid 5G_AKA auth credentials.
func validAuthCredentialsMockRow() *mockRow {
	return &mockRow{
		scanFn: func(dest ...any) error {
			scanString(dest[0], "imsi-001010000000001")
			scanString(dest[1], "5G_AKA")
			scanBytes(dest[2], bytes.Repeat([]byte{0x01}, 16))
			scanBytes(dest[3], bytes.Repeat([]byte{0x02}, 16))
			scanString(dest[4], "000000000001")
			scanString(dest[5], "8000")
			return nil
		},
	}
}

// successRow returns a mockRow that scans a single string value.
func successRow(val string) *mockRow {
	return &mockRow{
		scanFn: func(dest ...any) error {
			scanString(dest[0], val)
			return nil
		},
	}
}

// --- TestNewService ---

func TestNewService(t *testing.T) {
	db := &mockDB{}
	svc := NewService(db)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.db != db {
		t.Error("NewService did not set db field correctly")
	}
}

// --- TestIncrementSQN ---

func TestIncrementSQN(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "zero to one",
			input: []byte{0, 0, 0, 0, 0, 0},
			want:  []byte{0, 0, 0, 0, 0, 1},
		},
		{
			name:  "one to two",
			input: []byte{0, 0, 0, 0, 0, 1},
			want:  []byte{0, 0, 0, 0, 0, 2},
		},
		{
			name:  "byte boundary carry",
			input: []byte{0, 0, 0, 0, 0, 0xFF},
			want:  []byte{0, 0, 0, 0, 1, 0},
		},
		{
			name:  "multi-byte carry",
			input: []byte{0, 0, 0, 0xFF, 0xFF, 0xFF},
			want:  []byte{0, 0, 1, 0, 0, 0},
		},
		{
			name:  "max minus one",
			input: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE},
			want:  []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			name:  "48-bit overflow wraps to zero",
			input: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			want:  []byte{0, 0, 0, 0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := incrementSQN(tt.input)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("incrementSQN(%x) = %x, want %x", tt.input, got, tt.want)
			}
		})
	}
}

// --- GenerateAuthData tests ---

func TestGenerateAuthData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "authentication_data") && strings.Contains(sql, "SELECT") {
				return validAuthCredentialsMockRow()
			}
			return successRow("imsi-001010000000001")
		},
	}
	svc := NewService(db)

	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	result, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.AuthType != "5G_AKA" {
		t.Errorf("AuthType = %q, want %q", result.AuthType, "5G_AKA")
	}
	if result.Supi != "imsi-001010000000001" {
		t.Errorf("Supi = %q, want %q", result.Supi, "imsi-001010000000001")
	}

	av := result.AuthenticationVector
	if av == nil {
		t.Fatal("AuthenticationVector is nil")
	}
	if av.AvType != "5G_HE_AKA" {
		t.Errorf("AvType = %q, want %q", av.AvType, "5G_HE_AKA")
	}
	// RAND = 16 bytes → 32 hex chars
	if len(av.Rand) != 32 {
		t.Errorf("Rand hex length = %d, want 32", len(av.Rand))
	}
	// AUTN = 16 bytes → 32 hex chars
	if len(av.Autn) != 32 {
		t.Errorf("Autn hex length = %d, want 32", len(av.Autn))
	}
	// XRES* = 16 bytes → 32 hex chars
	if len(av.XresStar) != 32 {
		t.Errorf("XresStar hex length = %d, want 32", len(av.XresStar))
	}
	// Kausf = 32 bytes → 64 hex chars
	if len(av.Kausf) != 64 {
		t.Errorf("Kausf hex length = %d, want 64", len(av.Kausf))
	}
	// All fields must be valid hex.
	for _, f := range []struct{ name, val string }{
		{"Rand", av.Rand},
		{"Autn", av.Autn},
		{"XresStar", av.XresStar},
		{"Kausf", av.Kausf},
	} {
		if _, decErr := hex.DecodeString(f.val); decErr != nil {
			t.Errorf("%s is not valid hex: %v", f.name, decErr)
		}
	}
}

func TestGenerateAuthData_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.GenerateAuthData(context.Background(), "bad-supi", req)
	assertProblemStatus(t, err, 400)
}

func TestGenerateAuthData_MissingSNName(t *testing.T) {
	svc := NewService(&mockDB{})
	req := &AuthenticationInfoRequest{ServingNetworkName: ""}
	_, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", req)
	assertProblemStatus(t, err, 400)
}

func TestGenerateAuthData_SUCI(t *testing.T) {
	svc := NewService(&mockDB{})
	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.GenerateAuthData(context.Background(), "suci-0-001-01-0-0-0-001010000000001", req)
	assertProblemStatus(t, err, 501)
}

func TestGenerateAuthData_UserNotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "authentication_data") && strings.Contains(sql, "SELECT") {
				return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			}
			t.Fatal("unexpected query after auth credentials lookup should have failed")
			return &mockRow{}
		},
	}
	svc := NewService(db)
	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", req)
	assertProblemStatus(t, err, 404)
}

func TestGenerateAuthData_UnsupportedAuth(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					scanString(dest[0], "imsi-001010000000001")
					scanString(dest[1], "UNSUPPORTED_METHOD")
					scanBytes(dest[2], bytes.Repeat([]byte{0x01}, 16))
					scanBytes(dest[3], bytes.Repeat([]byte{0x02}, 16))
					scanString(dest[4], "000000000001")
					scanString(dest[5], "8000")
					return nil
				},
			}
		},
	}
	svc := NewService(db)
	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", req)
	assertProblemStatus(t, err, 400)
}

func TestGenerateAuthData_NilRequest(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", nil)
	assertProblemStatus(t, err, 400)
}

func TestGenerateAuthData_InvalidSQN(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					scanString(dest[0], "imsi-001010000000001")
					scanString(dest[1], "5G_AKA")
					scanBytes(dest[2], bytes.Repeat([]byte{0x01}, 16))
					scanBytes(dest[3], bytes.Repeat([]byte{0x02}, 16))
					scanString(dest[4], "ZZZZ") // invalid hex
					scanString(dest[5], "8000")
					return nil
				},
			}
		},
	}
	svc := NewService(db)
	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", req)
	assertProblemStatus(t, err, 500)
}

func TestGenerateAuthData_InvalidAMF(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					scanString(dest[0], "imsi-001010000000001")
					scanString(dest[1], "5G_AKA")
					scanBytes(dest[2], bytes.Repeat([]byte{0x01}, 16))
					scanBytes(dest[3], bytes.Repeat([]byte{0x02}, 16))
					scanString(dest[4], "000000000001")
					scanString(dest[5], "ZZZZ") // invalid hex
					return nil
				},
			}
		},
	}
	svc := NewService(db)
	req := &AuthenticationInfoRequest{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.GenerateAuthData(context.Background(), "imsi-001010000000001", req)
	assertProblemStatus(t, err, 500)
}

// --- ConfirmAuth tests ---

func TestConfirmAuth_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return successRow("imsi-001010000000001")
		},
	}
	svc := NewService(db)
	event := &AuthEvent{
		NfInstanceID:       "nf-001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Success:            true,
		TimeStamp:          "2024-01-01T00:00:00Z",
	}
	result, err := svc.ConfirmAuth(context.Background(), "imsi-001010000000001", event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.NfInstanceID != "nf-001" {
		t.Errorf("NfInstanceID = %q, want %q", result.NfInstanceID, "nf-001")
	}
	if result.AuthType != "5G_AKA" {
		t.Errorf("AuthType = %q, want %q", result.AuthType, "5G_AKA")
	}
}

func TestConfirmAuth_NilEvent(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.ConfirmAuth(context.Background(), "imsi-001010000000001", nil)
	assertProblemStatus(t, err, 400)
}

func TestConfirmAuth_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	event := &AuthEvent{
		NfInstanceID:       "nf-001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	_, err := svc.ConfirmAuth(context.Background(), "bad-supi", event)
	assertProblemStatus(t, err, 400)
}

func TestConfirmAuth_MissingFields(t *testing.T) {
	tests := []struct {
		name  string
		event *AuthEvent
	}{
		{
			name: "empty nfInstanceId",
			event: &AuthEvent{
				NfInstanceID:       "",
				AuthType:           "5G_AKA",
				ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
			},
		},
		{
			name: "empty authType",
			event: &AuthEvent{
				NfInstanceID:       "nf-001",
				AuthType:           "",
				ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
			},
		},
		{
			name: "empty servingNetworkName",
			event: &AuthEvent{
				NfInstanceID:       "nf-001",
				AuthType:           "5G_AKA",
				ServingNetworkName: "",
			},
		},
	}

	svc := NewService(&mockDB{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ConfirmAuth(context.Background(), "imsi-001010000000001", tt.event)
			assertProblemStatus(t, err, 400)
		})
	}
}

func TestConfirmAuth_SetsTimestamp(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return successRow("imsi-001010000000001")
		},
	}
	svc := NewService(db)
	event := &AuthEvent{
		NfInstanceID:       "nf-001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Success:            true,
		TimeStamp:          "",
	}
	result, err := svc.ConfirmAuth(context.Background(), "imsi-001010000000001", event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TimeStamp == "" {
		t.Error("TimeStamp was not auto-set when empty")
	}
}

// --- DeleteAuthEvent tests ---

func TestDeleteAuthEvent_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return successRow("imsi-001010000000001")
		},
	}
	svc := NewService(db)
	if err := svc.DeleteAuthEvent(context.Background(), "imsi-001010000000001", "event-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteAuthEvent_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	err := svc.DeleteAuthEvent(context.Background(), "bad-supi", "event-001")
	assertProblemStatus(t, err, 400)
}

func TestDeleteAuthEvent_EmptyEventID(t *testing.T) {
	svc := NewService(&mockDB{})
	err := svc.DeleteAuthEvent(context.Background(), "imsi-001010000000001", "")
	assertProblemStatus(t, err, 400)
}

func TestDeleteAuthEvent_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	err := svc.DeleteAuthEvent(context.Background(), "imsi-001010000000001", "event-001")
	assertProblemStatus(t, err, 404)
}

// assertProblemStatus verifies the error is a *ProblemDetails with the expected HTTP status.
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
