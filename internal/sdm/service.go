package sdm

// Business logic layer for the Nudm_SDM service.
//
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
// 3GPP: TS 29.503 Nudm_SDM — Subscriber Data Management service operations
// 3GPP: TS 29.505 — Usage of the Unified Data Repository services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the SDM service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow and Query.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Service implements the Nudm_SDM business logic.
//
// Based on: docs/service-decomposition.md §2.2
type Service struct {
	db DB
}

// NewService creates a new SDM service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateSUPIOrUeId validates a SUPI or ueId path parameter. For SDM, most
// endpoints accept a SUPI. Some accept either SUPI or GPSI as ueId.
func validateSUPIOrUeId(ueID string) error {
	if identifiers.IsSUPI(ueID) {
		return identifiers.ValidateSUPI(ueID)
	}
	if identifiers.IsGPSI(ueID) {
		return identifiers.ValidateGPSI(ueID)
	}
	return fmt.Errorf("invalid identifier format: must be SUPI (imsi-) or GPSI (msisdn-/extid-): %s", ueID)
}

// GetAmData retrieves Access and Mobility subscription data for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/am-data)
// 3GPP: TS 29.503 Nudm_SDM — GetAmData
// 3GPP: TS 29.505 — AccessAndMobilitySubscriptionData
func (s *Service) GetAmData(ctx context.Context, supi string) (*AccessAndMobilitySubscriptionData, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT gpsis, internal_group_ids, subscribed_ue_ambr, nssai,
		        rat_restrictions, forbidden_areas, service_area_restriction,
		        rfsp_index, subs_reg_timer, active_time, sor_info, upu_info,
		        mico_allowed, shared_am_data_ids, odb_packet_services,
		        subscribed_dnn_list, service_gap_time, trace_data, cag_data,
		        routing_indicator, mps_priority, mcs_priority
		 FROM udm.access_mobility_subscription WHERE supi = $1`,
		supi,
	)

	var data AccessAndMobilitySubscriptionData
	err := row.Scan(
		&data.Gpsis, &data.InternalGroupIds, &data.SubscribedUeAmbr, &data.Nssai,
		&data.RatRestrictions, &data.ForbiddenAreas, &data.ServiceAreaRestriction,
		&data.RfspIndex, &data.SubsRegTimer, &data.ActiveTime, &data.SorInfo, &data.UpuInfo,
		&data.MicoAllowed, &data.SharedAmDataIds, &data.OdbPacketServices,
		&data.SubscribedDnnList, &data.ServiceGapTime, &data.TraceData, &data.CagData,
		&data.RoutingIndicator, &data.MpsPriority, &data.McsPriority,
	)
	if err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("sdm: AM data not found for SUPI: %s", supi),
			errors.CauseDataNotFound,
		)
	}

	return &data, nil
}

// GetSmData retrieves Session Management subscription data for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/sm-data)
// 3GPP: TS 29.503 Nudm_SDM — GetSmData
// 3GPP: TS 29.505 — SessionManagementSubscriptionData
func (s *Service) GetSmData(ctx context.Context, supi string) ([]SessionManagementSubscriptionData, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	rows, err := s.db.Query(ctx,
		`SELECT single_nssai, dnn_configurations, internal_group_ids, shared_data_ids
		 FROM udm.session_management_subscription WHERE supi = $1`,
		supi,
	)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("sdm: query SM data: %s", err))
	}
	defer rows.Close()

	var results []SessionManagementSubscriptionData
	for rows.Next() {
		var d SessionManagementSubscriptionData
		if err := rows.Scan(&d.SingleNssai, &d.DnnConfigurations, &d.InternalGroupIds, &d.SharedDataIds); err != nil {
			return nil, errors.NewInternalError(fmt.Sprintf("sdm: scan SM data: %s", err))
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("sdm: iterate SM data: %s", err))
	}

	if len(results) == 0 {
		return nil, errors.NewNotFound(
			fmt.Sprintf("sdm: SM data not found for SUPI: %s", supi),
			errors.CauseDataNotFound,
		)
	}

	return results, nil
}

// GetSmfSelData retrieves SMF selection subscription data for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/smf-select-data)
// 3GPP: TS 29.503 Nudm_SDM — GetSmfSelData
func (s *Service) GetSmfSelData(ctx context.Context, supi string) (*SmfSelectionSubscriptionData, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT subscribed_snssai_infos, shared_snssai_infos_ids
		 FROM udm.smf_selection_subscription WHERE supi = $1`,
		supi,
	)

	var data SmfSelectionSubscriptionData
	err := row.Scan(&data.SubscribedSnssaiInfos, &data.SharedSnssaiInfosIds)
	if err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("sdm: SMF selection data not found for SUPI: %s", supi),
			errors.CauseDataNotFound,
		)
	}

	return &data, nil
}

