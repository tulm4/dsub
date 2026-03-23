package ee

// Business logic layer for the Nudm_EE service.
//
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
// 3GPP: TS 29.503 Nudm_EE — Event Exposure service operations
// 3GPP: TS 23.502 §4.15.3 — Event Exposure procedure

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

// DB defines the database operations required by the EE service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_EE business logic.
//
// Based on: docs/service-decomposition.md §2.4
type Service struct {
	db DB
}

// NewService creates a new EE service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateUeID validates a ueIdentity path parameter. For EE endpoints the
// ueIdentity may be a SUPI (imsi-), GPSI (msisdn-/extid-), or a group
// identifier (group- prefix).
func validateUeID(ueID string) error {
	if identifiers.IsSUPI(ueID) {
		return identifiers.ValidateSUPI(ueID)
	}
	if identifiers.IsGPSI(ueID) {
		return identifiers.ValidateGPSI(ueID)
	}
	if strings.HasPrefix(ueID, "group-") {
		if len(ueID) <= len("group-") {
			return fmt.Errorf("invalid group identifier: must be group-<non-empty>: %s", ueID)
		}
		return nil
	}
	return fmt.Errorf("invalid identifier format: must be SUPI (imsi-), GPSI (msisdn-/extid-), or group ID (group-): %s", ueID)
}

