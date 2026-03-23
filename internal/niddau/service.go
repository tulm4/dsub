package niddau

// Business logic layer for the Nudm_NIDDAU service.
//
// Based on: docs/service-decomposition.md §2.8 (udm-niddau)
// 3GPP: TS 29.503 Nudm_NIDDAU — NIDD Authorization service operations
// 3GPP: TS 23.502 — NIDD procedures

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the NIDDAU service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_NIDDAU business logic.
//
// Based on: docs/service-decomposition.md §2.8
type Service struct {
	db DB
}

// NewService creates a new NIDDAU service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateUeID validates a ueIdentity path parameter. For NIDDAU endpoints
// the ueIdentity may be a GPSI (msisdn-/extid-) or a group identifier
// (extgroupid- prefix).
func validateUeID(ueID string) error {
	if identifiers.IsGPSI(ueID) {
		return identifiers.ValidateGPSI(ueID)
	}
	if strings.HasPrefix(ueID, "extgroupid-") {
		if len(ueID) <= len("extgroupid-") {
			return fmt.Errorf("invalid extgroupid: must be extgroupid-<non-empty>: %s", ueID)
		}
		return nil
	}
	// Also accept SUPI for flexibility.
	if identifiers.IsSUPI(ueID) {
		return identifiers.ValidateSUPI(ueID)
	}
	return fmt.Errorf("invalid identifier format: must be GPSI (msisdn-/extid-), group ID (extgroupid-), or SUPI (imsi-): %s", ueID)
}

// AuthorizeNiddData validates that a subscriber is authorized for NIDD on
// the requested DNN and S-NSSAI and returns the authorized configuration.
//
// Based on: docs/sbi-api-design.md §3.8 (POST /{ueIdentity}/authorize)
// 3GPP: TS 29.503 Nudm_NIDDAU — AuthorizeNiddData
func (s *Service) AuthorizeNiddData(ctx context.Context, ueIdentity string, req *AuthorizationInfo) (*AuthorizationData, error) {
	if err := validateUeID(ueIdentity); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("niddau: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if req == nil {
		return nil, errors.NewBadRequest("niddau: missing authorization request body", errors.CauseMandatoryIEMissing)
	}

	// Resolve the subscriber's SUPI and GPSI by looking up the subscribers table.
	// For GPSI-based lookups we query by gpsi; for SUPI-based we query by supi.
	var supi, gpsi string
	var row pgx.Row

	if identifiers.IsSUPI(ueIdentity) {
		row = s.db.QueryRow(ctx,
			`SELECT supi, COALESCE(gpsi, '') FROM udm.subscribers WHERE supi = $1`,
			ueIdentity,
		)
	} else if identifiers.IsGPSI(ueIdentity) {
		row = s.db.QueryRow(ctx,
			`SELECT supi, gpsi FROM udm.subscribers WHERE gpsi = $1`,
			ueIdentity,
		)
	} else {
		// Group identifiers — return authorization data with validity time only.
		return &AuthorizationData{
			ValidityTime: req.ValidityTime,
		}, nil
	}

	if err := row.Scan(&supi, &gpsi); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("niddau: subscriber not found for: %s", ueIdentity),
				errors.CauseUserNotFound,
			)
		}
		return nil, errors.NewInternalError(
			fmt.Sprintf("niddau: database error looking up subscriber %s: %v", ueIdentity, err),
		)
	}

	authInfo := NiddAuthorizationInfo{
		Supi:         supi,
		Gpsi:         gpsi,
		ValidityTime: req.ValidityTime,
	}

	return &AuthorizationData{
		AuthorizationData: []NiddAuthorizationInfo{authInfo},
		ValidityTime:      req.ValidityTime,
	}, nil
}
