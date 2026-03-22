package uecm

// Business logic layer for the Nudm_UECM service.
//
// Based on: docs/service-decomposition.md §2.3 (udm-uecm)
// 3GPP: TS 29.503 Nudm_UECM — UE Context Management service operations
// 3GPP: TS 23.502 §4.2.2.2 — Registration procedure

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// accessType3GPP and accessTypeNon3GPP are the access type values used as
// composite primary key components in the amf_registrations and
// smsf_registrations tables.
const (
	accessType3GPP    = "3GPP_ACCESS"
	accessTypeNon3GPP = "NON_3GPP_ACCESS"
)

// DB defines the database operations required by the UECM service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow, Query, and Exec.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Service implements the Nudm_UECM business logic.
//
// Based on: docs/service-decomposition.md §2.3
type Service struct {
	db DB
}

// NewService creates a new UECM service with the given database dependency.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validateUeID validates a ueId path parameter. For UECM endpoints the ueId
// is typically a SUPI (imsi-) or GPSI (msisdn-/extid-).
func validateUeID(ueID string) error {
	if identifiers.IsSUPI(ueID) {
		return identifiers.ValidateSUPI(ueID)
	}
	if identifiers.IsGPSI(ueID) {
		return identifiers.ValidateGPSI(ueID)
	}
	return fmt.Errorf("invalid identifier format: must be SUPI (imsi-) or GPSI (msisdn-/extid-): %s", ueID)
}

// Register3GppAccess registers or updates an AMF registration for 3GPP access.
// If a different AMF was previously registered the old registration is
// overwritten (the caller should notify the old AMF via deregistration callback).
//
// Based on: docs/sbi-api-design.md §3.3 (PUT /{ueId}/registrations/amf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — 3GppRegistration
// 3GPP: TS 23.502 §4.2.2.2.2 — AMF registration in UDM
func (s *Service) Register3GppAccess(ctx context.Context, ueID string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, false, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if reg == nil {
		return nil, false, errors.NewBadRequest("uecm: missing registration body", errors.CauseMandatoryIEMissing)
	}
	if reg.AmfInstanceID == "" {
		return nil, false, errors.NewBadRequest("uecm: amfInstanceId is required", errors.CauseMandatoryIEMissing)
	}
	if reg.DeregCallbackURI == "" {
		return nil, false, errors.NewBadRequest("uecm: deregCallbackUri is required", errors.CauseMandatoryIEMissing)
	}

	guamiBytes, _ := json.Marshal(reg.Guami)

	// UPSERT — returns whether a row already existed.
	var existed bool
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.amf_registrations (supi, access_type, amf_instance_id, dereg_callback_uri, guami, rat_type, initial_registration_ind, pei, registration_time)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (supi, access_type) DO UPDATE SET
		     amf_instance_id = EXCLUDED.amf_instance_id,
		     dereg_callback_uri = EXCLUDED.dereg_callback_uri,
		     guami = EXCLUDED.guami,
		     rat_type = EXCLUDED.rat_type,
		     initial_registration_ind = EXCLUDED.initial_registration_ind,
		     pei = EXCLUDED.pei,
		     registration_time = EXCLUDED.registration_time
		 RETURNING (xmax <> 0)`,
		ueID, accessType3GPP, reg.AmfInstanceID, reg.DeregCallbackURI,
		guamiBytes, reg.RatType, reg.InitialRegistrationInd, reg.Pei, reg.RegistrationTime,
	)

	if err := row.Scan(&existed); err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("uecm: register 3GPP AMF: %s", err))
	}

	created := !existed
	return reg, created, nil
}

// Get3GppRegistration retrieves the current AMF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations/amf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — Get3GppRegistration
func (s *Service) Get3GppRegistration(ctx context.Context, ueID string) (*Amf3GppAccessRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT amf_instance_id, dereg_callback_uri, guami, rat_type, initial_registration_ind, pei, registration_time
		 FROM udm.amf_registrations WHERE supi = $1 AND access_type = $2`,
		ueID, accessType3GPP,
	)

	var reg Amf3GppAccessRegistration
	if err := row.Scan(
		&reg.AmfInstanceID, &reg.DeregCallbackURI, &reg.Guami,
		&reg.RatType, &reg.InitialRegistrationInd, &reg.Pei, &reg.RegistrationTime,
	); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: 3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return &reg, nil
}

