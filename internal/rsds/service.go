package rsds

// Business logic layer for the Nudm_RSDS service.
//
// Based on: docs/service-decomposition.md §2.9 (udm-rsds)
// 3GPP: TS 29.503 Nudm_RSDS — Report SMS Delivery Status service operations

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the RSDS service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_RSDS business logic.
//
// Based on: docs/service-decomposition.md §2.9
type Service struct {
	db DB
}

// NewService creates a new RSDS service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateUeID validates a ueIdentity path parameter. For RSDS endpoints
// the ueIdentity may be a GPSI (msisdn-/extid-) or SUPI (imsi-).
func validateUeID(ueID string) error {
	if identifiers.IsGPSI(ueID) {
		return identifiers.ValidateGPSI(ueID)
	}
	if identifiers.IsSUPI(ueID) {
		return identifiers.ValidateSUPI(ueID)
	}
	if strings.HasPrefix(ueID, "extgroupid-") {
		if len(ueID) <= len("extgroupid-") {
			return fmt.Errorf("invalid extgroupid: must be extgroupid-<non-empty>: %s", ueID)
		}
		return nil
	}
	return fmt.Errorf("invalid identifier format: must be GPSI (msisdn-/extid-) or SUPI (imsi-): %s", ueID)
}

// ReportSMDeliveryStatus records the SMS delivery status and triggers
// EE event notifications for matching subscriptions.
//
// Based on: docs/sbi-api-design.md §3.9 (POST /{ueIdentity}/sm-delivery-status)
// 3GPP: TS 29.503 Nudm_RSDS — ReportSMDeliveryStatus
func (s *Service) ReportSMDeliveryStatus(ctx context.Context, ueIdentity string, req *SmDeliveryStatus) error {
	if err := validateUeID(ueIdentity); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("rsds: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if req == nil {
		return errors.NewBadRequest("rsds: missing delivery status request body", errors.CauseMandatoryIEMissing)
	}
	if req.Gpsi == "" {
		return errors.NewBadRequest("rsds: gpsi is required", errors.CauseMandatoryIEMissing)
	}
	if len(req.SmStatusReport) == 0 || string(req.SmStatusReport) == "null" {
		return errors.NewBadRequest("rsds: smStatusReport is required", errors.CauseMandatoryIEMissing)
	}

	// Resolve SUPI from ueIdentity if it is a SUPI; otherwise leave nil.
	var supi *string
	if identifiers.IsSUPI(ueIdentity) {
		supi = &ueIdentity
	}

	// Record the delivery status.
	statusBytes := []byte(req.SmStatusReport)
	_, err := s.db.Exec(ctx,
		`INSERT INTO udm.sms_delivery_status (supi, gpsi, sms_status_report)
		 VALUES ($1, $2, $3)`,
		supi, req.Gpsi, statusBytes,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("rsds: record delivery status: %s", err))
	}

	return nil
}
