package pp

// Business logic layer for the Nudm_PP service.
//
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
// 3GPP: TS 29.503 Nudm_PP — Parameter Provisioning service operations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the PP service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_PP business logic.
//
// Based on: docs/service-decomposition.md §2.5
type Service struct {
	db DB
}

// NewService creates a new PP service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateSUPI validates a ueId path parameter. For PP endpoints the ueId
// must be a SUPI (imsi-).
func validateSUPI(ueID string) error {
	if !identifiers.IsSUPI(ueID) {
		return fmt.Errorf("invalid identifier format: must be SUPI (imsi-): %s", ueID)
	}
	return identifiers.ValidateSUPI(ueID)
}

// GetPPData retrieves provisioned parameter data for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.5 (GET /{ueId}/pp-data)
// 3GPP: TS 29.503 Nudm_PP — GetPPData
func (s *Service) GetPPData(ctx context.Context, ueID string) (*PpData, error) {
	if err := validateSUPI(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("pp: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	var data PpData
	row := s.db.QueryRow(ctx,
		`SELECT communication_characteristics, supported_features, expected_ue_behaviour,
		        ec_restriction, acs_info, sor_info, five_mbs_authorization_info,
		        steering_container, pp_dl_packet_count, pp_dl_packet_count_ext,
		        pp_maximum_response_time, pp_maximum_latency
		 FROM udm.pp_data WHERE supi = $1`,
		ueID,
	)

	err := row.Scan(
		&data.CommunicationCharacteristics,
		&data.SupportedFeatures,
		&data.ExpectedUeBehaviour,
		&data.EcRestriction,
		&data.AcsInfo,
		&data.SorInfo,
		&data.FiveMbsAuthorizationInfo,
		&data.SteeringContainer,
		&data.PpDlPacketCount,
		&data.PpDlPacketCountExt,
		&data.PpMaximumResponseTime,
		&data.PpMaximumLatency,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("pp: no provisioned parameter data for %s", ueID),
				errors.CauseDataNotFound,
			)
		}
		return nil, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}

	return &data, nil
}

// marshalNullableJSON returns the raw JSON bytes, or nil if raw is nil.
// json.RawMessage is already a []byte alias containing valid JSON.
func marshalNullableJSON(raw json.RawMessage) []byte {
	if raw == nil {
		return nil
	}
	return []byte(raw)
}

