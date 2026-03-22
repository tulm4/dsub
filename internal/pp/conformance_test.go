package pp

// 3GPP API conformance tests for Nudm_PP (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_PP

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// assertProblemDetailsContentType is defined in handler_test.go

// TestConformance_GetPPData_Returns200 verifies that GET on pp-data
// returns HTTP 200 per TS 29.503.
func TestConformance_GetPPData_Returns200(t *testing.T) {
	dlCount := 5
	svc := &mockService{
		getPPDataFn: func(_ context.Context, _ string) (*PpData, error) {
			return &PpData{
				SupportedFeatures: "A1",
				PpDlPacketCount:   &dlCount,
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/imsi-001010000000001/pp-data",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_GetPPData_NotFound_Returns404 verifies that when no PP
// data exists, a 404 with application/problem+json is returned per RFC 7807.
func TestConformance_GetPPData_NotFound_Returns404(t *testing.T) {
	svc := &mockService{
		getPPDataFn: func(_ context.Context, _ string) (*PpData, error) {
			return nil, errors.NewNotFound("pp data not found", errors.CauseDataNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/imsi-999990000000001/pp-data",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause == "" {
		t.Error("ProblemDetails.Cause is empty but expected DATA_NOT_FOUND")
	}
}

// TestConformance_UpdatePPData_Returns200 verifies that PATCH on pp-data
// returns HTTP 200 per TS 29.503.
func TestConformance_UpdatePPData_Returns200(t *testing.T) {
	svc := &mockService{
		updatePPDataFn: func(_ context.Context, _ string, patch *PpData) (*PpData, error) {
			return patch, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"supportedFeatures":"A1"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-pp/v1/imsi-001010000000001/pp-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_Create5GVnGroup_Returns201 verifies that creating a new
// 5G VN group returns HTTP 201 per TS 29.503.
func TestConformance_Create5GVnGroup_Returns201(t *testing.T) {
	svc := &mockService{
		create5GVnGroupFn: func(_ context.Context, _ string, cfg *VnGroupConfiguration) (*VnGroupConfiguration, bool, error) {
			return cfg, true, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"internet","members":["imsi-001010000000001"]}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-pp/v1/5g-vn-groups/group-001",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_Delete5GVnGroup_Returns204 verifies that deleting a
// 5G VN group returns HTTP 204 per TS 29.503.
func TestConformance_Delete5GVnGroup_Returns204(t *testing.T) {
	svc := &mockService{
		delete5GVnGroupFn: func(_ context.Context, _ string) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-pp/v1/5g-vn-groups/group-001",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}