// Update3GppRegistration patches the current AMF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (PATCH /{ueId}/registrations/amf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — Update3GppRegistration
func (s *Service) Update3GppRegistration(ctx context.Context, ueID string, patch *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if patch == nil {
		return nil, errors.NewBadRequest("uecm: missing patch body", errors.CauseMandatoryIEMissing)
	}

	row := s.db.QueryRow(ctx,
		`UPDATE udm.amf_registrations
		 SET pei = COALESCE(NULLIF($1, ''), pei),
		     purge_flag = $2
		 WHERE supi = $3 AND access_type = $4
		 RETURNING amf_instance_id`,
		patch.Pei, patch.PurgeFlag, ueID, accessType3GPP,
	)

	var amfID string
	if err := row.Scan(&amfID); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: 3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	patch.AmfInstanceID = amfID
	return patch, nil
}

// DeregAMF processes an AMF-initiated deregistration for 3GPP access.
//
// Based on: docs/sbi-api-design.md §3.3 (POST /{ueId}/registrations/amf-3gpp-access/dereg-amf)
// 3GPP: TS 29.503 Nudm_UECM — DeregAMF
// 3GPP: TS 23.502 §4.2.2.3.2 — UE-initiated de-registration
func (s *Service) DeregAMF(ctx context.Context, ueID string, data *DeregistrationData) error {
	if err := validateUeID(ueID); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if data == nil {
		return errors.NewBadRequest("uecm: missing deregistration body", errors.CauseMandatoryIEMissing)
	}
	if data.DeregReason == "" {
		return errors.NewBadRequest("uecm: deregReason is required", errors.CauseMandatoryIEMissing)
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM udm.amf_registrations WHERE supi = $1 AND access_type = $2`,
		ueID, accessType3GPP,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("uecm: deregister 3GPP AMF: %s", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("uecm: 3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return nil
}

// RegisterNon3GppAccess registers or updates an AMF registration for non-3GPP access.
//
// Based on: docs/sbi-api-design.md §3.3 (PUT /{ueId}/registrations/amf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — Non3GppRegistration
func (s *Service) RegisterNon3GppAccess(ctx context.Context, ueID string, reg *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, bool, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, false, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if reg == nil {
		return nil, false, errors.NewBadRequest("uecm: missing registration body", errors.CauseMandatoryIEMissing)
	}
	if reg.AmfInstanceID == "" {
		return nil, false, errors.NewBadRequest("uecm: amfInstanceId is required", errors.CauseMandatoryIEMissing)
	}
	if reg.DeregCallbackURI == "" {
		return nil, false, errors.NewBadRequest("uecm: deregCallbackUri is required", errors.CauseMandatoryIEMissing)
	}

	guamiBytes, _ := json.Marshal(reg.Guami)

	var existed bool
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.amf_registrations (supi, access_type, amf_instance_id, dereg_callback_uri, guami, rat_type, initial_registration_ind, pei, registration_time)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (supi, access_type) DO UPDATE SET
		     amf_instance_id = EXCLUDED.amf_instance_id,
		     dereg_callback_uri = EXCLUDED.dereg_callback_uri,
		     guami = EXCLUDED.guami,
		     rat_type = EXCLUDED.rat_type,
		     initial_registration_ind = EXCLUDED.initial_registration_ind,
		     pei = EXCLUDED.pei,
		     registration_time = EXCLUDED.registration_time
		 RETURNING (xmax <> 0)`,
		ueID, accessTypeNon3GPP, reg.AmfInstanceID, reg.DeregCallbackURI,
		guamiBytes, reg.RatType, reg.InitialRegistrationInd, reg.Pei, reg.RegistrationTime,
	)

	if err := row.Scan(&existed); err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("uecm: register non-3GPP AMF: %s", err))
	}

	created := !existed
	return reg, created, nil
}

