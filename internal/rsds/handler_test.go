package rsds

// HTTP handler tests for the Nudm_RSDS service.
//
// Based on: docs/sbi-api-design.md §3.9 (RSDS Endpoints)
// Based on: docs/sbi-api-design.md §7 (Error Handling)

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// handlerMockService implements ServiceInterface for handler tests.
type handlerMockService struct {
	reportSMDeliveryStatusFn func(ctx context.Context, ueIdentity string, req *SmDeliveryStatus) error
}

func (m *handlerMockService) ReportSMDeliveryStatus(ctx context.Context, ueIdentity string, req *SmDeliveryStatus) error {
	if m.reportSMDeliveryStatusFn != nil {
		return m.reportSMDeliveryStatusFn(ctx, ueIdentity, req)
	}
	return errors.NewNotImplemented("not implemented")
}

// newTestMux creates an http.ServeMux wired to the given mock service.
func newTestMux(svc *handlerMockService) *http.ServeMux {
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

// assertProblemDetailsContentType verifies the Content-Type is application/problem+json.
func assertProblemDetailsContentType(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	ct := w.Header().Get("Content-Type")
	expected := "application/problem+json"
	if ct != expected {
		t.Errorf("Content-Type: got %q, want %q", ct, expected)
	}
}

// --- ReportSMDeliveryStatus Tests ---

func TestHandleReportSMDeliveryStatus_Success(t *testing.T) {
	svc := &handlerMockService{
		reportSMDeliveryStatusFn: func(_ context.Context, _ string, _ *SmDeliveryStatus) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	body := `{"gpsi":"msisdn-12025551234","smStatusReport":{"status":"delivered"}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReportSMDeliveryStatus_InvalidBody(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleReportSMDeliveryStatus_ServiceError(t *testing.T) {
	svc := &handlerMockService{
		reportSMDeliveryStatusFn: func(_ context.Context, _ string, _ *SmDeliveryStatus) error {
			return errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	body := `{"gpsi":"msisdn-12025551234","smStatusReport":{"status":"failed"}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleReportSMDeliveryStatus_BadRequest(t *testing.T) {
	svc := &handlerMockService{
		reportSMDeliveryStatusFn: func(_ context.Context, _ string, _ *SmDeliveryStatus) error {
			return errors.NewBadRequest("gpsi is required", errors.CauseMandatoryIEMissing)
		},
	}
	mux := newTestMux(svc)

	body := `{"gpsi":"","smStatusReport":{"status":"delivered"}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- Route dispatch tests ---

func TestRoute_UnknownEndpoint(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/nonexistent",
		http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRoute_UnsupportedMethod(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// GET is not registered on the mux, so Go's default mux returns 405 or 404
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 or 405, got %d: %s", w.Code, w.Body.String())
	}
}
