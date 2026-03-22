package sdm

// HTTP handler layer for the Nudm_SDM service.
//
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
// Based on: docs/sbi-api-design.md §3.2 (SDM Endpoints)
// 3GPP: TS 29.503 Nudm_SDM — Subscriber Data Management service API

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// apiRoot is the base path for the Nudm_SDM API per TS 29.503.
const apiRoot = "/nudm-sdm/v2"

// ServiceInterface defines the business logic operations used by the handler.
// This interface decouples the handler from the concrete Service for testing.
type ServiceInterface interface {
	GetAmData(ctx context.Context, supi string) (*AccessAndMobilitySubscriptionData, error)
	GetSmData(ctx context.Context, supi string) ([]SessionManagementSubscriptionData, error)
	GetSmfSelData(ctx context.Context, supi string) (*SmfSelectionSubscriptionData, error)
	GetNSSAI(ctx context.Context, supi string) (*Nssai, error)
	GetSmsData(ctx context.Context, supi string) (*SmsSubscriptionData, error)
	GetSmsMngtData(ctx context.Context, supi string) (*SmsManagementSubscriptionData, error)
	GetUeCtxInAmfData(ctx context.Context, supi string) (*UeContextInAmfData, error)
	GetUeCtxInSmfData(ctx context.Context, supi string) (*UeContextInSmfData, error)
	GetUeCtxInSmsfData(ctx context.Context, supi string) (*UeContextInSmsfData, error)
	GetTraceConfigData(ctx context.Context, supi string) (*TraceData, error)
	GetDataSets(ctx context.Context, supi string, datasetNames []string) (*SubscriptionDataSets, error)
	GetIdTranslation(ctx context.Context, ueID string) (*IdTranslationResult, error)
	Subscribe(ctx context.Context, ueID string, sub *SdmSubscription) (*SdmSubscription, error)
	ModifySubscription(ctx context.Context, ueID, subscriptionID string, patch *SdmSubscription) (*SdmSubscription, error)
	Unsubscribe(ctx context.Context, ueID, subscriptionID string) error
}

// Handler handles HTTP requests for the Nudm_SDM API.
type Handler struct {
	svc ServiceInterface
}

