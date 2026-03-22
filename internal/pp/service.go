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

// marshalNullableJSON returns the JSON encoding of raw, or nil if raw is nil.
// json.RawMessage is a []byte alias so json.Marshal cannot fail for it.
func marshalNullableJSON(raw json.RawMessage) []byte {
	if raw == nil {
		return nil
	}
	b, _ := json.Marshal(raw)
	return b
}

// UpdatePPData creates or updates provisioned parameter data for a subscriber.
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
		     communication_characteristics = EXCLUDED.communication_characteristics,
		     supported_features = EXCLUDED.supported_features,
		     expected_ue_behaviour = EXCLUDED.expected_ue_behaviour,
		     ec_restriction = EXCLUDED.ec_restriction,
		     acs_info = EXCLUDED.acs_info,
		     sor_info = EXCLUDED.sor_info,
		     five_mbs_authorization_info = EXCLUDED.five_mbs_authorization_info,
		     steering_container = EXCLUDED.steering_container,
		     pp_dl_packet_count = EXCLUDED.pp_dl_packet_count,
		     pp_dl_packet_count_ext = EXCLUDED.pp_dl_packet_count_ext,
		     pp_maximum_response_time = EXCLUDED.pp_maximum_response_time,
		     pp_maximum_latency = EXCLUDED.pp_maximum_latency,
		     updated_at = NOW()
		 RETURNING communication_characteristics, supported_features,
		           expected_ue_behaviour, ec_restriction, acs_info, sor_info,
		           five_mbs_authorization_info, steering_container,
		           pp_dl_packet_count, pp_dl_packet_count_ext,
		           pp_maximum_response_time, pp_maximum_latency`,
		ueID,
		commCharBytes,
		patch.SupportedFeatures,
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