// UpdatePPData creates or updates provisioned parameter data for a subscriber.
// Uses COALESCE to preserve existing values when fields are absent in the patch.
//
// Based on: docs/sbi-api-design.md §3.5 (PATCH /{ueId}/pp-data)
// 3GPP: TS 29.503 Nudm_PP — UpdatePPData
func (s *Service) UpdatePPData(ctx context.Context, ueID string, patch *PpData) (*PpData, error) {
	if err := validateSUPI(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("pp: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if patch == nil {
		return nil, errors.NewBadRequest("pp: missing request body", errors.CauseMandatoryIEMissing)
	}

	commCharBytes := marshalNullableJSON(patch.CommunicationCharacteristics)
	expectedUeBytes := marshalNullableJSON(patch.ExpectedUeBehaviour)
	ecRestrictionBytes := marshalNullableJSON(patch.EcRestriction)
	acsInfoBytes := marshalNullableJSON(patch.AcsInfo)
	sorInfoBytes := marshalNullableJSON(patch.SorInfo)
	fiveMbsBytes := marshalNullableJSON(patch.FiveMbsAuthorizationInfo)
	steeringBytes := marshalNullableJSON(patch.SteeringContainer)
	ppDlCountExtBytes := marshalNullableJSON(patch.PpDlPacketCountExt)

	// supportedFeatures: pass nil (SQL NULL) when the field is not provided
	// (zero value for string) to preserve via COALESCE.
	// An explicit empty string is treated the same as absent since the 3GPP
	// SupportedFeatures field is always non-empty when present.
	var supportedFeatures *string
	if patch.SupportedFeatures != "" {
		supportedFeatures = &patch.SupportedFeatures
	}

	var result PpData
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.pp_data (
		     supi, communication_characteristics, supported_features,
		     expected_ue_behaviour, ec_restriction, acs_info, sor_info,
		     five_mbs_authorization_info, steering_container,
		     pp_dl_packet_count, pp_dl_packet_count_ext,
		     pp_maximum_response_time, pp_maximum_latency
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (supi) DO UPDATE SET
		     communication_characteristics = COALESCE(EXCLUDED.communication_characteristics, udm.pp_data.communication_characteristics),
		     supported_features = COALESCE(EXCLUDED.supported_features, udm.pp_data.supported_features),
		     expected_ue_behaviour = COALESCE(EXCLUDED.expected_ue_behaviour, udm.pp_data.expected_ue_behaviour),
		     ec_restriction = COALESCE(EXCLUDED.ec_restriction, udm.pp_data.ec_restriction),
		     acs_info = COALESCE(EXCLUDED.acs_info, udm.pp_data.acs_info),
		     sor_info = COALESCE(EXCLUDED.sor_info, udm.pp_data.sor_info),
		     five_mbs_authorization_info = COALESCE(EXCLUDED.five_mbs_authorization_info, udm.pp_data.five_mbs_authorization_info),
		     steering_container = COALESCE(EXCLUDED.steering_container, udm.pp_data.steering_container),
		     pp_dl_packet_count = COALESCE(EXCLUDED.pp_dl_packet_count, udm.pp_data.pp_dl_packet_count),
		     pp_dl_packet_count_ext = COALESCE(EXCLUDED.pp_dl_packet_count_ext, udm.pp_data.pp_dl_packet_count_ext),
		     pp_maximum_response_time = COALESCE(EXCLUDED.pp_maximum_response_time, udm.pp_data.pp_maximum_response_time),
		     pp_maximum_latency = COALESCE(EXCLUDED.pp_maximum_latency, udm.pp_data.pp_maximum_latency),
		     updated_at = NOW()
		 RETURNING communication_characteristics, supported_features,
		           expected_ue_behaviour, ec_restriction, acs_info, sor_info,
		           five_mbs_authorization_info, steering_container,
		           pp_dl_packet_count, pp_dl_packet_count_ext,
		           pp_maximum_response_time, pp_maximum_latency`,
		ueID,
		commCharBytes,
		supportedFeatures,
		expectedUeBytes,
		ecRestrictionBytes,
		acsInfoBytes,
		sorInfoBytes,
		fiveMbsBytes,
		steeringBytes,
		patch.PpDlPacketCount,
		ppDlCountExtBytes,
		patch.PpMaximumResponseTime,
		patch.PpMaximumLatency,
	)

	err := row.Scan(
		&result.CommunicationCharacteristics,
		&result.SupportedFeatures,
		&result.ExpectedUeBehaviour,
		&result.EcRestriction,
		&result.AcsInfo,
		&result.SorInfo,
		&result.FiveMbsAuthorizationInfo,
		&result.SteeringContainer,
		&result.PpDlPacketCount,
		&result.PpDlPacketCountExt,
		&result.PpMaximumResponseTime,
		&result.PpMaximumLatency,
	)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// 5G VN Group operations
// Based on: docs/sbi-api-design.md §3.5 (5G VN Group endpoints)
// 3GPP: TS 29.503 Nudm_PP — 5GVnGroupConfiguration CRUD
// ---------------------------------------------------------------------------

// validateExtGroupID validates the extGroupId path parameter.
func validateExtGroupID(extGroupID string) error {
	if extGroupID == "" {
		return errors.NewBadRequest("pp: extGroupId is required", errors.CauseMandatoryIEMissing)
	}
	return nil
}

// marshalStringSlice marshals a string slice to JSON bytes for JSONB storage.
// Returns nil if the slice is nil (used by COALESCE in PATCH to preserve existing value).
// An empty slice marshals to '[]', which explicitly clears the array.
func marshalStringSlice(s []string) ([]byte, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// scanVnGroup scans a database row into a VnGroupConfiguration.
// Columns must be in order: dnn, s_nssai, pdu_session_types, app_descriptors,
// secondary_auth, dn_aaa_address, dn_aaa_fqdn, members, reference_id,
// af_instance_id, internal_group_identifier, mtc_provider_information.
func scanVnGroup(row pgx.Row) (*VnGroupConfiguration, error) {
	var result VnGroupConfiguration
	var secondaryAuth bool
	var membersJSON []byte
	err := row.Scan(
		&result.Dnn,
		&result.SNssai,
		&result.PduSessionTypes,
		&result.AppDescriptors,
		&secondaryAuth,
		&result.DnAaaAddress,
		&result.DnAaaFqdn,
		&membersJSON,
		&result.ReferenceId,
		&result.AfInstanceId,
		&result.InternalGroupIdentifier,
		&result.MtcProviderInformation,
	)
	if err != nil {
		return nil, err
	}
	result.SecondaryAuth = &secondaryAuth
	if membersJSON != nil {
		if err := json.Unmarshal(membersJSON, &result.Members); err != nil {
			return nil, fmt.Errorf("unmarshal members: %w", err)
		}
	}
	return &result, nil
}

// Create5GVnGroup creates or replaces a 5G VN group configuration.
//
// Based on: docs/sbi-api-design.md §3.5 (PUT /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Create5GVnGroup
func (s *Service) Create5GVnGroup(ctx context.Context, extGroupID string, cfg *VnGroupConfiguration) (*VnGroupConfiguration, bool, error) {
	if err := validateExtGroupID(extGroupID); err != nil {
		return nil, false, err
	}
	if cfg == nil {
		return nil, false, errors.NewBadRequest("pp: missing request body", errors.CauseMandatoryIEMissing)
	}

	sNssaiBytes := marshalNullableJSON(cfg.SNssai)
	appDescBytes := marshalNullableJSON(cfg.AppDescriptors)
	dnAaaAddrBytes := marshalNullableJSON(cfg.DnAaaAddress)
	mtcProvBytes := marshalNullableJSON(cfg.MtcProviderInformation)

	membersBytes, err := marshalStringSlice(cfg.Members)
	if err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("pp: failed to marshal members: %v", err))
	}
	// vn_groups.members is NOT NULL DEFAULT '[]'::jsonb — never insert SQL NULL.
	if membersBytes == nil {
		membersBytes = []byte("[]")
	}

	var secondaryAuth bool
	if cfg.SecondaryAuth != nil {
		secondaryAuth = *cfg.SecondaryAuth
	}

	var created bool
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.vn_groups (
		     ext_group_id, dnn, s_nssai, pdu_session_types, app_descriptors,
		     secondary_auth, dn_aaa_address, dn_aaa_fqdn, members,
		     reference_id, af_instance_id, internal_group_identifier,
		     mtc_provider_information
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (ext_group_id) DO UPDATE SET
		     dnn = EXCLUDED.dnn,
		     s_nssai = EXCLUDED.s_nssai,
		     pdu_session_types = EXCLUDED.pdu_session_types,
		     app_descriptors = EXCLUDED.app_descriptors,
		     secondary_auth = EXCLUDED.secondary_auth,
		     dn_aaa_address = EXCLUDED.dn_aaa_address,
		     dn_aaa_fqdn = EXCLUDED.dn_aaa_fqdn,
		     members = EXCLUDED.members,
		     reference_id = EXCLUDED.reference_id,
		     af_instance_id = EXCLUDED.af_instance_id,
		     internal_group_identifier = EXCLUDED.internal_group_identifier,
		     mtc_provider_information = EXCLUDED.mtc_provider_information,
		     updated_at = NOW()
		 RETURNING (xmax = 0)`,
		extGroupID,
		cfg.Dnn,
		sNssaiBytes,
		cfg.PduSessionTypes,
		appDescBytes,
		secondaryAuth,
		dnAaaAddrBytes,
		cfg.DnAaaFqdn,
		membersBytes,
		cfg.ReferenceId,
		cfg.AfInstanceId,
		cfg.InternalGroupIdentifier,
		mtcProvBytes,
	)
	if err := row.Scan(&created); err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}

	return cfg, created, nil
}

