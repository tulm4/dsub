package rsds

// HTTP handler layer for the Nudm_RSDS service.
//
// Based on: docs/service-decomposition.md §2.9 (udm-rsds)
// Based on: docs/sbi-api-design.md §3.9 (RSDS Endpoints)
// 3GPP: TS 29.503 Nudm_RSDS — Report SMS Delivery Status API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_RSDS API per TS 29.503.
const apiRoot = "/nudm-rsds/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	ReportSMDeliveryStatus(ctx context.Context, ueIdentity string, req *SmDeliveryStatus) error
}

// Handler handles HTTP requests for the Nudm_RSDS API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new RSDS HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_RSDS endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.9
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	// POST /{ueIdentity}/sm-delivery-status
	case r.Method == http.MethodPost && matchPath(segments, "*/sm-delivery-status"):
		h.handleReportSMDeliveryStatus(w, r, segments[0])

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// handleReportSMDeliveryStatus handles POST /{ueIdentity}/sm-delivery-status.
//
// Based on: docs/sbi-api-design.md §3.9
// 3GPP: TS 29.503 Nudm_RSDS — ReportSMDeliveryStatus
func (h *Handler) handleReportSMDeliveryStatus(w http.ResponseWriter, r *http.Request, ueIdentity string) {
	var req SmDeliveryStatus
	if err := sbi.ReadJSON(r, &req); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.ReportSMDeliveryStatus(r.Context(), ueIdentity, &req); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeSvcError writes a ProblemDetails error response. If the error is already
// a *ProblemDetails, it is written directly; otherwise a 500 is returned.
func writeSvcError(w http.ResponseWriter, err error) {
	if pd, ok := err.(*errors.ProblemDetails); ok {
		errors.WriteProblemDetails(w, pd)
		return
	}
	errors.WriteProblemDetails(w, errors.NewInternalError(err.Error()))
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	var segments []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

// matchPath checks if path segments match a pattern where * matches any single segment.
func matchPath(segments []string, pattern string) bool {
	patternSegments := splitPath(pattern)
	if len(segments) != len(patternSegments) {
		return false
	}
	for i, p := range patternSegments {
		if p == "*" {
			continue
		}
		if segments[i] != p {
			return false
		}
	}
	return true
}