// GetNSSAI retrieves subscribed NSSAI for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/nssai)
// 3GPP: TS 29.503 Nudm_SDM — GetNSSAI
func (s *Service) GetNSSAI(ctx context.Context, supi string) (*Nssai, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT nssai FROM udm.access_mobility_subscription WHERE supi = $1`,
		supi,
	)

	var nssaiRaw json.RawMessage
	if err := row.Scan(&nssaiRaw); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("sdm: NSSAI data not found for SUPI: %s", supi),
			errors.CauseDataNotFound,
		)
	}

	var nssai Nssai
	if len(nssaiRaw) > 0 {
		if err := json.Unmarshal(nssaiRaw, &nssai); err != nil {
			return nil, errors.NewInternalError(fmt.Sprintf("sdm: unmarshal NSSAI: %s", err))
		}
	}

	return &nssai, nil
}

// GetDataSets retrieves multiple subscription data sets in a single request.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi})
// 3GPP: TS 29.503 Nudm_SDM — GetDataSets
func (s *Service) GetDataSets(ctx context.Context, supi string, datasetNames []string) (*SubscriptionDataSets, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	result := &SubscriptionDataSets{}
	for _, name := range datasetNames {
		switch name {
		case "AM":
			amData, err := s.GetAmData(ctx, supi)
			if err == nil {
				result.AmData = amData
			}
		case "SMF_SEL":
			smfSelData, err := s.GetSmfSelData(ctx, supi)
			if err == nil {
				result.SmfSelData = smfSelData
			}
		case "SMS_SUB":
			smsData, err := s.GetSmsData(ctx, supi)
			if err == nil {
				result.SmsSubsData = smsData
			}
		case "SM":
			smData, err := s.GetSmData(ctx, supi)
			if err == nil {
				result.SmData = smData
			}
		}
	}

	return result, nil
}

// GetSmsData retrieves SMS subscription data for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/sms-data)
// 3GPP: TS 29.503 Nudm_SDM — GetSmsData
func (s *Service) GetSmsData(ctx context.Context, supi string) (*SmsSubscriptionData, error) {
	return getJSONBData[SmsSubscriptionData](ctx, s.db, supi, "sms_subscription_data", "sms_data", "SMS")
}

// GetSmsMngtData retrieves SMS management subscription data for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/sms-mng-data)
// 3GPP: TS 29.503 Nudm_SDM — GetSmsMngtData
func (s *Service) GetSmsMngtData(ctx context.Context, supi string) (*SmsManagementSubscriptionData, error) {
	return getJSONBData[SmsManagementSubscriptionData](ctx, s.db, supi, "sms_management_subscription", "sms_mng_data", "SMS management")
}

// GetUeCtxInAmfData retrieves UE context in AMF data.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/ue-context-in-amf-data)
// 3GPP: TS 29.503 Nudm_SDM — GetUeCtxInAmfData
func (s *Service) GetUeCtxInAmfData(ctx context.Context, supi string) (*UeContextInAmfData, error) {
	return getJSONBData[UeContextInAmfData](ctx, s.db, supi, "amf_registrations", "amf_context", "UE context in AMF")
}

// GetUeCtxInSmfData retrieves UE context in SMF data.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/ue-context-in-smf-data)
// 3GPP: TS 29.503 Nudm_SDM — GetUeCtxInSmfData
func (s *Service) GetUeCtxInSmfData(ctx context.Context, supi string) (*UeContextInSmfData, error) {
	return getJSONBData[UeContextInSmfData](ctx, s.db, supi, "smf_registrations", "smf_context", "UE context in SMF")
}

// GetUeCtxInSmsfData retrieves UE context in SMSF data.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/ue-context-in-smsf-data)
// 3GPP: TS 29.503 Nudm_SDM — GetUeCtxInSmsfData
func (s *Service) GetUeCtxInSmsfData(ctx context.Context, supi string) (*UeContextInSmsfData, error) {
	return getJSONBData[UeContextInSmsfData](ctx, s.db, supi, "smsf_registrations", "smsf_context", "UE context in SMSF")
}

// GetTraceConfigData retrieves trace configuration data.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/trace-data)
// 3GPP: TS 29.503 Nudm_SDM — GetTraceConfigData
func (s *Service) GetTraceConfigData(ctx context.Context, supi string) (*TraceData, error) {
	return getJSONBData[TraceData](ctx, s.db, supi, "access_mobility_subscription", "trace_data", "trace config")
}

// GetIdTranslation translates a UE identity (SUPI to GPSI or GPSI to SUPI).
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{ueId}/id-translation-result)
// 3GPP: TS 29.503 Nudm_SDM — GetSupiOrGpsi
func (s *Service) GetIdTranslation(ctx context.Context, ueID string) (*IdTranslationResult, error) {
	if err := validateSUPIOrUeId(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid identifier: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	var result IdTranslationResult

	if identifiers.IsSUPI(ueID) {
		row := s.db.QueryRow(ctx,
			`SELECT supi, gpsi FROM udm.subscribers WHERE supi = $1`,
			ueID,
		)
		if err := row.Scan(&result.Supi, &result.Gpsi); err != nil {
			return nil, errors.NewNotFound(
				fmt.Sprintf("sdm: subscriber not found: %s", ueID),
				errors.CauseUserNotFound,
			)
		}
	} else {
		row := s.db.QueryRow(ctx,
			`SELECT supi, gpsi FROM udm.subscribers WHERE gpsi = $1`,
			ueID,
		)
		if err := row.Scan(&result.Supi, &result.Gpsi); err != nil {
			return nil, errors.NewNotFound(
				fmt.Sprintf("sdm: subscriber not found: %s", ueID),
				errors.CauseUserNotFound,
			)
		}
	}

	return &result, nil
}

// Subscribe creates a new SDM subscription for data change notifications.
//
// Based on: docs/sbi-api-design.md §3.2 (POST /{ueId}/sdm-subscriptions)
// 3GPP: TS 29.503 Nudm_SDM — Subscribe
func (s *Service) Subscribe(ctx context.Context, ueID string, sub *SdmSubscription) (*SdmSubscription, error) {
	if sub == nil {
		return nil, errors.NewBadRequest("sdm: missing subscription body", errors.CauseMandatoryIEMissing)
	}

	if err := validateSUPIOrUeId(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid identifier: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	if sub.NfInstanceID == "" {
		return nil, errors.NewBadRequest("sdm: nfInstanceId is required", errors.CauseMandatoryIEMissing)
	}
	if sub.CallbackReference == "" {
		return nil, errors.NewBadRequest("sdm: callbackReference is required", errors.CauseMandatoryIEMissing)
	}
	if len(sub.MonitoredResourceUris) == 0 {
		return nil, errors.NewBadRequest("sdm: monitoredResourceUris is required", errors.CauseMandatoryIEMissing)
	}

	// Generate a subscription ID via the database
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.sdm_subscriptions (supi, nf_instance_id, callback_reference, monitored_resource_uris, expires)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING subscription_id`,
		ueID, sub.NfInstanceID, sub.CallbackReference, sub.MonitoredResourceUris, sub.ExpiryTime,
	)

	var subscriptionID string
	if err := row.Scan(&subscriptionID); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("sdm: create subscription: %s", err))
	}

	sub.SubscriptionID = subscriptionID
	return sub, nil
}

