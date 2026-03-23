package ssau

// Service layer tests for the Nudm_SSAU service.
//
// Based on: docs/service-decomposition.md §2.7 (udm-ssau)
// Based on: docs/testing-strategy.md (unit testing patterns)

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
)

// testGPSI is a valid GPSI for use across all service tests.
const testGPSI = "msisdn-12025551234"

// testServiceType is the default service type for tests.
const testServiceType = "AF_GUIDANCE_FOR_URSP"

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

// mockRow implements pgx.Row for unit tests.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

// mockDB implements the DB interface for unit tests.
type mockDB struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, pgx.ErrNoRows
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag(""), nil
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// assertProblemStatus asserts that err is a *ProblemDetails with the given HTTP status.
func assertProblemStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != wantStatus {
		t.Errorf("expected status %d, got %d", wantStatus, pd.Status)
	}
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestServiceImplementsInterface(t *testing.T) {
	var _ ServiceInterface = (*Service)(nil)
}

// ---------------------------------------------------------------------------
// ValidateUeID tests
// ---------------------------------------------------------------------------

func TestValidateUeID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid GPSI msisdn", input: "msisdn-12025551234", wantErr: false},
		{name: "valid GPSI extid", input: "extid-user@example.com", wantErr: false},
		{name: "valid extgroupid", input: "extgroupid-group1", wantErr: false},
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
		{name: "empty extgroupid", input: "extgroupid-", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUeID(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateUeID(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateServiceType tests
// ---------------------------------------------------------------------------

func TestValidateServiceType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "AF_GUIDANCE_FOR_URSP", input: "AF_GUIDANCE_FOR_URSP", wantErr: false},
		{name: "AF_REQUESTED_QOS", input: "AF_REQUESTED_QOS", wantErr: false},
		{name: "unknown", input: "UNKNOWN_TYPE", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateServiceType(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateServiceType(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Authorize tests
// ---------------------------------------------------------------------------

func TestAuthorize_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "test-auth-id-123"
				}
				if p, ok := dest[1].(*[]byte); ok {
					*p = []byte(`{"gpsi":"msisdn-12025551234"}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)

	req := &ServiceSpecificAuthorizationInfo{
		Dnn:  "ims",
		AfID: "af-001",
	}
	result, err := svc.Authorize(context.Background(), testGPSI, testServiceType, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AuthID != "test-auth-id-123" {
		t.Errorf("expected authId test-auth-id-123, got %s", result.AuthID)
	}
	if result.AuthorizationUeID == nil {
		t.Error("expected non-nil authorizationUeId")
	}
}

func TestAuthorize_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &ServiceSpecificAuthorizationInfo{}
	_, err := svc.Authorize(context.Background(), "bad-id", testServiceType, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestAuthorize_InvalidServiceType(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &ServiceSpecificAuthorizationInfo{}
	_, err := svc.Authorize(context.Background(), testGPSI, "INVALID_TYPE", req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestAuthorize_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.Authorize(context.Background(), testGPSI, testServiceType, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestAuthorize_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection reset")
			}}
		},
	}
	svc := NewService(db)

	req := &ServiceSpecificAuthorizationInfo{}
	_, err := svc.Authorize(context.Background(), testGPSI, testServiceType, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// Remove tests
// ---------------------------------------------------------------------------

func TestRemove_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	req := &ServiceSpecificAuthorizationRemoveData{AuthID: "auth-123"}
	err := svc.Remove(context.Background(), testGPSI, testServiceType, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemove_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &ServiceSpecificAuthorizationRemoveData{AuthID: "auth-123"}
	err := svc.Remove(context.Background(), "bad-id", testServiceType, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRemove_InvalidServiceType(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &ServiceSpecificAuthorizationRemoveData{AuthID: "auth-123"}
	err := svc.Remove(context.Background(), testGPSI, "INVALID_TYPE", req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRemove_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.Remove(context.Background(), testGPSI, testServiceType, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRemove_EmptyAuthID(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &ServiceSpecificAuthorizationRemoveData{AuthID: ""}
	err := svc.Remove(context.Background(), testGPSI, testServiceType, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRemove_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	req := &ServiceSpecificAuthorizationRemoveData{AuthID: "nonexistent"}
	err := svc.Remove(context.Background(), testGPSI, testServiceType, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestRemove_DBError(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag(""), fmt.Errorf("db failure")
		},
	}
	svc := NewService(db)

	req := &ServiceSpecificAuthorizationRemoveData{AuthID: "auth-123"}
	err := svc.Remove(context.Background(), testGPSI, testServiceType, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}