// allowedColumns is the allowlist of column names that can appear in
// dynamically constructed WHERE clauses.  Only these three values are
// returned by identityColumns.
var allowedColumns = map[string]bool{
	"supi":        true,
	"gpsi":        true,
	"ue_group_id": true,
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

// safeColumn validates that col is in the allowedColumns allowlist and panics
// if it is not. This prevents SQL injection via dynamic column names.
func safeColumn(col string) string {
	if !allowedColumns[col] {
		panic(fmt.Sprintf("ee: unexpected column name: %q", col))
	}
	return col
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

// CreateSubscription creates a new event exposure subscription.
//
// Based on: docs/sbi-api-design.md §3.4 (POST /{ueIdentity}/ee-subscriptions)
// 3GPP: TS 29.503 Nudm_EE — CreateEeSubscription
func (s *Service) CreateSubscription(ctx context.Context, ueIdentity string, sub *EeSubscription) (*CreatedEeSubscription, error) {
	if err := validateUeID(ueIdentity); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("ee: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if sub == nil {
		return nil, errors.NewBadRequest("ee: missing subscription body", errors.CauseMandatoryIEMissing)
	}
	if sub.CallbackReference == "" {
		return nil, errors.NewBadRequest("ee: callbackReference is required", errors.CauseMandatoryIEMissing)
	}
	if len(sub.MonitoringConfigurations) == 0 || string(sub.MonitoringConfigurations) == "null" {
		return nil, errors.NewBadRequest("ee: monitoringConfigurations is required", errors.CauseMandatoryIEMissing)
	}

	col, val := identityColumns(ueIdentity)

	// json.RawMessage is a []byte alias — json.Marshal cannot fail for it.
	monCfgBytes, _ := json.Marshal(sub.MonitoringConfigurations)
	repOptBytes, _ := json.Marshal(sub.ReportingOptions)
	immRepBytes := marshalNullableJSON(sub.ImmediateReportData)

	var subscriptionID string
	row := s.db.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO udm.ee_subscriptions (%s, callback_reference, monitoring_configurations, reporting_options, supported_features, scef_id, nf_instance_id, data_restoration_callback_uri, excluded_unsubscribed_ues, immediate_report_data, expiry_time)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING subscription_id`, safeColumn(col)),
		val, sub.CallbackReference, monCfgBytes, repOptBytes,
		sub.SupportedFeatures, sub.ScefID, sub.NfInstanceID,
		sub.DataRestorationCallbackURI, sub.ExcludedUnsubscribedUes, immRepBytes, sub.ExpiryTime,
	)

	if err := row.Scan(&subscriptionID); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("ee: create subscription: %s", err))
	}

	return &CreatedEeSubscription{
		EeSubscription: sub,
		SubscriptionID: subscriptionID,
	}, nil
}

// UpdateSubscription modifies an existing event exposure subscription.
//
// Based on: docs/sbi-api-design.md §3.4 (PATCH /{ueIdentity}/ee-subscriptions/{subscriptionId})
// 3GPP: TS 29.503 Nudm_EE — UpdateEeSubscription
func (s *Service) UpdateSubscription(ctx context.Context, ueIdentity, subscriptionID string, patch *PatchEeSubscription) (*EeSubscription, error) {
	if err := validateUeID(ueIdentity); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("ee: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if patch == nil {
		return nil, errors.NewBadRequest("ee: missing patch body", errors.CauseMandatoryIEMissing)
	}

	col, val := identityColumns(ueIdentity)

	// Marshal JSONB patch fields.
	var monCfgBytes, repOptBytes, immRepBytes []byte
	if patch.MonitoringConfigurations != nil {
		monCfgBytes, _ = json.Marshal(*patch.MonitoringConfigurations)
	}
	if patch.ReportingOptions != nil {
		repOptBytes, _ = json.Marshal(*patch.ReportingOptions)
	}
	if patch.ImmediateReportData != nil {
		immRepBytes, _ = json.Marshal(*patch.ImmediateReportData)
	}

	row := s.db.QueryRow(ctx,
		fmt.Sprintf(
			`UPDATE udm.ee_subscriptions
		 SET callback_reference = COALESCE($1, callback_reference),
		     monitoring_configurations = COALESCE($2, monitoring_configurations),
		     reporting_options = COALESCE($3, reporting_options),
		     supported_features = COALESCE($4, supported_features),
		     expiry_time = COALESCE($5, expiry_time),
		     scef_id = COALESCE($6, scef_id),
		     nf_instance_id = COALESCE($7, nf_instance_id),
		     data_restoration_callback_uri = COALESCE($8, data_restoration_callback_uri),
		     excluded_unsubscribed_ues = COALESCE($9, excluded_unsubscribed_ues),
		     immediate_report_data = COALESCE($10, immediate_report_data)
		 WHERE subscription_id = $11 AND %s = $12
		 RETURNING callback_reference, monitoring_configurations, reporting_options, supported_features,
		           scef_id, nf_instance_id, data_restoration_callback_uri, excluded_unsubscribed_ues,
		           immediate_report_data, expiry_time`, safeColumn(col)),
		patch.CallbackReference,
		monCfgBytes,
		repOptBytes,
		patch.SupportedFeatures,
		patch.ExpiryTime,
		patch.ScefID,
		patch.NfInstanceID,
		patch.DataRestorationCallbackURI,
		patch.ExcludedUnsubscribedUes,
		immRepBytes,
		subscriptionID, val,
	)

	var result EeSubscription
	if err := row.Scan(
		&result.CallbackReference,
		&result.MonitoringConfigurations,
		&result.ReportingOptions,
		&result.SupportedFeatures,
		&result.ScefID,
		&result.NfInstanceID,
		&result.DataRestorationCallbackURI,
		&result.ExcludedUnsubscribedUes,
		&result.ImmediateReportData,
		&result.ExpiryTime,
	); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("ee: subscription %s not found for: %s", subscriptionID, ueIdentity),
			errors.CauseSubscriptionNotFound,
		)
	}

	return &result, nil
}

// DeleteSubscription removes an event exposure subscription.
//
// Based on: docs/sbi-api-design.md §3.4 (DELETE /{ueIdentity}/ee-subscriptions/{subscriptionId})
// 3GPP: TS 29.503 Nudm_EE — DeleteEeSubscription
func (s *Service) DeleteSubscription(ctx context.Context, ueIdentity, subscriptionID string) error {
	if err := validateUeID(ueIdentity); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("ee: invalid ueIdentity: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	col, val := identityColumns(ueIdentity)

	tag, err := s.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM udm.ee_subscriptions WHERE subscription_id = $1 AND %s = $2`, safeColumn(col)),
		subscriptionID, val,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("ee: delete subscription: %s", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("ee: subscription %s not found for: %s", subscriptionID, ueIdentity),
			errors.CauseSubscriptionNotFound,
		)
	}

	return nil
}

// GetMatchingSubscriptions returns all EE subscriptions that match a given SUPI
// and should receive event notifications. This is called by other services
// (e.g., UECM) when events occur that need to be reported to EE subscribers.
//
// Based on: docs/sequence-diagrams.md §10 (Event Exposure)
// 3GPP: TS 29.503 Nudm_EE — Event notification
func (s *Service) GetMatchingSubscriptions(ctx context.Context, supi string) ([]EeEventReport, error) {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("ee: invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	rows, err := s.db.Query(ctx,
		`SELECT subscription_id, callback_reference, monitoring_configurations
		 FROM udm.ee_subscriptions
		 WHERE supi = $1`,
		supi,
	)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("ee: query matching subscriptions: %s", err))
	}
	defer rows.Close()

	var reports []EeEventReport
	for rows.Next() {
		var r EeEventReport
		if err := rows.Scan(&r.SubscriptionID, &r.CallbackReference, &r.MonitoringReport); err != nil {
			return nil, errors.NewInternalError(fmt.Sprintf("ee: scan subscription: %s", err))
		}
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("ee: iterate subscriptions: %s", err))
	}
	return reports, nil
}