// GetNon3GppRegistration retrieves the current AMF non-3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations/amf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — GetNon3GppRegistration
func (s *Service) GetNon3GppRegistration(ctx context.Context, ueID string) (*AmfNon3GppAccessRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT amf_instance_id, dereg_callback_uri, guami, rat_type, initial_registration_ind, pei, registration_time
		 FROM udm.amf_registrations WHERE supi = $1 AND access_type = $2`,
		ueID, accessTypeNon3GPP,
	)

	var reg AmfNon3GppAccessRegistration
	if err := row.Scan(
		&reg.AmfInstanceID, &reg.DeregCallbackURI, &reg.Guami,
		&reg.RatType, &reg.InitialRegistrationInd, &reg.Pei, &reg.RegistrationTime,
	); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: non-3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return &reg, nil
}

// UpdateNon3GppRegistration patches the current AMF non-3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (PATCH /{ueId}/registrations/amf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — UpdateNon3GppRegistration
func (s *Service) UpdateNon3GppRegistration(ctx context.Context, ueID string, patch *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if patch == nil {
		return nil, errors.NewBadRequest("uecm: missing patch body", errors.CauseMandatoryIEMissing)
	}

	row := s.db.QueryRow(ctx,
		`UPDATE udm.amf_registrations
		 SET pei = COALESCE(NULLIF($1, ''), pei),
		     purge_flag = $2
		 WHERE supi = $3 AND access_type = $4
		 RETURNING amf_instance_id`,
		patch.Pei, patch.PurgeFlag, ueID, accessTypeNon3GPP,
	)

	var amfID string
	if err := row.Scan(&amfID); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: non-3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	patch.AmfInstanceID = amfID
	return patch, nil
}

// RegisterSmf registers or updates a SMF registration for a PDU session.
//
// Based on: docs/sbi-api-design.md §3.3 (PUT /{ueId}/registrations/smf-registrations/{pduSessionId})
// 3GPP: TS 29.503 Nudm_UECM — Registration (SMF)
func (s *Service) RegisterSmf(ctx context.Context, ueID string, pduSessionID int, reg *SmfRegistration) (*SmfRegistration, bool, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, false, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if reg == nil {
		return nil, false, errors.NewBadRequest("uecm: missing registration body", errors.CauseMandatoryIEMissing)
	}
	if reg.SmfInstanceID == "" {
		return nil, false, errors.NewBadRequest("uecm: smfInstanceId is required", errors.CauseMandatoryIEMissing)
	}

	reg.PduSessionID = pduSessionID

	var existed bool
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.smf_registrations (supi, pdu_session_id, smf_instance_id, dnn, single_nssai, plmn_id, registration_time)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (supi, pdu_session_id) DO UPDATE SET
		     smf_instance_id = EXCLUDED.smf_instance_id,
		     dnn = EXCLUDED.dnn,
		     single_nssai = EXCLUDED.single_nssai,
		     plmn_id = EXCLUDED.plmn_id,
		     registration_time = EXCLUDED.registration_time
		 RETURNING (xmax <> 0)`,
		ueID, pduSessionID, reg.SmfInstanceID, reg.Dnn, reg.SingleNssai, reg.PlmnID, reg.RegistrationTime,
	)

	if err := row.Scan(&existed); err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("uecm: register SMF: %s", err))
	}

	created := !existed
	return reg, created, nil
}

// GetSmfRegistration retrieves all SMF registrations for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations/smf-registrations)
// 3GPP: TS 29.503 Nudm_UECM — GetSmfRegistration
func (s *Service) GetSmfRegistration(ctx context.Context, ueID string) ([]SmfRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	rows, err := s.db.Query(ctx,
		`SELECT smf_instance_id, pdu_session_id, dnn, single_nssai, plmn_id, registration_time
		 FROM udm.smf_registrations WHERE supi = $1`,
		ueID,
	)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("uecm: query SMF registrations: %s", err))
	}
	defer rows.Close()

	var results []SmfRegistration
	for rows.Next() {
		var r SmfRegistration
		if err := rows.Scan(
			&r.SmfInstanceID, &r.PduSessionID, &r.Dnn,
			&r.SingleNssai, &r.PlmnID, &r.RegistrationTime,
		); err != nil {
			return nil, errors.NewInternalError(fmt.Sprintf("uecm: scan SMF registration: %s", err))
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("uecm: iterate SMF registrations: %s", err))
	}

	if len(results) == 0 {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: no SMF registrations for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return results, nil
}