// Get5GVnGroup retrieves a 5G VN group configuration.
//
// Based on: docs/sbi-api-design.md §3.5 (GET /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Get5GVnGroup
func (s *Service) Get5GVnGroup(ctx context.Context, extGroupID string) (*VnGroupConfiguration, error) {
	if err := validateExtGroupID(extGroupID); err != nil {
		return nil, err
	}

	row := s.db.QueryRow(ctx,
		`SELECT dnn, s_nssai, pdu_session_types, app_descriptors,
		        secondary_auth, dn_aaa_address, dn_aaa_fqdn, members,
		        reference_id, af_instance_id, internal_group_identifier,
		        mtc_provider_information
		 FROM udm.vn_groups WHERE ext_group_id = $1`,
		extGroupID,
	)

	result, err := scanVnGroup(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("pp: 5G VN group not found: %s", extGroupID),
				errors.CauseDataNotFound,
			)
		}
		return nil, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}
	return result, nil
}

// Modify5GVnGroup modifies an existing 5G VN group configuration.
//
// Based on: docs/sbi-api-design.md §3.5 (PATCH /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Modify5GVnGroup
func (s *Service) Modify5GVnGroup(ctx context.Context, extGroupID string, patch *VnGroupConfiguration) (*VnGroupConfiguration, error) {
	if err := validateExtGroupID(extGroupID); err != nil {
		return nil, err
	}
	if patch == nil {
		return nil, errors.NewBadRequest("pp: missing request body", errors.CauseMandatoryIEMissing)
	}

	sNssaiBytes := marshalNullableJSON(patch.SNssai)
	appDescBytes := marshalNullableJSON(patch.AppDescriptors)
	dnAaaAddrBytes := marshalNullableJSON(patch.DnAaaAddress)
	mtcProvBytes := marshalNullableJSON(patch.MtcProviderInformation)

	membersBytes, err := marshalStringSlice(patch.Members)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("pp: failed to marshal members: %v", err))
	}

	row := s.db.QueryRow(ctx,
		`UPDATE udm.vn_groups SET
		     dnn = COALESCE(NULLIF($2, ''), dnn),
		     s_nssai = COALESCE($3, s_nssai),
		     pdu_session_types = COALESCE($4, pdu_session_types),
		     app_descriptors = COALESCE($5, app_descriptors),
		     secondary_auth = COALESCE($6, secondary_auth),
		     dn_aaa_address = COALESCE($7, dn_aaa_address),
		     dn_aaa_fqdn = COALESCE(NULLIF($8, ''), dn_aaa_fqdn),
		     members = COALESCE($9, members),
		     reference_id = COALESCE(NULLIF($10, ''), reference_id),
		     af_instance_id = COALESCE(NULLIF($11, ''), af_instance_id),
		     internal_group_identifier = COALESCE(NULLIF($12, ''), internal_group_identifier),
		     mtc_provider_information = COALESCE($13, mtc_provider_information),
		     updated_at = NOW()
		 WHERE ext_group_id = $1
		 RETURNING dnn, s_nssai, pdu_session_types, app_descriptors,
		           secondary_auth, dn_aaa_address, dn_aaa_fqdn, members,
		           reference_id, af_instance_id, internal_group_identifier,
		           mtc_provider_information`,
		extGroupID,
		patch.Dnn,
		sNssaiBytes,
		patch.PduSessionTypes,
		appDescBytes,
		patch.SecondaryAuth,
		dnAaaAddrBytes,
		patch.DnAaaFqdn,
		membersBytes,
		patch.ReferenceId,
		patch.AfInstanceId,
		patch.InternalGroupIdentifier,
		mtcProvBytes,
	)

	result, err := scanVnGroup(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("pp: 5G VN group not found: %s", extGroupID),
				errors.CauseDataNotFound,
			)
		}
		return nil, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}
	return result, nil
}

