package ssau

// Business logic layer for the Nudm_SSAU service.
//
// Based on: docs/service-decomposition.md §2.7 (udm-ssau)
// 3GPP: TS 29.503 Nudm_SSAU — Service-Specific Authorization service operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the SSAU service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_SSAU business logic.
//
// Based on: docs/service-decomposition.md §2.7
type Service struct {
	db DB
}

// NewService creates a new SSAU service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validServiceTypes enumerates the allowed service type values for SSAU
// authorization per TS 29.503.
var validServiceTypes = map[string]bool{
	"AF_GUIDANCE_FOR_URSP": true,
	"AF_REQUESTED_QOS":     true,
}

// validateUeID validates a ueIdentity path parameter. For SSAU endpoints
// the ueIdentity may be a GPSI (msisdn-/extid-), a group identifier
// (group- prefix), or a SUPI (imsi-).
func validateUeID(ueID string) error {
	if identifiers.IsGPSI(ueID) {
		return identifiers.ValidateGPSI(ueID)
	}
	if strings.HasPrefix(ueID, "group-") {
		if len(ueID) <= len("group-") {
			return fmt.Errorf("invalid group identifier: must be group-<non-empty>: %s", ueID)
		}
		return nil
	}
	// Also accept SUPI for flexibility.
	if identifiers.IsSUPI(ueID) {
		return identifiers.ValidateSUPI(ueID)
	}
	return fmt.Errorf("invalid identifier format: must be GPSI (msisdn-/extid-), group ID (group-), or SUPI (imsi-): %s", ueID)
}

// validateServiceType validates that the serviceType is a known value.
func validateServiceType(serviceType string) error {
	if !validServiceTypes[serviceType] {
		return fmt.Errorf("unsupported serviceType: %s", serviceType)
	}
	return nil
}

// identityColumns returns the column name and value to use for the given ueIdentity.
// The returned column is always one of "supi", "gpsi", or "ue_group_id".
func identityColumns(ueID string) (string, string) {
	if identifiers.IsSUPI(ueID) {
		return "supi", ueID
	}
	if identifiers.IsGPSI(ueID) {
		return "gpsi", ueID
	}
	return "ue_group_id", ueID
}

// allowedColumns is the allowlist of column names that can appear in
// dynamically constructed WHERE clauses.
var allowedColumns = map[string]bool{
	"supi":        true,
	"gpsi":        true,
	"ue_group_id": true,
}

// safeColumn validates that col is in the allowedColumns allowlist and panics
// if it is not. This prevents SQL injection via dynamic column names.
func safeColumn(col string) string {
	if !allowedColumns[col] {
		panic(fmt.Sprintf("ssau: unexpected column name: %q", col))
	}
	return col
}

// Authorize performs service-specific authorization for a UE.
//
// Based on: docs/sbi-api-design.md §3.7 (POST /{ueIdentity}/{serviceType}/authorize)
// 3GPP: TS 29.503 Nudm_SSAU — ServiceSpecificAuthorization
func (s *Service) Authorize(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error) {
	if err := validateUeID(ueIdentity); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("ssau: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if err := validateServiceType(serviceType); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("ssau: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if req == nil {
		return nil, errors.NewBadRequest("ssau: missing authorization request body", errors.CauseMandatoryIEMissing)
	}

	col, val := identityColumns(ueIdentity)

	var snssaiBytes []byte
	if req.SNssai != nil {
		snssaiBytes = []byte(req.SNssai)
	}

	var authorizationData []byte
	ueIDJSON, _ := json.Marshal(map[string]string{col: val})

	var authID string
	row := s.db.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO udm.ssau_authorizations (%s, service_type, snssai, dnn, authorization_data, auth_callback_uri, af_id, nef_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING auth_id, authorization_data`, safeColumn(col)),
		val, serviceType, snssaiBytes, req.Dnn, ueIDJSON,
		req.AuthUpdateCallbackURI, req.AfID, req.NefID,
	)

	if err := row.Scan(&authID, &authorizationData); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("ssau: create authorization: %s", err))
	}

	return &ServiceSpecificAuthorizationData{
		AuthorizationUeID: ueIDJSON,
		AuthID:            authID,
	}, nil
}

// Remove revokes a service-specific authorization.
//
// Based on: docs/sbi-api-design.md §3.7 (POST /{ueIdentity}/{serviceType}/remove)
// 3GPP: TS 29.503 Nudm_SSAU — ServiceSpecificAuthorizationRemoval
func (s *Service) Remove(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationRemoveData) error {
	if err := validateUeID(ueIdentity); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("ssau: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if err := validateServiceType(serviceType); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("ssau: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if req == nil || req.AuthID == "" {
		return errors.NewBadRequest("ssau: authId is required", errors.CauseMandatoryIEMissing)
	}

	col, val := identityColumns(ueIdentity)

	tag, err := s.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM udm.ssau_authorizations WHERE auth_id = $1 AND %s = $2 AND service_type = $3`, safeColumn(col)),
		req.AuthID, val, serviceType,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("ssau: remove authorization: %s", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("ssau: authorization %s not found for: %s", req.AuthID, ueIdentity),
			errors.CauseContextNotFound,
		)
	}

	return nil
}