// ModifySubscription modifies an existing SDM subscription.
//
// Based on: docs/sbi-api-design.md §3.2 (PATCH /{ueId}/sdm-subscriptions/{subscriptionId})
// 3GPP: TS 29.503 Nudm_SDM — Modify
func (s *Service) ModifySubscription(ctx context.Context, ueID, subscriptionID string, patch *SdmSubscription) (*SdmSubscription, error) {
	if patch == nil {
		return nil, errors.NewBadRequest("sdm: missing patch body", errors.CauseMandatoryIEMissing)
	}

	if err := validateSUPIOrUeId(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid identifier: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`UPDATE udm.sdm_subscriptions
		 SET callback_reference = COALESCE(NULLIF($1, ''), callback_reference),
		     monitored_resource_uris = COALESCE($2, monitored_resource_uris),
		     expires = COALESCE(NULLIF($3, ''), expires)
		 WHERE subscription_id = $4 AND supi = $5
		 RETURNING subscription_id`,
		patch.CallbackReference, patch.MonitoredResourceUris, patch.ExpiryTime,
		subscriptionID, ueID,
	)

	var updatedID string
	if err := row.Scan(&updatedID); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("sdm: subscription not found: %s", subscriptionID),
			errors.CauseSubscriptionNotFound,
		)
	}

	patch.SubscriptionID = updatedID
	return patch, nil
}