// Delete5GVnGroup deletes a 5G VN group configuration.
//
// Based on: docs/sbi-api-design.md §3.5 (DELETE /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Delete5GVnGroup
func (s *Service) Delete5GVnGroup(ctx context.Context, extGroupID string) error {
	if err := validateExtGroupID(extGroupID); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM udm.vn_groups WHERE ext_group_id = $1`,
		extGroupID,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("pp: 5G VN group not found: %s", extGroupID),
			errors.CauseDataNotFound,
		)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MBS Group Membership operations
// Based on: docs/sbi-api-design.md §3.5 (MBS Group Membership endpoints)
// 3GPP: TS 29.503 Nudm_PP — MulticastMbsGroupMemb CRUD
// ---------------------------------------------------------------------------

// CreateMbsGroupMembership creates or replaces an MBS group membership.
//
// Based on: docs/sbi-api-design.md §3.5 (PUT /mbs-group-membership/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — CreateMbsGroupMembership
func (s *Service) CreateMbsGroupMembership(ctx context.Context, extGroupID string, memb *MbsGroupMemb) (*MbsGroupMemb, bool, error) {
	if err := validateExtGroupID(extGroupID); err != nil {
		return nil, false, err
	}
	if memb == nil {
		return nil, false, errors.NewBadRequest("pp: missing request body", errors.CauseMandatoryIEMissing)
	}

	multicastBytes := marshalNullableJSON(memb.MulticastGroupMemb)
	// mbs_group_membership.multicast_group_memb is NOT NULL DEFAULT '[]'::jsonb —
	// never insert SQL NULL.
	if multicastBytes == nil {
		multicastBytes = []byte("[]")
	}

	var created bool
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.mbs_group_membership (
		     ext_group_id, multicast_group_memb, af_instance_id,
		     internal_group_identifier
		 ) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (ext_group_id) DO UPDATE SET
		     multicast_group_memb = EXCLUDED.multicast_group_memb,
		     af_instance_id = EXCLUDED.af_instance_id,
		     internal_group_identifier = EXCLUDED.internal_group_identifier,
		     updated_at = NOW()
		 RETURNING (xmax = 0)`,
		extGroupID,
		multicastBytes,
		memb.AfInstanceId,
		memb.InternalGroupIdentifier,
	)
	if err := row.Scan(&created); err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}

	return memb, created, nil
}