// NewHandler creates a new SDM HTTP handler wrapping the given service.
func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all Nudm_SDM endpoint routes on the given mux.
//
// Based on: docs/sbi-api-design.md §3.2
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
	// --- Shared data & group endpoints (no SUPI prefix) ---
	case r.Method == http.MethodGet && len(segments) == 1 && segments[0] == "shared-data":
		h.handleNotImplemented(w, "GetSharedData")
	case r.Method == http.MethodGet && matchPath(segments, "shared-data/*"):
		h.handleNotImplemented(w, "GetIndividualSharedData")
	case r.Method == http.MethodGet && matchPath(segments, "group-data/group-identifiers"):
		h.handleNotImplemented(w, "GetGroupIdentifiers")
	case r.Method == http.MethodGet && len(segments) == 1 && segments[0] == "multiple-identifiers":
		h.handleNotImplemented(w, "GetMultipleIdentifiers")

	// --- Shared data subscriptions ---
	case r.Method == http.MethodPost && len(segments) == 1 && segments[0] == "shared-data-subscriptions":
		h.handleNotImplemented(w, "SubscribeToSharedData")
	case r.Method == http.MethodPatch && matchPath(segments, "shared-data-subscriptions/*"):
		h.handleNotImplemented(w, "ModifySharedDataSubs")
	case r.Method == http.MethodDelete && matchPath(segments, "shared-data-subscriptions/*"):
		h.handleNotImplemented(w, "UnsubscribeForSharedData")

	// --- SDM subscriptions ---
	case r.Method == http.MethodPost && matchPath(segments, "*/sdm-subscriptions"):
		h.handleSubscribe(w, r, segments[0])
	case r.Method == http.MethodPatch && matchPath(segments, "*/sdm-subscriptions/*"):
		h.handleModifySubscription(w, r, segments[0], segments[2])
	case r.Method == http.MethodDelete && matchPath(segments, "*/sdm-subscriptions/*"):
		h.handleUnsubscribe(w, r, segments[0], segments[2])

	// --- Identity translation ---
	case r.Method == http.MethodGet && matchPath(segments, "*/id-translation-result"):
		h.handleGetIdTranslation(w, r, segments[0])

	// --- Acknowledgments ---
	case r.Method == http.MethodPut && matchPath(segments, "*/am-data/sor-ack"):
		h.handleAcknowledge(w, r)
	case r.Method == http.MethodPut && matchPath(segments, "*/am-data/upu-ack"):
		h.handleAcknowledge(w, r)
	case r.Method == http.MethodPut && matchPath(segments, "*/am-data/subscribed-snssais-ack"):
		h.handleAcknowledge(w, r)
	case r.Method == http.MethodPut && matchPath(segments, "*/am-data/cag-ack"):
		h.handleAcknowledge(w, r)
	case r.Method == http.MethodPost && matchPath(segments, "*/am-data/update-sor"):
		h.handleAcknowledge(w, r)

	// --- ECR data ---
	case r.Method == http.MethodGet && matchPath(segments, "*/am-data/ecr-data"):
		h.handleNotImplemented(w, "GetEcrData")

	// --- Data retrieval (GET /{supi} and GET /{supi}/<resource>) ---
	case r.Method == http.MethodGet && len(segments) == 1:
		h.handleGetDataSets(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/am-data"):
		h.handleGetAmData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/sm-data"):
		h.handleGetSmData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/smf-select-data"):
		h.handleGetSmfSelData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/nssai"):
		h.handleGetNSSAI(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/sms-data"):
		h.handleGetSmsData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/sms-mng-data"):
		h.handleGetSmsMngtData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/ue-context-in-amf-data"):
		h.handleGetUeCtxInAmfData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/ue-context-in-smf-data"):
		h.handleGetUeCtxInSmfData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/ue-context-in-smsf-data"):
		h.handleGetUeCtxInSmsfData(w, r, segments[0])
	case r.Method == http.MethodGet && matchPath(segments, "*/trace-data"):
		h.handleGetTraceConfigData(w, r, segments[0])

	// --- Not-yet-implemented data retrieval endpoints ---
	case r.Method == http.MethodGet && matchPath(segments, "*/lcs-privacy-data"):
		h.handleNotImplemented(w, "GetLcsPrivacyData")
	case r.Method == http.MethodGet && matchPath(segments, "*/lcs-mo-data"):
		h.handleNotImplemented(w, "GetLcsMoData")
	case r.Method == http.MethodGet && matchPath(segments, "*/lcs-bca-data"):
		h.handleNotImplemented(w, "GetLcsBcaData")
	case r.Method == http.MethodGet && matchPath(segments, "*/lcs-subscription-data"):
		h.handleNotImplemented(w, "GetLcsSubscriptionData")
	case r.Method == http.MethodGet && matchPath(segments, "*/v2x-data"):
		h.handleNotImplemented(w, "GetV2xData")
	case r.Method == http.MethodGet && matchPath(segments, "*/prose-data"):
		h.handleNotImplemented(w, "GetProseData")
	case r.Method == http.MethodGet && matchPath(segments, "*/a2x-data"):
		h.handleNotImplemented(w, "GetA2xData")
	case r.Method == http.MethodGet && matchPath(segments, "*/5mbs-data"):
		h.handleNotImplemented(w, "GetMbsData")
	case r.Method == http.MethodGet && matchPath(segments, "*/uc-data"):
		h.handleNotImplemented(w, "GetUcData")
	case r.Method == http.MethodGet && matchPath(segments, "*/time-sync-data"):
		h.handleNotImplemented(w, "GetTimeSyncSubscriptionData")
	case r.Method == http.MethodGet && matchPath(segments, "*/ranging-slpos-data"):
		h.handleNotImplemented(w, "GetRangingSlPosData")
	case r.Method == http.MethodGet && matchPath(segments, "*/rangingsl-privacy-data"):
		h.handleNotImplemented(w, "GetRangingSlPrivacyData")

	default:
		errors.WriteProblemDetails(w, errors.NewNotFound("endpoint not found", ""))
	}
}

