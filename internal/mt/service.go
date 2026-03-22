package mt

// Business logic layer for the Nudm_MT service.
//
// Based on: docs/service-decomposition.md §2.6 (udm-mt)
// 3GPP: TS 29.503 Nudm_MT — Mobile Terminated service operations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// accessType3GPP is the access type value used to query the 3GPP AMF
// registration from the amf_registrations table.
const accessType3GPP = "3GPP_ACCESS"

// userStateRegistered is the default state when an AMF registration exists.
const userStateRegistered = "REGISTERED"

// DB defines the database operations required by the MT service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_MT business logic.
//
// Based on: docs/service-decomposition.md §2.6
type Service struct {
	db DB
}

// NewService creates a new MT service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateSUPI validates that the identifier is a valid SUPI. The MT
// service only accepts SUPIs (imsi-).
func validateSUPI(supi string) error {
	if !identifiers.IsSUPI(supi) {
		return fmt.Errorf("mt: identifier must be a SUPI (imsi-): %s", supi)
	}
	return identifiers.ValidateSUPI(supi)
}

// QueryUeInfo retrieves UE reachability and serving AMF information
// by querying the AMF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.6 (GET /{supi})
// 3GPP: TS 29.503 Nudm_MT — QueryUeInfo
func (s *Service) QueryUeInfo(ctx context.Context, supi string) (*UeInfo, error) {
	if err := validateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("mt: invalid supi: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT amf_instance_id, rat_type, access_type
		 FROM udm.amf_registrations
		 WHERE supi = $1 AND access_type = $2`,
		supi, accessType3GPP,
	)

	var amfInstanceID, ratType, accessType string
	if err := row.Scan(&amfInstanceID, &ratType, &accessType); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("mt: AMF registration not found for: %s", supi),
			errors.CauseContextNotFound,
		)
	}

	return &UeInfo{
		UserState:    userStateRegistered,
		ServingAmfId: amfInstanceID,
		RatType:      ratType,
		AccessType:   accessType,
	}, nil
}

// ProvideLocationInfo provides UE location information by querying the
// serving AMF from the AMF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.6 (POST /{supi}/loc-info/provide-loc-info)
// 3GPP: TS 29.503 Nudm_MT — ProvideLocationInfo
func (s *Service) ProvideLocationInfo(ctx context.Context, supi string, req *LocationInfoRequest) (*LocationInfoResult, error) {
	if err := validateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("mt: invalid supi: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if req == nil {
		return nil, errors.NewBadRequest("mt: missing location info request body", errors.CauseMandatoryIEMissing)
	}

	row := s.db.QueryRow(ctx,
		`SELECT amf_instance_id, rat_type
		 FROM udm.amf_registrations
		 WHERE supi = $1 AND access_type = $2`,
		supi, accessType3GPP,
	)

	var amfInstanceID, ratType string
	if err := row.Scan(&amfInstanceID, &ratType); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("mt: AMF registration not found for: %s", supi),
			errors.CauseContextNotFound,
		)
	}

	return &LocationInfoResult{
		Supi:         supi,
		ServingAmfId: amfInstanceID,
		UserState:    userStateRegistered,
		RatType:      ratType,
	}, nil
}
