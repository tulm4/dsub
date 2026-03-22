package ee

// HTTP handler layer for the Nudm_EE service.
//
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
// Based on: docs/sbi-api-design.md §3.4 (EE Endpoints)
// 3GPP: TS 29.503 Nudm_EE — Event Exposure service API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_EE API per TS 29.503.
const apiRoot = "/nudm-ee/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	CreateSubscription(ctx context.Context, ueIdentity string, sub *EeSubscription) (*CreatedEeSubscription, error)
	UpdateSubscription(ctx context.Context, ueIdentity, subscriptionID string, patch *PatchEeSubscription) (*EeSubscription, error)
	DeleteSubscription(ctx context.Context, ueIdentity, subscriptionID string) error
}

// Handler handles HTTP requests for the Nudm_EE API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new EE HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_EE endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.4
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST "+apiRoot+"/", h.route)
	mux.HandleFunc("PATCH "+apiRoot+"/", h.route)
	mux.HandleFunc("DELETE "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	// POST /{ueIdentity}/ee-subscriptions
	case r.Method == http.MethodPost && matchPath(segments, "*/ee-subscriptions"):
		h.handleCreateSubscription(w, r, segments[0])

	// PATCH /{ueIdentity}/ee-subscriptions/{subscriptionId}
	case r.Method == http.MethodPatch && matchPath(segments, "*/ee-subscriptions/*"):
		h.handleUpdateSubscription(w, r, segments[0], segments[2])

	// DELETE /{ueIdentity}/ee-subscriptions/{subscriptionId}
	case r.Method == http.MethodDelete && matchPath(segments, "*/ee-subscriptions/*"):
		h.handleDeleteSubscription(w, r, segments[0], segments[2])

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// handleCreateSubscription handles POST /{ueIdentity}/ee-subscriptions.
//
// Based on: docs/sbi-api-design.md §3.4
// 3GPP: TS 29.503 Nudm_EE — CreateEeSubscription
func (h *Handler) handleCreateSubscription(w http.ResponseWriter, r *http.Request, ueIdentity string) {
	var sub EeSubscription
	if err := sbi.ReadJSON(r, &sub); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.CreateSubscription(r.Context(), ueIdentity, &sub)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusCreated, result)
}

// handleUpdateSubscription handles PATCH /{ueIdentity}/ee-subscriptions/{subscriptionId}.
//
// Based on: docs/sbi-api-design.md §3.4
// 3GPP: TS 29.503 Nudm_EE — UpdateEeSubscription
func (h *Handler) handleUpdateSubscription(w http.ResponseWriter, r *http.Request, ueIdentity, subscriptionID string) {
	var patch PatchEeSubscription
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.UpdateSubscription(r.Context(), ueIdentity, subscriptionID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDeleteSubscription handles DELETE /{ueIdentity}/ee-subscriptions/{subscriptionId}.
//
// Based on: docs/sbi-api-design.md §3.4
// 3GPP: TS 29.503 Nudm_EE — DeleteEeSubscription
func (h *Handler) handleDeleteSubscription(w http.ResponseWriter, r *http.Request, ueIdentity, subscriptionID string) {
	if err := h.svc.DeleteSubscription(r.Context(), ueIdentity, subscriptionID); err != nil {
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