// RetrieveSmfRegistration retrieves a single SMF registration by PDU session ID.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations/smf-registrations/{pduSessionId})
// 3GPP: TS 29.503 Nudm_UECM — RetrieveSmfRegistration
func (s *Service) RetrieveSmfRegistration(ctx context.Context, ueID string, pduSessionID int) (*SmfRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT smf_instance_id, pdu_session_id, dnn, single_nssai, plmn_id, registration_time
		 FROM udm.smf_registrations WHERE supi = $1 AND pdu_session_id = $2`,
		ueID, pduSessionID,
	)

	var r SmfRegistration
	if err := row.Scan(
		&r.SmfInstanceID, &r.PduSessionID, &r.Dnn,
		&r.SingleNssai, &r.PlmnID, &r.RegistrationTime,
	); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: SMF registration not found for PDU session %d: %s", pduSessionID, ueID),
			errors.CauseContextNotFound,
		)
	}

	return &r, nil
}

// DeregisterSmf removes a SMF registration for a PDU session.
//
// Based on: docs/sbi-api-design.md §3.3 (DELETE /{ueId}/registrations/smf-registrations/{pduSessionId})
// 3GPP: TS 29.503 Nudm_UECM — SmfDeregistration
func (s *Service) DeregisterSmf(ctx context.Context, ueID string, pduSessionID int) error {
	if err := validateUeID(ueID); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM udm.smf_registrations WHERE supi = $1 AND pdu_session_id = $2`,
		ueID, pduSessionID,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("uecm: deregister SMF: %s", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("uecm: SMF registration not found for PDU session %d: %s", pduSessionID, ueID),
			errors.CauseContextNotFound,
		)
	}

	return nil
}

// UpdateSmfRegistration patches an existing SMF registration.
//
// Based on: docs/sbi-api-design.md §3.3 (PATCH /{ueId}/registrations/smf-registrations/{pduSessionId})
// 3GPP: TS 29.503 Nudm_UECM — UpdateSmfRegistration
func (s *Service) UpdateSmfRegistration(ctx context.Context, ueID string, pduSessionID int, patch *SmfRegistration) (*SmfRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if patch == nil {
		return nil, errors.NewBadRequest("uecm: missing patch body", errors.CauseMandatoryIEMissing)
	}

	row := s.db.QueryRow(ctx,
		`UPDATE udm.smf_registrations
		 SET dnn = COALESCE(NULLIF($1, ''), dnn),
		     registration_time = COALESCE(NULLIF($2, ''), registration_time)
		 WHERE supi = $3 AND pdu_session_id = $4
		 RETURNING smf_instance_id`,
		patch.Dnn, patch.RegistrationTime, ueID, pduSessionID,
	)

	var smfID string
	if err := row.Scan(&smfID); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: SMF registration not found for PDU session %d: %s", pduSessionID, ueID),
			errors.CauseContextNotFound,
		)
	}

	patch.SmfInstanceID = smfID
	patch.PduSessionID = pduSessionID
	return patch, nil
}

// RegisterSmsf3Gpp registers or updates an SMSF registration for 3GPP access.
//
// Based on: docs/sbi-api-design.md §3.3 (PUT /{ueId}/registrations/smsf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — 3GppSmsfRegistration
func (s *Service) RegisterSmsf3Gpp(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error) {
	return s.registerSmsf(ctx, ueID, accessType3GPP, reg)
}

// GetSmsf3GppRegistration retrieves the current SMSF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations/smsf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — Get3GppSmsfRegistration
func (s *Service) GetSmsf3GppRegistration(ctx context.Context, ueID string) (*SmsfRegistration, error) {
	return s.getSmsf(ctx, ueID, accessType3GPP)
}

// DeregisterSmsf3Gpp removes the SMSF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (DELETE /{ueId}/registrations/smsf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — 3GppSmsfDeregistration
func (s *Service) DeregisterSmsf3Gpp(ctx context.Context, ueID string) error {
	return s.deregisterSmsf(ctx, ueID, accessType3GPP)
}

// UpdateSmsf3GppRegistration patches the current SMSF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (PATCH /{ueId}/registrations/smsf-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — UpdateSmsf3GppRegistration
func (s *Service) UpdateSmsf3GppRegistration(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error) {
	return s.updateSmsf(ctx, ueID, accessType3GPP, patch)
}

// RegisterSmsfNon3Gpp registers or updates an SMSF registration for non-3GPP access.
//
// Based on: docs/sbi-api-design.md §3.3 (PUT /{ueId}/registrations/smsf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — Non3GppSmsfRegistration
func (s *Service) RegisterSmsfNon3Gpp(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error) {
	return s.registerSmsf(ctx, ueID, accessTypeNon3GPP, reg)
}

// GetSmsfNon3GppRegistration retrieves the current SMSF non-3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations/smsf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — GetNon3GppSmsfRegistration
func (s *Service) GetSmsfNon3GppRegistration(ctx context.Context, ueID string) (*SmsfRegistration, error) {
	return s.getSmsf(ctx, ueID, accessTypeNon3GPP)
}

