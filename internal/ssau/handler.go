package ssau

// HTTP handler layer for the Nudm_SSAU service.
//
// Based on: docs/service-decomposition.md §2.7 (udm-ssau)
// Based on: docs/sbi-api-design.md §3.7 (SSAU Endpoints)
// 3GPP: TS 29.503 Nudm_SSAU — Service-Specific Authorization API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_SSAU API per TS 29.503.
const apiRoot = "/nudm-ssau/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	Authorize(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error)
	Remove(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationRemoveData) error
}

// Handler handles HTTP requests for the Nudm_SSAU API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new SSAU HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_SSAU endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.7
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	// POST /{ueIdentity}/{serviceType}/authorize
	case r.Method == http.MethodPost && matchPath(segments, "*/*/authorize"):
		h.handleAuthorize(w, r, segments[0], segments[1])

	// POST /{ueIdentity}/{serviceType}/remove
	case r.Method == http.MethodPost && matchPath(segments, "*/*/remove"):
		h.handleRemove(w, r, segments[0], segments[1])

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// handleAuthorize handles POST /{ueIdentity}/{serviceType}/authorize.
//
// Based on: docs/sbi-api-design.md §3.7
// 3GPP: TS 29.503 Nudm_SSAU — ServiceSpecificAuthorization
func (h *Handler) handleAuthorize(w http.ResponseWriter, r *http.Request, ueIdentity, serviceType string) {
	var req ServiceSpecificAuthorizationInfo
	if err := sbi.ReadJSON(r, &req); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.Authorize(r.Context(), ueIdentity, serviceType, &req)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleRemove handles POST /{ueIdentity}/{serviceType}/remove.
//
// Based on: docs/sbi-api-design.md §3.7
// 3GPP: TS 29.503 Nudm_SSAU — ServiceSpecificAuthorizationRemoval
func (h *Handler) handleRemove(w http.ResponseWriter, r *http.Request, ueIdentity, serviceType string) {
	var req ServiceSpecificAuthorizationRemoveData
	if err := sbi.ReadJSON(r, &req); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.Remove(r.Context(), ueIdentity, serviceType, &req); err != nil {
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
