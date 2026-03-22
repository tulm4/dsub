package ueau

// Business logic layer for the Nudm_UEAU service.
//
// Based on: docs/service-decomposition.md §2.1 (udm-ueau)
// 3GPP: TS 29.503 Nudm_UEAU — UE Authentication service operations
// 3GPP: TS 33.501 §6.1.3 — 5G-AKA authentication procedure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the UEAU service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// SUCIResolver resolves a SUCI to a SUPI via the udm-ueid service.
// When nil, SUCI de-concealment is not available and requests with SUCI
// identifiers return 501 Not Implemented.
//
// Based on: docs/service-decomposition.md §2.10 (udm-ueid)
// 3GPP: TS 33.501 §6.12 — SUCI de-concealment
type SUCIResolver interface {
	ResolveSUCI(ctx context.Context, suci string) (supi string, err error)
}

// Service implements the Nudm_UEAU business logic.
//
// Based on: docs/service-decomposition.md §2.1
type Service struct {
	db           DB
	suciResolver SUCIResolver
}

// NewService creates a new UEAU service with the given database dependency.
// opts may contain a SUCIResolver for SUCI de-concealment support (Phase 4).
func NewService(db DB, opts ...ServiceOption) *Service {
	s := &Service{db: db}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ServiceOption configures optional dependencies on the UEAU Service.
type ServiceOption func(*Service)

// WithSUCIResolver attaches a SUCI resolver to the UEAU service,
// enabling SUCI-based authentication requests.
//
// Based on: docs/service-decomposition.md §2.1, §2.10
func WithSUCIResolver(resolver SUCIResolver) ServiceOption {
	return func(s *Service) {
		s.suciResolver = resolver
	}
}

// authCredentials holds subscriber authentication credentials read from the DB.
type authCredentials struct {
	SUPI       string
	AuthMethod string
	K          []byte
	OPc        []byte
	SQN        string
	AMFValue   string
}

// GenerateAuthData implements the GenerateAuthData operation per TS 29.503.
// It retrieves authentication credentials, generates a 5G authentication vector,
// and increments the SQN.
//
// Based on: docs/sequence-diagrams.md §2 (5G UE Registration Flow)
// 3GPP: TS 29.503 Nudm_UEAU — GenerateAuthData
// 3GPP: TS 33.501 §6.1.3.2 — Authentication vector generation
func (s *Service) GenerateAuthData(ctx context.Context, supiOrSuci string, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
	if req == nil {
		return nil, errors.NewBadRequest("missing request body", errors.CauseMandatoryIEMissing)
	}

	if req.ServingNetworkName == "" {
		return nil, errors.NewBadRequest("servingNetworkName is required", errors.CauseMandatoryIEMissing)
	}

	// Resolve identifier: if SUCI, use the udm-ueid service for de-concealment.
	// If no SUCI resolver is configured, return 501 Not Implemented.
	//
	// Based on: docs/sequence-diagrams.md §2 (SUCI resolution in registration flow)
	// 3GPP: TS 33.501 §6.12 — SUCI de-concealment
	var supi string
	if identifiers.IsSUCI(supiOrSuci) {
		if s.suciResolver == nil {
			return nil, errors.NewNotImplemented("SUCI de-concealment not yet configured")
		}
		resolved, resolveErr := s.suciResolver.ResolveSUCI(ctx, supiOrSuci)
		if resolveErr != nil {
			return nil, resolveErr
		}
		supi = resolved
	} else {
		supi = supiOrSuci
	}
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	// Retrieve authentication credentials from database
	creds, err := s.getAuthCredentials(ctx, supi)
	if err != nil {
		return nil, err
	}

	// Check auth method supports 5G-AKA
	if creds.AuthMethod != "5G_AKA" && creds.AuthMethod != "EAP_AKA_PRIME" {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("unsupported auth method: %s", creds.AuthMethod),
			errors.CauseAuthenticationRejected,
		)
	}

	// Decode SQN from hex string to bytes (6 bytes = 48 bits)
	sqnBytes, err := hex.DecodeString(creds.SQN)
	if err != nil || len(sqnBytes) != 6 {
		return nil, errors.NewInternalError("invalid SQN in database")
	}

	// Decode AMF value from hex string to bytes (2 bytes = 16 bits)
	amfBytes, err := hex.DecodeString(creds.AMFValue)
	if err != nil || len(amfBytes) != 2 {
		return nil, errors.NewInternalError("invalid AMF value in database")
	}

	// Generate random RAND (128-bit)
	randBytes := make([]byte, 16)
	if _, randErr := rand.Read(randBytes); randErr != nil {
		return nil, errors.NewInternalError("failed to generate RAND")
	}

	// Handle resynchronization if present
	if req.ResynchronizationInfo != nil {
		decodedRand, decErr := hex.DecodeString(req.ResynchronizationInfo.Rand)
		if decErr != nil || len(decodedRand) != 16 {
			return nil, errors.NewBadRequest("invalid RAND in resynchronizationInfo", errors.CauseMandatoryIEIncorrect)
		}
		randBytes = decodedRand
		// AUTS processing for SQN resync would be done here in a full implementation.
		// For now, we use the existing SQN.
	}

	// Generate authentication vector using Milenage
	av, err := GenerateAuthVector(creds.K, creds.OPc, sqnBytes, amfBytes, randBytes)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Sprintf("auth vector generation failed: %s", err))
	}

	// Derive 5G keys
	sqnXorAK := make([]byte, 6)
	for i := 0; i < 6; i++ {
		sqnXorAK[i] = sqnBytes[i] ^ av.AK[i]
	}
	xresStar := DeriveXResStar(av.CK, av.IK, req.ServingNetworkName, randBytes, av.XRES)
	kausf := DeriveKausf(av.CK, av.IK, req.ServingNetworkName, sqnXorAK)

	// Atomically increment the SQN using optimistic locking. The WHERE clause
	// checks the old SQN value so that concurrent callers cannot reuse the same
	// sequence number (prevents replay). If a concurrent caller already
	// incremented the SQN, this UPDATE matches zero rows → ErrNoRows → retry.
	newSQN := incrementSQN(sqnBytes)
	if err := s.updateSQNOptimistic(ctx, supi, creds.SQN, hex.EncodeToString(newSQN)); err != nil {
		return nil, errors.NewInternalError("failed to update SQN (concurrent modification)")
	}

	result := &AuthenticationInfoResult{
		AuthType: creds.AuthMethod,
		AuthenticationVector: &AuthenticationVector{
			AvType:   "5G_HE_AKA",
			Rand:     hex.EncodeToString(randBytes),
			Autn:     hex.EncodeToString(av.AUTN),
			XresStar: hex.EncodeToString(xresStar),
			Kausf:    hex.EncodeToString(kausf),
		},
		Supi: supi,
	}

	return result, nil
}