// DeregisterSmsfNon3Gpp removes the SMSF non-3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (DELETE /{ueId}/registrations/smsf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — Non3GppSmsfDeregistration
func (s *Service) DeregisterSmsfNon3Gpp(ctx context.Context, ueID string) error {
	return s.deregisterSmsf(ctx, ueID, accessTypeNon3GPP)
}

// UpdateSmsfNon3GppRegistration patches the current SMSF non-3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (PATCH /{ueId}/registrations/smsf-non-3gpp-access)
// 3GPP: TS 29.503 Nudm_UECM — UpdateSmsfNon3GppRegistration
func (s *Service) UpdateSmsfNon3GppRegistration(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error) {
	return s.updateSmsf(ctx, ueID, accessTypeNon3GPP, patch)
}

// registerSmsf is the shared implementation for 3GPP/non-3GPP SMSF registration.
func (s *Service) registerSmsf(ctx context.Context, ueID, accessType string, reg *SmsfRegistration) (*SmsfRegistration, bool, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, false, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if reg == nil {
		return nil, false, errors.NewBadRequest("uecm: missing registration body", errors.CauseMandatoryIEMissing)
	}
	if reg.SmsfInstanceID == "" {
		return nil, false, errors.NewBadRequest("uecm: smsfInstanceId is required", errors.CauseMandatoryIEMissing)
	}

	var existed bool
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.smsf_registrations (supi, access_type, smsf_instance_id, plmn_id, registration_time)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (supi, access_type) DO UPDATE SET
		     smsf_instance_id = EXCLUDED.smsf_instance_id,
		     plmn_id = EXCLUDED.plmn_id,
		     registration_time = EXCLUDED.registration_time
		 RETURNING (xmax <> 0)`,
		ueID, accessType, reg.SmsfInstanceID, reg.PlmnID, reg.RegistrationTime,
	)

	if err := row.Scan(&existed); err != nil {
		return nil, false, errors.NewInternalError(fmt.Sprintf("uecm: register SMSF (%s): %s", accessType, err))
	}

	created := !existed
	return reg, created, nil
}

// getSmsf is the shared implementation for retrieving 3GPP/non-3GPP SMSF registration.
func (s *Service) getSmsf(ctx context.Context, ueID, accessType string) (*SmsfRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	row := s.db.QueryRow(ctx,
		`SELECT smsf_instance_id, plmn_id, registration_time
		 FROM udm.smsf_registrations WHERE supi = $1 AND access_type = $2`,
		ueID, accessType,
	)

	var reg SmsfRegistration
	if err := row.Scan(&reg.SmsfInstanceID, &reg.PlmnID, &reg.RegistrationTime); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: SMSF registration (%s) not found for: %s", accessType, ueID),
			errors.CauseContextNotFound,
		)
	}

	return &reg, nil
}

// deregisterSmsf is the shared implementation for 3GPP/non-3GPP SMSF deregistration.
func (s *Service) deregisterSmsf(ctx context.Context, ueID, accessType string) error {
	if err := validateUeID(ueID); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM udm.smsf_registrations WHERE supi = $1 AND access_type = $2`,
		ueID, accessType,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("uecm: deregister SMSF (%s): %s", accessType, err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("uecm: SMSF registration (%s) not found for: %s", accessType, ueID),
			errors.CauseContextNotFound,
		)
	}

	return nil
}

// updateSmsf is the shared implementation for patching 3GPP/non-3GPP SMSF registration.
func (s *Service) updateSmsf(ctx context.Context, ueID, accessType string, patch *SmsfRegistration) (*SmsfRegistration, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if patch == nil {
		return nil, errors.NewBadRequest("uecm: missing patch body", errors.CauseMandatoryIEMissing)
	}

	row := s.db.QueryRow(ctx,
		`UPDATE udm.smsf_registrations
		 SET plmn_id = COALESCE($1, plmn_id),
		     registration_time = COALESCE(NULLIF($2, ''), registration_time)
		 WHERE supi = $3 AND access_type = $4
		 RETURNING smsf_instance_id`,
		patch.PlmnID, patch.RegistrationTime, ueID, accessType,
	)

	var smsfID string
	if err := row.Scan(&smsfID); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: SMSF registration (%s) not found for: %s", accessType, ueID),
			errors.CauseContextNotFound,
		)
	}

	patch.SmsfInstanceID = smsfID
	return patch, nil
}

