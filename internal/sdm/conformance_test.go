package sdm

// 3GPP API conformance tests for Nudm_SDM (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_SDM

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_GetAmData_ContentType verifies that a successful
// GetAmData response uses the application/json Content-Type header
// as required by 3GPP TS 29.503.
func TestConformance_GetAmData_ContentType(t *testing.T) {
	svc := &mockService{
		getAmDataFn: func(_ context.Context, _ string) (*AccessAndMobilitySubscriptionData, error) {
			return &AccessAndMobilitySubscriptionData{
				Gpsis: []string{"msisdn-12025551234"},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/am-data", http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_GetAmData_Returns200 verifies that a successful
// GetAmData request returns HTTP 200 per TS 29.503.
func TestConformance_GetAmData_Returns200(t *testing.T) {
	svc := &mockService{
		getAmDataFn: func(_ context.Context, _ string) (*AccessAndMobilitySubscriptionData, error) {
			return &AccessAndMobilitySubscriptionData{
				Gpsis: []string{"msisdn-12025551234"},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/am-data", http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestConformance_GetAmData_NotFound_ProblemDetails verifies that when
// the subscriber is not found, a 404 response with application/problem+json
// Content-Type is returned per RFC 7807.
func TestConformance_GetAmData_NotFound_ProblemDetails(t *testing.T) {
	svc := &mockService{
		getAmDataFn: func(_ context.Context, _ string) (*AccessAndMobilitySubscriptionData, error) {
			return nil, errors.NewNotFound("subscriber not found", "USER_NOT_FOUND")
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-999990000000001/am-data", http.NoBody)
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
	if pd.Status != http.StatusNotFound {
		t.Errorf("ProblemDetails.Status: got %d, want %d", pd.Status, http.StatusNotFound)
	}
}

// TestConformance_GetDataSets_RequiresDatasetNames verifies that a GET
// request to the data sets endpoint without the required dataset-names
// query parameter returns 400 Bad Request per TS 29.503.
func TestConformance_GetDataSets_RequiresDatasetNames(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001", http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Status != http.StatusBadRequest {
		t.Errorf("ProblemDetails.Status: got %d, want %d", pd.Status, http.StatusBadRequest)
	}
}

// TestConformance_Subscribe_Returns201 verifies that creating a new SDM
// subscription via POST returns HTTP 201 per TS 29.503.
func TestConformance_Subscribe_Returns201(t *testing.T) {
	svc := &mockService{
		subscribeFn: func(_ context.Context, _ string, sub *SdmSubscription) (*SdmSubscription, error) {
			sub.SubscriptionID = "sub-001"
			return sub, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"nfInstanceId":"amf-001","callbackReference":"https://amf.example.com/callback","monitoredResourceUris":["am-data"]}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions",
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

// TestConformance_Unsubscribe_Returns204 verifies that deleting an SDM
// subscription returns HTTP 204 per TS 29.503.
func TestConformance_Unsubscribe_Returns204(t *testing.T) {
	svc := &mockService{
		unsubscribeFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions/sub-001",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_NotImplemented_Returns501 verifies that unimplemented
// endpoints return 501 Not Implemented with ProblemDetails per TS 29.503.
func TestConformance_NotImplemented_Returns501(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	paths := []string{
		"/nudm-sdm/v2/shared-data",
		"/nudm-sdm/v2/imsi-001010000000001/lcs-privacy-data",
		"/nudm-sdm/v2/imsi-001010000000001/v2x-data",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusNotImplemented {
				t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNotImplemented, w.Body.String())
			}
			assertProblemDetailsContentType(t, w)

			var pd errors.ProblemDetails
			if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
				t.Fatalf("decode ProblemDetails: %v", err)
			}
			if pd.Status != http.StatusNotImplemented {
				t.Errorf("ProblemDetails.Status: got %d, want %d", pd.Status, http.StatusNotImplemented)
			}
		})
	}
}

// TestConformance_ProblemDetails_Format verifies that ProblemDetails error
// responses contain the required fields per RFC 7807: status, title, and
// the 3GPP-specific cause field where applicable.
func TestConformance_ProblemDetails_Format(t *testing.T) {
	tests := []struct {
		name      string
		svc       *mockService
		method    string
		path      string
		body      string
		wantCode  int
		wantCause bool
	}{
		{
			name: "NotFound_HasCause",
			svc: &mockService{
				getAmDataFn: func(_ context.Context, _ string) (*AccessAndMobilitySubscriptionData, error) {
					return nil, errors.NewNotFound("subscriber not found", "USER_NOT_FOUND")
				},
			},
			method:    http.MethodGet,
			path:      "/nudm-sdm/v2/imsi-999990000000001/am-data",
			wantCode:  http.StatusNotFound,
			wantCause: true,
		},
		{
			name:     "BadRequest_MissingDatasetNames",
			svc:      &mockService{},
			method:   http.MethodGet,
			path:     "/nudm-sdm/v2/imsi-001010000000001",
			wantCode: http.StatusBadRequest,
			// BadRequest from handler sets cause via CauseMandatoryIEMissing
			wantCause: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := newTestMux(tc.svc)

			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, http.NoBody)
			}
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tc.wantCode {
				t.Errorf("status code: got %d, want %d", w.Code, tc.wantCode)
			}
			assertProblemDetailsContentType(t, w)

			var pd errors.ProblemDetails
			if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
				t.Fatalf("decode ProblemDetails: %v", err)
			}
			if pd.Status == 0 {
				t.Error("ProblemDetails.Status is missing (0)")
			}
			if pd.Title == "" {
				t.Error("ProblemDetails.Title is empty")
			}
			if tc.wantCause && pd.Cause == "" {
				t.Error("ProblemDetails.Cause is empty but expected a value")
			}
		})
	}
}

// TestConformance_Acknowledge_Returns204 verifies that PUT /am-data/sor-ack
// returns HTTP 204 when the body is valid JSON per TS 29.503.
func TestConformance_Acknowledge_Returns204(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	body := `{"provisioningTime":"2024-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-sdm/v2/imsi-001010000000001/am-data/sor-ack",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}