// ConfirmAuth implements the ConfirmAuth operation per TS 29.503.
// It stores the authentication event result.
//
// 3GPP: TS 29.503 Nudm_UEAU — ConfirmAuth
func (s *Service) ConfirmAuth(ctx context.Context, supi string, event *AuthEvent) (*AuthEvent, error) {
	if event == nil {
		return nil, errors.NewBadRequest("missing request body", errors.CauseMandatoryIEMissing)
	}

	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	if event.NfInstanceID == "" {
		return nil, errors.NewBadRequest("nfInstanceId is required", errors.CauseMandatoryIEMissing)
	}
	if event.AuthType == "" {
		return nil, errors.NewBadRequest("authType is required", errors.CauseMandatoryIEMissing)
	}
	if event.ServingNetworkName == "" {
		return nil, errors.NewBadRequest("servingNetworkName is required", errors.CauseMandatoryIEMissing)
	}

	// Set timestamp if not provided
	if event.TimeStamp == "" {
		event.TimeStamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Insert auth event into database
	if err := s.insertAuthEvent(ctx, supi, event); err != nil {
		return nil, errors.NewInternalError("failed to store auth event")
	}

	return event, nil
}

// DeleteAuthEvent implements the DeleteAuth operation per TS 29.503.
// It removes an authentication event (PUT with authRemovalInd=true).
//
// 3GPP: TS 29.503 Nudm_UEAU — DeleteAuth
func (s *Service) DeleteAuthEvent(ctx context.Context, supi, authEventID string) error {
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return errors.NewBadRequest(
			fmt.Sprintf("invalid SUPI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	if authEventID == "" {
		return errors.NewBadRequest("authEventId is required", errors.CauseMandatoryIEMissing)
	}

	// Delete from authentication_status table
	row := s.db.QueryRow(ctx,
		"DELETE FROM udm.authentication_status WHERE supi = $1 AND serving_network_name = $2 RETURNING supi",
		supi, authEventID,
	)
	var deleted string
	if err := row.Scan(&deleted); err != nil {
		return errors.NewNotFound("auth event not found", errors.CauseDataNotFound)
	}

	return nil
}

// getAuthCredentials retrieves authentication credentials for a subscriber.
func (s *Service) getAuthCredentials(ctx context.Context, supi string) (*authCredentials, error) {
	row := s.db.QueryRow(ctx,
		"SELECT supi, auth_method, k_key, opc_key, sqn, amf_value FROM udm.authentication_data WHERE supi = $1",
		supi,
	)

	var creds authCredentials
	err := row.Scan(&creds.SUPI, &creds.AuthMethod, &creds.K, &creds.OPc, &creds.SQN, &creds.AMFValue)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("authentication data not found for SUPI: %s", supi),
				errors.CauseUserNotFound,
			)
		}
		return nil, errors.NewInternalError(
			fmt.Sprintf("failed to query authentication data for SUPI %s: %s", supi, err),
		)
	}

	return &creds, nil
}