// GetMbsGroupMembership retrieves an MBS group membership.
//
// Based on: docs/sbi-api-design.md §3.5 (GET /mbs-group-membership/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — GetMbsGroupMembership
func (s *Service) GetMbsGroupMembership(ctx context.Context, extGroupID string) (*MbsGroupMemb, error) {
	if err := validateExtGroupID(extGroupID); err != nil {
		return nil, err
	}

	var result MbsGroupMemb
	row := s.db.QueryRow(ctx,
		`SELECT multicast_group_memb, af_instance_id, internal_group_identifier
		 FROM udm.mbs_group_membership WHERE ext_group_id = $1`,
		extGroupID,
	)
	err := row.Scan(
		&result.MulticastGroupMemb,
		&result.AfInstanceId,
		&result.InternalGroupIdentifier,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("pp: MBS group membership not found: %s", extGroupID),
				errors.CauseDataNotFound,
			)
		}
		return nil, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}
	return &result, nil
}

// ModifyMbsGroupMembership modifies an existing MBS group membership.
//
// Based on: docs/sbi-api-design.md §3.5 (PATCH /mbs-group-membership/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — ModifyMbsGroupMembership
func (s *Service) ModifyMbsGroupMembership(ctx context.Context, extGroupID string, patch *MbsGroupMemb) (*MbsGroupMemb, error) {
	if err := validateExtGroupID(extGroupID); err != nil {
		return nil, err
	}
	if patch == nil {
		return nil, errors.NewBadRequest("pp: missing request body", errors.CauseMandatoryIEMissing)
	}

	multicastBytes := marshalNullableJSON(patch.MulticastGroupMemb)

	var result MbsGroupMemb
	row := s.db.QueryRow(ctx,
		`UPDATE udm.mbs_group_membership SET
		     multicast_group_memb = COALESCE($2, multicast_group_memb),
		     af_instance_id = COALESCE(NULLIF($3, ''), af_instance_id),
		     internal_group_identifier = COALESCE(NULLIF($4, ''), internal_group_identifier),
		     updated_at = NOW()
		 WHERE ext_group_id = $1
		 RETURNING multicast_group_memb, af_instance_id, internal_group_identifier`,
		extGroupID,
		multicastBytes,
		patch.AfInstanceId,
		patch.InternalGroupIdentifier,
	)
	err := row.Scan(
		&result.MulticastGroupMemb,
		&result.AfInstanceId,
		&result.InternalGroupIdentifier,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("pp: MBS group membership not found: %s", extGroupID),
				errors.CauseDataNotFound,
			)
		}
		return nil, errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}
	return &result, nil
}

// DeleteMbsGroupMembership deletes an MBS group membership.
//
// Based on: docs/sbi-api-design.md §3.5 (DELETE /mbs-group-membership/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — DeleteMbsGroupMembership
func (s *Service) DeleteMbsGroupMembership(ctx context.Context, extGroupID string) error {
	if err := validateExtGroupID(extGroupID); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM udm.mbs_group_membership WHERE ext_group_id = $1`,
		extGroupID,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("pp: database error: %v", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("pp: MBS group membership not found: %s", extGroupID),
			errors.CauseDataNotFound,
		)
	}
	return nil
}

// GetSdmSubscriptionsForNotify returns SDM subscriptions that should be notified
// when subscriber data changes for the given SUPI. This queries the
// udm.sdm_subscriptions table so that the PP service can dispatch
// modification-notification callbacks after provisioned parameter updates.
//
// Based on: docs/sequence-diagrams.md §8 (Subscription Data Update Notification)
// 3GPP: TS 29.503 Nudm_SDM — SDM Subscription / Notification
func (s *Service) GetSdmSubscriptionsForNotify(ctx context.Context, supi string) ([]SdmSubscriptionInfo, error) {
	if err := validateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("pp: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	rows, err := s.db.Query(ctx,
		`SELECT subscription_id, callback_reference, monitored_resource_uris
		 FROM udm.sdm_subscriptions
		 WHERE supi = $1`,
		supi,
	)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("pp: query SDM subscriptions: %s", err))
	}
	defer rows.Close()

	var subs []SdmSubscriptionInfo
	for rows.Next() {
		var sub SdmSubscriptionInfo
		if err := rows.Scan(&sub.SubscriptionID, &sub.CallbackReference, &sub.MonitoredResourceURIs); err != nil {
			return nil, errors.NewInternalError(fmt.Sprintf("pp: scan SDM subscription: %s", err))
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("pp: iterate SDM subscriptions: %s", err))
	}
	return subs, nil
}
