package pp

// HTTP handler layer for the Nudm_PP service.
//
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
// Based on: docs/sbi-api-design.md §3.5 (PP Endpoints)
// 3GPP: TS 29.503 Nudm_PP — Parameter Provisioning service API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_PP API per TS 29.503.
const apiRoot = "/nudm-pp/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	GetPPData(ctx context.Context, ueID string) (*PpData, error)
	UpdatePPData(ctx context.Context, ueID string, patch *PpData) (*PpData, error)
}

// Handler handles HTTP requests for the Nudm_PP API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new PP HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_PP endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.5
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiRoot+"/", h.route)
	mux.HandleFunc("PATCH "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	// 5G VN Group and MBS Group operations — not yet implemented.
	for _, seg := range segments {
		if seg == "5g-vn-groups" || seg == "mbs-group-membership" {
			h.handleNotImplemented(w, seg)
			return
		}
	}

	switch {
	// GET /{ueId}/pp-data
	case r.Method == http.MethodGet && matchPath(segments, "*/pp-data"):
		h.handleGetPPData(w, r, segments[0])

	// PATCH /{ueId}/pp-data
	case r.Method == http.MethodPatch && matchPath(segments, "*/pp-data"):
		h.handleUpdatePPData(w, r, segments[0])

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// handleGetPPData handles GET /{ueId}/pp-data.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — GetPPData
func (h *Handler) handleGetPPData(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetPPData(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUpdatePPData handles PATCH /{ueId}/pp-data.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — UpdatePPData
func (h *Handler) handleUpdatePPData(w http.ResponseWriter, r *http.Request, ueID string) {
	var patch PpData
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.UpdatePPData(r.Context(), ueID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleNotImplemented returns 501 for endpoints not yet implemented.
func (h *Handler) handleNotImplemented(w http.ResponseWriter, operation string) {
	errors.WriteProblemDetails(w, errors.NewNotImplemented(operation+" not yet implemented"))
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
