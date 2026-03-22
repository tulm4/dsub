package mt

// HTTP handler layer for the Nudm_MT service.
//
// Based on: docs/service-decomposition.md §2.6 (udm-mt)
// Based on: docs/sbi-api-design.md §3.6 (MT Endpoints)
// 3GPP: TS 29.503 Nudm_MT — Mobile Terminated service API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_MT API per TS 29.503.
const apiRoot = "/nudm-mt/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	QueryUeInfo(ctx context.Context, supi string) (*UeInfo, error)
	ProvideLocationInfo(ctx context.Context, supi string, req *LocationInfoRequest) (*LocationInfoResult, error)
}

// Handler handles HTTP requests for the Nudm_MT API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new MT HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_MT endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.6
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiRoot+"/", h.route)
	mux.HandleFunc("POST "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	// GET /{supi} — QueryUeInfo
	case r.Method == http.MethodGet && len(segments) == 1:
		h.handleQueryUeInfo(w, r, segments[0])

	// POST /{supi}/loc-info/provide-loc-info — ProvideLocationInfo
	case r.Method == http.MethodPost && matchPath(segments, "*/loc-info/provide-loc-info"):
		h.handleProvideLocationInfo(w, r, segments[0])

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// handleQueryUeInfo handles GET /{supi}.
//
// Based on: docs/sbi-api-design.md §3.6
// 3GPP: TS 29.503 Nudm_MT — QueryUeInfo
func (h *Handler) handleQueryUeInfo(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.QueryUeInfo(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleProvideLocationInfo handles POST /{supi}/loc-info/provide-loc-info.
//
// Based on: docs/sbi-api-design.md §3.6
// 3GPP: TS 29.503 Nudm_MT — ProvideLocationInfo
func (h *Handler) handleProvideLocationInfo(w http.ResponseWriter, r *http.Request, supi string) {
	var req LocationInfoRequest
	if err := sbi.ReadJSON(r, &req); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.ProvideLocationInfo(r.Context(), supi, &req)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
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