// GetRegistrations retrieves all current NF registrations for a subscriber.
//
// Based on: docs/sbi-api-design.md §3.3 (GET /{ueId}/registrations)
// 3GPP: TS 29.503 Nudm_UECM — GetRegistrations
func (s *Service) GetRegistrations(ctx context.Context, ueID string) (*RegistrationDataSets, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	result := &RegistrationDataSets{}

	// AMF 3GPP access
	if reg, err := s.Get3GppRegistration(ctx, ueID); err == nil {
		result.Amf3GppAccess = reg
	}

	// AMF non-3GPP access
	if reg, err := s.GetNon3GppRegistration(ctx, ueID); err == nil {
		result.AmfNon3GppAccess = reg
	}

	// SMF registrations
	if regs, err := s.GetSmfRegistration(ctx, ueID); err == nil {
		result.SmfRegistrations = regs
	}

	// SMSF 3GPP access
	if reg, err := s.GetSmsf3GppRegistration(ctx, ueID); err == nil {
		result.Smsf3GppAccess = reg
	}

	// SMSF non-3GPP access
	if reg, err := s.GetSmsfNon3GppRegistration(ctx, ueID); err == nil {
		result.SmsfNon3GppAccess = reg
	}

	return result, nil
}

// PeiUpdate updates the PEI for an AMF 3GPP access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (POST /{ueId}/registrations/amf-3gpp-access/pei-update)
// 3GPP: TS 29.503 Nudm_UECM — PeiUpdate
func (s *Service) PeiUpdate(ctx context.Context, ueID string, info *PeiUpdateInfo) error {
	if err := validateUeID(ueID); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if info == nil || info.Pei == "" {
		return errors.NewBadRequest("uecm: pei is required", errors.CauseMandatoryIEMissing)
	}

	tag, err := s.db.Exec(ctx,
		`UPDATE udm.amf_registrations SET pei = $1 WHERE supi = $2 AND access_type = $3`,
		info.Pei, ueID, accessType3GPP,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("uecm: PEI update: %s", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("uecm: 3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return nil
}

// UpdateRoamingInformation updates the roaming information for an AMF 3GPP
// access registration.
//
// Based on: docs/sbi-api-design.md §3.3 (POST /{ueId}/registrations/amf-3gpp-access/roaming-info-update)
// 3GPP: TS 29.503 Nudm_UECM — UpdateRoamingInformation
func (s *Service) UpdateRoamingInformation(ctx context.Context, ueID string, info *RoamingInfoUpdate) error {
	if err := validateUeID(ueID); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}
	if info == nil {
		return errors.NewBadRequest("uecm: missing roaming info body", errors.CauseMandatoryIEMissing)
	}

	// Note: a real implementation would update roaming-related fields.
	// For now we just verify the registration exists.
	tag, err := s.db.Exec(ctx,
		`UPDATE udm.amf_registrations SET purge_flag = purge_flag WHERE supi = $1 AND access_type = $2`,
		ueID, accessType3GPP,
	)
	if err != nil {
		return errors.NewInternalError(fmt.Sprintf("uecm: roaming info update: %s", err))
	}
	if tag.RowsAffected() == 0 {
		return errors.NewNotFound(
			fmt.Sprintf("uecm: 3GPP AMF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return nil
}

// SendRoutingInfoSm retrieves routing information for SMS delivery.
//
// Based on: docs/sbi-api-design.md §3.3 (POST /{ueId}/registrations/send-routing-info-sm)
// 3GPP: TS 29.503 Nudm_UECM — SendRoutingInfoSm
func (s *Service) SendRoutingInfoSm(ctx context.Context, ueID string, _ *RoutingInfoSmRequest) (*RoutingInfoSmResponse, error) {
	if err := validateUeID(ueID); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("uecm: invalid ueId: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	// Look up the SMSF registration (prefer 3GPP access).
	row := s.db.QueryRow(ctx,
		`SELECT smsf_instance_id FROM udm.smsf_registrations WHERE supi = $1 ORDER BY access_type LIMIT 1`,
		ueID,
	)

	var resp RoutingInfoSmResponse
	if err := row.Scan(&resp.SmsfInstanceID); err != nil {
		return nil, errors.NewNotFound(
			fmt.Sprintf("uecm: SMSF registration not found for: %s", ueID),
			errors.CauseContextNotFound,
		)
	}

	return &resp, nil
}

// parsePduSessionID parses a PDU session ID from a string path parameter.
func parsePduSessionID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id < 0 || id > 255 {
		return 0, fmt.Errorf("invalid pduSessionId: %s", s)
	}
	return id, nil
}
