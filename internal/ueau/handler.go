package ueau

// HTTP handler layer for the Nudm_UEAU service.
//
// Based on: docs/service-decomposition.md §2.1 (udm-ueau)
// Based on: docs/sbi-api-design.md §3.1 (UEAU Endpoints)
// 3GPP: TS 29.503 Nudm_UEAU — UE Authentication service API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_UEAU API per TS 29.503.
const apiRoot = "/nudm-ueau/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	GenerateAuthData(ctx context.Context, supiOrSuci string, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error)
	ConfirmAuth(ctx context.Context, supi string, event *AuthEvent) (*AuthEvent, error)
	DeleteAuthEvent(ctx context.Context, supi, authEventID string) error
}

// Handler handles HTTP requests for the Nudm_UEAU API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new UEAU HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_UEAU endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.1
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// POST /{supiOrSuci}/security-information/generate-auth-data
	mux.HandleFunc("POST "+apiRoot+"/", h.route)
	// Handle all methods on the API root for proper routing
	mux.HandleFunc("GET "+apiRoot+"/", h.route)
	mux.HandleFunc("PUT "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	// Strip the API root prefix to get the relative path
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	case r.Method == http.MethodPost && matchPath(segments, "*/security-information/generate-auth-data"):
		h.HandleGenerateAuthData(w, r, segments[0])

	case r.Method == http.MethodGet && matchPath(segments, "*/security-information-rg"):
		h.HandleGetRgAuthData(w, r, segments[0])

	case r.Method == http.MethodPost && matchPath(segments, "*/auth-events"):
		h.HandleConfirmAuth(w, r, segments[0])

	case r.Method == http.MethodPut && matchPath(segments, "*/auth-events/*"):
		h.HandleDeleteAuth(w, r, segments[0], segments[2])

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// HandleGenerateAuthData handles POST /{supiOrSuci}/security-information/generate-auth-data.
//
// Based on: docs/sbi-api-design.md §3.1
// 3GPP: TS 29.503 Nudm_UEAU — GenerateAuthData
// 3GPP: TS 33.501 §6.1.3 — 5G-AKA authentication procedure
func (h *Handler) HandleGenerateAuthData(w http.ResponseWriter, r *http.Request, supiOrSuci string) {
	var req AuthenticationInfoRequest
	if err := sbi.ReadJSON(r, &req); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.GenerateAuthData(r.Context(), supiOrSuci, &req)
	if err != nil {
		writeSvcError(w, err)
		return
	}

	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// HandleGetRgAuthData handles GET /{supiOrSuci}/security-information-rg.
//
// Based on: docs/sbi-api-design.md §3.1
// 3GPP: TS 29.503 Nudm_UEAU — GetRgAuthData
func (h *Handler) HandleGetRgAuthData(w http.ResponseWriter, _ *http.Request, _ string) {
	// RG auth data retrieval — not yet implemented in Phase 3
	errors.WriteProblemDetails(w, errors.NewNotImplemented("GetRgAuthData not yet implemented"))
}

// HandleConfirmAuth handles POST /{supi}/auth-events.
//
// Based on: docs/sbi-api-design.md §3.1
// 3GPP: TS 29.503 Nudm_UEAU — ConfirmAuth
func (h *Handler) HandleConfirmAuth(w http.ResponseWriter, r *http.Request, supi string) {
	var event AuthEvent
	if err := sbi.ReadJSON(r, &event); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.ConfirmAuth(r.Context(), supi, &event)
	if err != nil {
		writeSvcError(w, err)
		return
	}

	_ = sbi.WriteJSON(w, http.StatusCreated, result)
}

// HandleDeleteAuth handles PUT /{supi}/auth-events/{authEventId}.
//
// Based on: docs/sbi-api-design.md §3.1
// 3GPP: TS 29.503 Nudm_UEAU — DeleteAuth
func (h *Handler) HandleDeleteAuth(w http.ResponseWriter, r *http.Request, supi, authEventID string) {
	// Per 3GPP TS 29.503, the PUT method with authRemovalInd=true deletes the event.
	// We also accept the request body as an AuthEvent for the update case.
	var event AuthEvent
	if err := sbi.ReadJSON(r, &event); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.DeleteAuthEvent(r.Context(), supi, authEventID); err != nil {
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
