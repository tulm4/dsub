package uecm

// HTTP handler layer for the Nudm_UECM service.
//
// Based on: docs/service-decomposition.md §2.3 (udm-uecm)
// Based on: docs/sbi-api-design.md §3.3 (UECM Endpoints)
// 3GPP: TS 29.503 Nudm_UECM — UE Context Management service API

import (
	"context"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_UECM API per TS 29.503.
const apiRoot = "/nudm-uecm/v1"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	// AMF 3GPP access
	Register3GppAccess(ctx context.Context, ueID string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error)
	Get3GppRegistration(ctx context.Context, ueID string) (*Amf3GppAccessRegistration, error)
	Update3GppRegistration(ctx context.Context, ueID string, patch *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, error)
	DeregAMF(ctx context.Context, ueID string, data *DeregistrationData) error
	PeiUpdate(ctx context.Context, ueID string, info *PeiUpdateInfo) error
	UpdateRoamingInformation(ctx context.Context, ueID string, info *RoamingInfoUpdate) error

	// AMF non-3GPP access
	RegisterNon3GppAccess(ctx context.Context, ueID string, reg *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, bool, error)
	GetNon3GppRegistration(ctx context.Context, ueID string) (*AmfNon3GppAccessRegistration, error)
	UpdateNon3GppRegistration(ctx context.Context, ueID string, patch *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, error)

	// SMF registrations
	RegisterSmf(ctx context.Context, ueID string, pduSessionID int, reg *SmfRegistration) (*SmfRegistration, bool, error)
	GetSmfRegistration(ctx context.Context, ueID string) ([]SmfRegistration, error)
	RetrieveSmfRegistration(ctx context.Context, ueID string, pduSessionID int) (*SmfRegistration, error)
	UpdateSmfRegistration(ctx context.Context, ueID string, pduSessionID int, patch *SmfRegistration) (*SmfRegistration, error)
	DeregisterSmf(ctx context.Context, ueID string, pduSessionID int) error

	// SMSF 3GPP access
	RegisterSmsf3Gpp(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error)
	GetSmsf3GppRegistration(ctx context.Context, ueID string) (*SmsfRegistration, error)
	UpdateSmsf3GppRegistration(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error)
	DeregisterSmsf3Gpp(ctx context.Context, ueID string) error

	// SMSF non-3GPP access
	RegisterSmsfNon3Gpp(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error)
	GetSmsfNon3GppRegistration(ctx context.Context, ueID string) (*SmsfRegistration, error)
	UpdateSmsfNon3GppRegistration(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error)
	DeregisterSmsfNon3Gpp(ctx context.Context, ueID string) error

	// Aggregated and routing
	GetRegistrations(ctx context.Context, ueID string) (*RegistrationDataSets, error)
	SendRoutingInfoSm(ctx context.Context, ueID string, req *RoutingInfoSmRequest) (*RoutingInfoSmResponse, error)
}

// Handler handles HTTP requests for the Nudm_UECM API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new UECM HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_UECM endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.3
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiRoot+"/", h.route)
	mux.HandleFunc("POST "+apiRoot+"/", h.route)
	mux.HandleFunc("PUT "+apiRoot+"/", h.route)
	mux.HandleFunc("PATCH "+apiRoot+"/", h.route)
	mux.HandleFunc("DELETE "+apiRoot+"/", h.route)
}