// handleGetDataSets handles GET /{supi} — retrieves multiple data sets.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetDataSets
func (h *Handler) handleGetDataSets(w http.ResponseWriter, r *http.Request, supi string) {
	datasetNamesParam := r.URL.Query().Get("dataset-names")
	if datasetNamesParam == "" {
		errors.WriteProblemDetails(w, errors.NewBadRequest(
			"dataset-names query parameter is required",
			errors.CauseMandatoryIEMissing,
		))
		return
	}

	datasetNames := strings.Split(datasetNamesParam, ",")
	result, err := h.svc.GetDataSets(r.Context(), supi, datasetNames)
	if err != nil {
		writeSvcError(w, err)
		return
	}

	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetAmData handles GET /{supi}/am-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetAmData
func (h *Handler) handleGetAmData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetAmData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetSmData handles GET /{supi}/sm-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetSmData
func (h *Handler) handleGetSmData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetSmData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetSmfSelData handles GET /{supi}/smf-select-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetSmfSelData
func (h *Handler) handleGetSmfSelData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetSmfSelData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetNSSAI handles GET /{supi}/nssai.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetNSSAI
func (h *Handler) handleGetNSSAI(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetNSSAI(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetSmsData handles GET /{supi}/sms-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetSmsData
func (h *Handler) handleGetSmsData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetSmsData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetSmsMngtData handles GET /{supi}/sms-mng-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetSmsMngtData
func (h *Handler) handleGetSmsMngtData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetSmsMngtData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetUeCtxInAmfData handles GET /{supi}/ue-context-in-amf-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetUeCtxInAmfData
func (h *Handler) handleGetUeCtxInAmfData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetUeCtxInAmfData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetUeCtxInSmfData handles GET /{supi}/ue-context-in-smf-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetUeCtxInSmfData
func (h *Handler) handleGetUeCtxInSmfData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetUeCtxInSmfData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetUeCtxInSmsfData handles GET /{supi}/ue-context-in-smsf-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetUeCtxInSmsfData
func (h *Handler) handleGetUeCtxInSmsfData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetUeCtxInSmsfData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetTraceConfigData handles GET /{supi}/trace-data.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetTraceConfigData
func (h *Handler) handleGetTraceConfigData(w http.ResponseWriter, r *http.Request, supi string) {
	result, err := h.svc.GetTraceConfigData(r.Context(), supi)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleGetIdTranslation handles GET /{ueId}/id-translation-result.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — GetSupiOrGpsi
func (h *Handler) handleGetIdTranslation(w http.ResponseWriter, r *http.Request, ueID string) {
	result, err := h.svc.GetIdTranslation(r.Context(), ueID)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleSubscribe handles POST /{ueId}/sdm-subscriptions.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — Subscribe
func (h *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request, ueID string) {
	var sub SdmSubscription
	if err := sbi.ReadJSON(r, &sub); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.Subscribe(r.Context(), ueID, &sub)
	if err != nil {
		writeSvcError(w, err)
		return
	}

	_ = sbi.WriteJSON(w, http.StatusCreated, result)
}

// handleModifySubscription handles PATCH /{ueId}/sdm-subscriptions/{subscriptionId}.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — Modify
func (h *Handler) handleModifySubscription(w http.ResponseWriter, r *http.Request, ueID, subscriptionID string) {
	var patch SdmSubscription
	if err := sbi.ReadJSON(r, &patch); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}

	result, err := h.svc.ModifySubscription(r.Context(), ueID, subscriptionID, &patch)
	if err != nil {
		writeSvcError(w, err)
		return
	}

	_ = sbi.WriteJSON(w, http.StatusOK, result)
}

// handleUnsubscribe handles DELETE /{ueId}/sdm-subscriptions/{subscriptionId}.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — Unsubscribe
func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request, ueID, subscriptionID string) {
	if err := h.svc.Unsubscribe(r.Context(), ueID, subscriptionID); err != nil {
		writeSvcError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAcknowledge handles PUT acknowledgment endpoints (sor-ack, upu-ack, etc.).
// These simply record the acknowledgment and return 204 No Content.
//
// Based on: docs/sbi-api-design.md §3.2
// 3GPP: TS 29.503 Nudm_SDM — SorAckInfo, UpuAck, SubscribedSnssaisAck, CagAck
func (h *Handler) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	// Read and discard the body to validate it is well-formed JSON
	var ack json.RawMessage
	if err := sbi.ReadJSON(r, &ack); err != nil {
		errors.WriteProblemDetails(w, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