// Unsubscribe removes an existing SDM subscription.
//
// Based on: docs/sbi-api-design.md §3.2 (DELETE /{ueId}/sdm-subscriptions/{subscriptionId})
// 3GPP: TS 29.503 Nudm_SDM — Unsubscribe
func (s *Service) Unsubscribe(ctx context.Context, ueID, subscriptionID string) error {
	if err := validateSUPIOrUeId(ueID); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid identifier: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`DELETE FROM udm.sdm_subscriptions WHERE subscription_id = $1 AND supi = $2 RETURNING subscription_id`,
		subscriptionID, ueID,
	)

	var deleted string
	if err := row.Scan(&deleted); err != nil {
		return errors.NewNotFound(
			fmt.Sprintf("sdm: subscription not found: %s", subscriptionID),
			errors.CauseSubscriptionNotFound,
		)
	}

	return nil
}

// allowedTableColumns is a whitelist of valid table.column pairs for the
// generic JSONB retrieval helper. This prevents SQL injection even if the
// function signature is misused in future changes.
var allowedTableColumns = map[string]bool{
	"sms_subscription_data.sms_data":           true,
	"sms_management_subscription.sms_mng_data": true,
	"amf_registrations.amf_context":            true,
	"smf_registrations.smf_context":            true,
	"smsf_registrations.smsf_context":          true,
	"access_mobility_subscription.trace_data":  true,
}

// getJSONBData is a generic helper for retrieving JSONB data from a single
// column in a SUPI-keyed table. Many SDM data retrieval endpoints follow this
// same pattern: validate SUPI, query one row, unmarshal JSONB, return typed result.
//
// The table and column parameters are validated against an internal whitelist
// to prevent SQL injection.
func getJSONBData[T any](ctx context.Context, db DB, supi, table, column, label string) (*T, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("sdm: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	key := table + "." + column
	if !allowedTableColumns[key] {
		return nil, errors.NewInternalError(fmt.Sprintf("sdm: disallowed table/column: %s", key))
	}

	row := db.QueryRow(ctx,
		fmt.Sprintf("SELECT %s FROM udm.%s WHERE supi = $1", column, table),
		supi,
	)

	var raw json.RawMessage
	if err := row.Scan(&raw); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("sdm: %s data not found for SUPI: %s", label, supi),
			errors.CauseDataNotFound,
		)
	}

	var result T
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, errors.NewInternalError(fmt.Sprintf("sdm: unmarshal %s data: %s", label, err))
		}
	}

	return &result, nil
}