// route dispatches requests to the correct handler method based on the URL path.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiRoot)
	path = strings.TrimPrefix(path, "/")
	segments := splitPath(path)

	switch {
	// --- RestorePcscf (no ueId prefix) ---
	case r.Method == http.MethodPost && len(segments) == 1 && segments[0] == "restore-pcscf":
		h.handleNotImplemented(w, "RestorePcscf")

	// --- Registration overview ---
	// GET /{ueId}/registrations
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations"):
		h.handleGetRegistrations(w, r, segments[0])

	// POST /{ueId}/registrations/send-routing-info-sm
	case r.Method == http.MethodPost && matchPath(segments, "*/registrations/send-routing-info-sm"):
		h.handleSendRoutingInfoSm(w, r, segments[0])

	// --- 3GPP access AMF registration ---
	// PUT /{ueId}/registrations/amf-3gpp-access
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/amf-3gpp-access"):
		h.handleRegister3GppAccess(w, r, segments[0])

	// GET /{ueId}/registrations/amf-3gpp-access
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/amf-3gpp-access"):
		h.handleGet3GppRegistration(w, r, segments[0])

	// PATCH /{ueId}/registrations/amf-3gpp-access
	case r.Method == http.MethodPatch && matchPath(segments, "*/registrations/amf-3gpp-access"):
		h.handleUpdate3GppRegistration(w, r, segments[0])

	// POST /{ueId}/registrations/amf-3gpp-access/dereg-amf
	case r.Method == http.MethodPost && matchPath(segments, "*/registrations/amf-3gpp-access/dereg-amf"):
		h.handleDeregAMF(w, r, segments[0])

	// POST /{ueId}/registrations/amf-3gpp-access/pei-update
	case r.Method == http.MethodPost && matchPath(segments, "*/registrations/amf-3gpp-access/pei-update"):
		h.handlePeiUpdate(w, r, segments[0])

	// POST /{ueId}/registrations/amf-3gpp-access/roaming-info-update
	case r.Method == http.MethodPost && matchPath(segments, "*/registrations/amf-3gpp-access/roaming-info-update"):
		h.handleUpdateRoamingInformation(w, r, segments[0])

	// --- Non-3GPP access AMF registration ---
	// PUT /{ueId}/registrations/amf-non-3gpp-access
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/amf-non-3gpp-access"):
		h.handleRegisterNon3GppAccess(w, r, segments[0])

	// GET /{ueId}/registrations/amf-non-3gpp-access
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/amf-non-3gpp-access"):
		h.handleGetNon3GppRegistration(w, r, segments[0])

	// PATCH /{ueId}/registrations/amf-non-3gpp-access
	case r.Method == http.MethodPatch && matchPath(segments, "*/registrations/amf-non-3gpp-access"):
		h.handleUpdateNon3GppRegistration(w, r, segments[0])

	// --- SMF registrations ---
	// GET /{ueId}/registrations/smf-registrations
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/smf-registrations"):
		h.handleGetSmfRegistration(w, r, segments[0])

	// PUT /{ueId}/registrations/smf-registrations/{pduSessionId}
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/smf-registrations/*"):
		h.handleRegisterSmf(w, r, segments[0], segments[3])

	// GET /{ueId}/registrations/smf-registrations/{pduSessionId}
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/smf-registrations/*"):
		h.handleRetrieveSmfRegistration(w, r, segments[0], segments[3])

	// PATCH /{ueId}/registrations/smf-registrations/{pduSessionId}
	case r.Method == http.MethodPatch && matchPath(segments, "*/registrations/smf-registrations/*"):
		h.handleUpdateSmfRegistration(w, r, segments[0], segments[3])

	// DELETE /{ueId}/registrations/smf-registrations/{pduSessionId}
	case r.Method == http.MethodDelete && matchPath(segments, "*/registrations/smf-registrations/*"):
		h.handleDeregisterSmf(w, r, segments[0], segments[3])

	// --- SMSF 3GPP registration ---
	// PUT /{ueId}/registrations/smsf-3gpp-access
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/smsf-3gpp-access"):
		h.handleRegisterSmsf3Gpp(w, r, segments[0])

	// GET /{ueId}/registrations/smsf-3gpp-access
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/smsf-3gpp-access"):
		h.handleGetSmsf3GppRegistration(w, r, segments[0])

	// PATCH /{ueId}/registrations/smsf-3gpp-access
	case r.Method == http.MethodPatch && matchPath(segments, "*/registrations/smsf-3gpp-access"):
		h.handleUpdateSmsf3GppRegistration(w, r, segments[0])

	// DELETE /{ueId}/registrations/smsf-3gpp-access
	case r.Method == http.MethodDelete && matchPath(segments, "*/registrations/smsf-3gpp-access"):
		h.handleDeregisterSmsf3Gpp(w, r, segments[0])

	// --- SMSF non-3GPP registration ---
	// PUT /{ueId}/registrations/smsf-non-3gpp-access
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/smsf-non-3gpp-access"):
		h.handleRegisterSmsfNon3Gpp(w, r, segments[0])

	// GET /{ueId}/registrations/smsf-non-3gpp-access
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/smsf-non-3gpp-access"):
		h.handleGetSmsfNon3GppRegistration(w, r, segments[0])

	// PATCH /{ueId}/registrations/smsf-non-3gpp-access
	case r.Method == http.MethodPatch && matchPath(segments, "*/registrations/smsf-non-3gpp-access"):
		h.handleUpdateSmsfNon3GppRegistration(w, r, segments[0])

	// DELETE /{ueId}/registrations/smsf-non-3gpp-access
	case r.Method == http.MethodDelete && matchPath(segments, "*/registrations/smsf-non-3gpp-access"):
		h.handleDeregisterSmsfNon3Gpp(w, r, segments[0])

	// --- IP-SM-GW registration ---
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/ip-sm-gw"):
		h.handleNotImplemented(w, "IpSmGwRegistration")
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/ip-sm-gw"):
		h.handleNotImplemented(w, "GetIpSmGwRegistration")
	case r.Method == http.MethodDelete && matchPath(segments, "*/registrations/ip-sm-gw"):
		h.handleNotImplemented(w, "IpSmGwDeregistration")

	// --- NWDAF registration ---
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/nwdaf-registrations"):
		h.handleNotImplemented(w, "GetNwdafRegistration")
	case r.Method == http.MethodPut && matchPath(segments, "*/registrations/nwdaf-registrations/*"):
		h.handleNotImplemented(w, "NwdafRegistration")
	case r.Method == http.MethodPatch && matchPath(segments, "*/registrations/nwdaf-registrations/*"):
		h.handleNotImplemented(w, "UpdateNwdafRegistration")
	case r.Method == http.MethodDelete && matchPath(segments, "*/registrations/nwdaf-registrations/*"):
		h.handleNotImplemented(w, "NwdafDeregistration")

	// --- Location and trigger ---
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/location"):
		h.handleNotImplemented(w, "GetLocationInfo")
	case r.Method == http.MethodGet && matchPath(segments, "*/registrations/trigger-auth"):
		h.handleNotImplemented(w, "AuthTrigger")

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// --- AMF 3GPP access handlers ---

// handleRegister3GppAccess handles PUT /{ueId}/registrations/amf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — 3GppRegistration
func (h *Handler) handleRegister3GppAccess(w http.ResponseWriter, r *http.Request, ueID string) {
	var reg Amf3GppAccessRegistration
	if err := sbi.ReadJSON(r, &reg); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, created, err := h.svc.Register3GppAccess(r.Context(), ueID, &reg)
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

// handleGet3GppRegistration handles GET /{ueId}/registrations/amf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Get3GppRegistration
func (h *Handler) handleGet3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.Get3GppRegistration(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUpdate3GppRegistration handles PATCH /{ueId}/registrations/amf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Update3GppRegistration
func (h *Handler) handleUpdate3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	var patch Amf3GppAccessRegistration
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.Update3GppRegistration(r.Context(), ueID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDeregAMF handles POST /{ueId}/registrations/amf-3gpp-access/dereg-amf.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — DeregAMF
func (h *Handler) handleDeregAMF(w http.ResponseWriter, r *http.Request, ueID string) {
	var data DeregistrationData
	if err := sbi.ReadJSON(r, &data); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.DeregAMF(r.Context(), ueID, &data); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePeiUpdate handles POST /{ueId}/registrations/amf-3gpp-access/pei-update.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — PeiUpdate
func (h *Handler) handlePeiUpdate(w http.ResponseWriter, r *http.Request, ueID string) {
	var info PeiUpdateInfo
	if err := sbi.ReadJSON(r, &info); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.PeiUpdate(r.Context(), ueID, &info); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateRoamingInformation handles POST /{ueId}/registrations/amf-3gpp-access/roaming-info-update.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — UpdateRoamingInformation
func (h *Handler) handleUpdateRoamingInformation(w http.ResponseWriter, r *http.Request, ueID string) {
	var info RoamingInfoUpdate
	if err := sbi.ReadJSON(r, &info); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.UpdateRoamingInformation(r.Context(), ueID, &info); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- AMF non-3GPP access handlers ---

// handleRegisterNon3GppAccess handles PUT /{ueId}/registrations/amf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Non3GppRegistration
func (h *Handler) handleRegisterNon3GppAccess(w http.ResponseWriter, r *http.Request, ueID string) {
	var reg AmfNon3GppAccessRegistration
	if err := sbi.ReadJSON(r, &reg); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, created, err := h.svc.RegisterNon3GppAccess(r.Context(), ueID, &reg)
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

// handleGetNon3GppRegistration handles GET /{ueId}/registrations/amf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — GetNon3GppRegistration
func (h *Handler) handleGetNon3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetNon3GppRegistration(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUpdateNon3GppRegistration handles PATCH /{ueId}/registrations/amf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — UpdateNon3GppRegistration
func (h *Handler) handleUpdateNon3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	var patch AmfNon3GppAccessRegistration
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.UpdateNon3GppRegistration(r.Context(), ueID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// --- SMF registration handlers ---

// handleGetSmfRegistration handles GET /{ueId}/registrations/smf-registrations.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — GetSmfRegistration
func (h *Handler) handleGetSmfRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetSmfRegistration(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleRegisterSmf handles PUT /{ueId}/registrations/smf-registrations/{pduSessionId}.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Registration (SMF)
func (h *Handler) handleRegisterSmf(w http.ResponseWriter, r *http.Request, ueID, pduSessionIDStr string) {
	pduSessionID, err := parsePduSessionID(pduSessionIDStr)
	if err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	var reg SmfRegistration
	if readErr := sbi.ReadJSON(r, &reg); readErr != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(readErr.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, created, err := h.svc.RegisterSmf(r.Context(), ueID, pduSessionID, &reg)
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

// handleRetrieveSmfRegistration handles GET /{ueId}/registrations/smf-registrations/{pduSessionId}.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — RetrieveSmfRegistration
func (h *Handler) handleRetrieveSmfRegistration(w http.ResponseWriter, r *http.Request, ueID, pduSessionIDStr string) {
	pduSessionID, err := parsePduSessionID(pduSessionIDStr)
	if err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.RetrieveSmfRegistration(r.Context(), ueID, pduSessionID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUpdateSmfRegistration handles PATCH /{ueId}/registrations/smf-registrations/{pduSessionId}.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — UpdateSmfRegistration
func (h *Handler) handleUpdateSmfRegistration(w http.ResponseWriter, r *http.Request, ueID, pduSessionIDStr string) {
	pduSessionID, err := parsePduSessionID(pduSessionIDStr)
	if err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	var patch SmfRegistration
	if readErr := sbi.ReadJSON(r, &patch); readErr != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(readErr.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.UpdateSmfRegistration(r.Context(), ueID, pduSessionID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDeregisterSmf handles DELETE /{ueId}/registrations/smf-registrations/{pduSessionId}.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — SmfDeregistration
func (h *Handler) handleDeregisterSmf(w http.ResponseWriter, r *http.Request, ueID, pduSessionIDStr string) {
	pduSessionID, err := parsePduSessionID(pduSessionIDStr)
	if err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	if err := h.svc.DeregisterSmf(r.Context(), ueID, pduSessionID); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- SMSF 3GPP access handlers ---

// handleRegisterSmsf3Gpp handles PUT /{ueId}/registrations/smsf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — 3GppSmsfRegistration
func (h *Handler) handleRegisterSmsf3Gpp(w http.ResponseWriter, r *http.Request, ueID string) {
	var reg SmsfRegistration
	if err := sbi.ReadJSON(r, &reg); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, created, err := h.svc.RegisterSmsf3Gpp(r.Context(), ueID, &reg)
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

// handleGetSmsf3GppRegistration handles GET /{ueId}/registrations/smsf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Get3GppSmsfRegistration
func (h *Handler) handleGetSmsf3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetSmsf3GppRegistration(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUpdateSmsf3GppRegistration handles PATCH /{ueId}/registrations/smsf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — UpdateSmsf3GppRegistration
func (h *Handler) handleUpdateSmsf3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	var patch SmsfRegistration
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.UpdateSmsf3GppRegistration(r.Context(), ueID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDeregisterSmsf3Gpp handles DELETE /{ueId}/registrations/smsf-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — 3GppSmsfDeregistration
func (h *Handler) handleDeregisterSmsf3Gpp(w http.ResponseWriter, r *http.Request, ueID string) {
	if err := h.svc.DeregisterSmsf3Gpp(r.Context(), ueID); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- SMSF non-3GPP access handlers ---

// handleRegisterSmsfNon3Gpp handles PUT /{ueId}/registrations/smsf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Non3GppSmsfRegistration
func (h *Handler) handleRegisterSmsfNon3Gpp(w http.ResponseWriter, r *http.Request, ueID string) {
	var reg SmsfRegistration
	if err := sbi.ReadJSON(r, &reg); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, created, err := h.svc.RegisterSmsfNon3Gpp(r.Context(), ueID, &reg)
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

// handleGetSmsfNon3GppRegistration handles GET /{ueId}/registrations/smsf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — GetNon3GppSmsfRegistration
func (h *Handler) handleGetSmsfNon3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetSmsfNon3GppRegistration(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUpdateSmsfNon3GppRegistration handles PATCH /{ueId}/registrations/smsf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — UpdateSmsfNon3GppRegistration
func (h *Handler) handleUpdateSmsfNon3GppRegistration(w http.ResponseWriter, r *http.Request, ueID string) {
	var patch SmsfRegistration
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.UpdateSmsfNon3GppRegistration(r.Context(), ueID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleDeregisterSmsfNon3Gpp handles DELETE /{ueId}/registrations/smsf-non-3gpp-access.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — Non3GppSmsfDeregistration
func (h *Handler) handleDeregisterSmsfNon3Gpp(w http.ResponseWriter, r *http.Request, ueID string) {
	if err := h.svc.DeregisterSmsfNon3Gpp(r.Context(), ueID); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Aggregated / routing handlers ---

// handleGetRegistrations handles GET /{ueId}/registrations.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — GetRegistrations
func (h *Handler) handleGetRegistrations(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetRegistrations(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleSendRoutingInfoSm handles POST /{ueId}/registrations/send-routing-info-sm.
//
// Based on: docs/sbi-api-design.md §3.3
// 3GPP: TS 29.503 Nudm_UECM — SendRoutingInfoSm
func (h *Handler) handleSendRoutingInfoSm(w http.ResponseWriter, r *http.Request, ueID string) {
	var req RoutingInfoSmRequest
	if err := sbi.ReadJSON(r, &req); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.SendRoutingInfoSm(r.Context(), ueID, &req)
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
