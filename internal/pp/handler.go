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

	Create5GVnGroup(ctx context.Context, extGroupID string, cfg *VnGroupConfiguration) (*VnGroupConfiguration, bool, error)
	Get5GVnGroup(ctx context.Context, extGroupID string) (*VnGroupConfiguration, error)
	Modify5GVnGroup(ctx context.Context, extGroupID string, patch *VnGroupConfiguration) (*VnGroupConfiguration, error)
	Delete5GVnGroup(ctx context.Context, extGroupID string) error

	CreateMbsGroupMembership(ctx context.Context, extGroupID string, memb *MbsGroupMemb) (*MbsGroupMemb, bool, error)
	GetMbsGroupMembership(ctx context.Context, extGroupID string) (*MbsGroupMemb, error)
	ModifyMbsGroupMembership(ctx context.Context, extGroupID string, patch *MbsGroupMemb) (*MbsGroupMemb, error)
	DeleteMbsGroupMembership(ctx context.Context, extGroupID string) error
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
	mux.HandleFunc("PUT "+apiRoot+"/", h.route)
	mux.HandleFunc("DELETE "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	// PUT /5g-vn-groups/{extGroupId}
	case r.Method == http.MethodPut && matchPath(segments, "5g-vn-groups/*"):
		h.handleCreate5GVnGroup(w, r, segments[1])

	// GET /5g-vn-groups/{extGroupId}
	case r.Method == http.MethodGet && matchPath(segments, "5g-vn-groups/*"):
		h.handleGet5GVnGroup(w, r, segments[1])

	// PATCH /5g-vn-groups/{extGroupId}
	case r.Method == http.MethodPatch && matchPath(segments, "5g-vn-groups/*"):
		h.handleModify5GVnGroup(w, r, segments[1])

	// DELETE /5g-vn-groups/{extGroupId}
	case r.Method == http.MethodDelete && matchPath(segments, "5g-vn-groups/*"):
		h.handleDelete5GVnGroup(w, r, segments[1])

	// PUT /mbs-group-membership/{extGroupId}
	case r.Method == http.MethodPut && matchPath(segments, "mbs-group-membership/*"):
		h.handleCreateMbsGroupMembership(w, r, segments[1])

	// GET /mbs-group-membership/{extGroupId}
	case r.Method == http.MethodGet && matchPath(segments, "mbs-group-membership/*"):
		h.handleGetMbsGroupMembership(w, r, segments[1])

	// PATCH /mbs-group-membership/{extGroupId}
	case r.Method == http.MethodPatch && matchPath(segments, "mbs-group-membership/*"):
		h.handleModifyMbsGroupMembership(w, r, segments[1])

	// DELETE /mbs-group-membership/{extGroupId}
	case r.Method == http.MethodDelete && matchPath(segments, "mbs-group-membership/*"):
		h.handleDeleteMbsGroupMembership(w, r, segments[1])

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

// --- 5G VN Group handlers ---

// handleCreate5GVnGroup handles PUT /5g-vn-groups/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — Create5GVnGroup
func (h *Handler) handleCreate5GVnGroup(w http.ResponseWriter, r *http.Request, extGroupID string) {
	var cfg VnGroupConfiguration
	if err := sbi.ReadJSON(r, &cfg); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}
	result, created, err := h.svc.Create5GVnGroup(r.Context(), extGroupID, &cfg)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	_ = sbi.WriteJSON(w, status, result)
}

// handleGet5GVnGroup handles GET /5g-vn-groups/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — Get5GVnGroup
func (h *Handler) handleGet5GVnGroup(w http.ResponseWriter, r *http.Request, extGroupID string) {
	result, err := h.svc.Get5GVnGroup(r.Context(), extGroupID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleModify5GVnGroup handles PATCH /5g-vn-groups/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — Modify5GVnGroup
func (h *Handler) handleModify5GVnGroup(w http.ResponseWriter, r *http.Request, extGroupID string) {
	var patch VnGroupConfiguration
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}
	result, err := h.svc.Modify5GVnGroup(r.Context(), extGroupID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDelete5GVnGroup handles DELETE /5g-vn-groups/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — Delete5GVnGroup
func (h *Handler) handleDelete5GVnGroup(w http.ResponseWriter, r *http.Request, extGroupID string) {
	if err := h.svc.Delete5GVnGroup(r.Context(), extGroupID); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- MBS Group Membership handlers ---

// handleCreateMbsGroupMembership handles PUT /mbs-group-membership/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — CreateMbsGroupMembership
func (h *Handler) handleCreateMbsGroupMembership(w http.ResponseWriter, r *http.Request, extGroupID string) {
	var memb MbsGroupMemb
	if err := sbi.ReadJSON(r, &memb); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}
	result, created, err := h.svc.CreateMbsGroupMembership(r.Context(), extGroupID, &memb)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	_ = sbi.WriteJSON(w, status, result)
}

// handleGetMbsGroupMembership handles GET /mbs-group-membership/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — GetMbsGroupMembership
func (h *Handler) handleGetMbsGroupMembership(w http.ResponseWriter, r *http.Request, extGroupID string) {
	result, err := h.svc.GetMbsGroupMembership(r.Context(), extGroupID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleModifyMbsGroupMembership handles PATCH /mbs-group-membership/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — ModifyMbsGroupMembership
func (h *Handler) handleModifyMbsGroupMembership(w http.ResponseWriter, r *http.Request, extGroupID string) {
	var patch MbsGroupMemb
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}
	result, err := h.svc.ModifyMbsGroupMembership(r.Context(), extGroupID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDeleteMbsGroupMembership handles DELETE /mbs-group-membership/{extGroupId}.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — DeleteMbsGroupMembership
func (h *Handler) handleDeleteMbsGroupMembership(w http.ResponseWriter, r *http.Request, extGroupID string) {
	if err := h.svc.DeleteMbsGroupMembership(r.Context(), extGroupID); err != nil {
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