// updateSQNOptimistic atomically updates the SQN for a subscriber using
// optimistic locking. The UPDATE only matches when the current SQN equals
// oldSQN, preventing concurrent callers from reusing the same sequence number.
//
// 3GPP: TS 33.501 §6.1.3.4 — SQN management (replay protection)
func (s *Service) updateSQNOptimistic(ctx context.Context, supi, oldSQN, newSQN string) error {
	row := s.db.QueryRow(ctx,
		"UPDATE udm.authentication_data SET sqn = $1, updated_at = NOW() WHERE supi = $2 AND sqn = $3 RETURNING supi",
		newSQN, supi, oldSQN,
	)
	var updated string
	return row.Scan(&updated)
}

// insertAuthEvent stores an authentication event in the database.
func (s *Service) insertAuthEvent(ctx context.Context, supi string, event *AuthEvent) error {
	row := s.db.QueryRow(ctx,
		`INSERT INTO udm.authentication_status (supi, serving_network_name, auth_type, success, time_stamp, auth_removal_ind, nf_instance_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (supi, serving_network_name) DO UPDATE SET
		   auth_type = EXCLUDED.auth_type,
		   success = EXCLUDED.success,
		   time_stamp = EXCLUDED.time_stamp,
		   auth_removal_ind = EXCLUDED.auth_removal_ind,
		   nf_instance_id = EXCLUDED.nf_instance_id
		 RETURNING supi`,
		supi, event.ServingNetworkName, event.AuthType, event.Success,
		event.TimeStamp, event.AuthRemovalInd, event.NfInstanceID,
	)
	var inserted string
	return row.Scan(&inserted)
}

// incrementSQN increments a 48-bit sequence number by 1.
// The input must be exactly 6 bytes of valid SQN data.
func incrementSQN(sqn []byte) []byte {
	val := uint64(0)
	for _, b := range sqn {
		val = (val << 8) | uint64(b)
	}
	val++
	// Mask to 48 bits to handle overflow
	val &= 0xFFFFFFFFFFFF
	newSQN := make([]byte, 6)
	newSQN[0] = byte(val >> 40)
	newSQN[1] = byte(val >> 32)
	newSQN[2] = byte(val >> 24)
	newSQN[3] = byte(val >> 16)
	newSQN[4] = byte(val >> 8)
	newSQN[5] = byte(val)
	return newSQN
}
