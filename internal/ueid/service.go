package ueid

// Business logic layer for the Nudm_UEID service.
//
// Based on: docs/service-decomposition.md §2.10 (udm-ueid)
// 3GPP: TS 29.503 Nudm_UEID — UE Identification service operations
// 3GPP: TS 33.501 §6.12 — SUCI de-concealment procedure

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/identifiers"
)

// DB defines the database operations required by the UEID service.
// This interface allows unit tests to mock database calls.
// It is compatible with *pgxpool.Pool which implements QueryRow.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Service implements the Nudm_UEID business logic.
//
// Based on: docs/service-decomposition.md §2.10
type Service struct {
	db  DB
	hsm HSMDecrypter
}

// NewService creates a new UEID service with the given database dependency
// and HSM decrypter for ECIES de-concealment.
//
// Based on: docs/security.md §4.3/§4.4 (private key operations via HSM)
func NewService(db DB, hsm HSMDecrypter) *Service {
	return &Service{db: db, hsm: hsm}
}

// ResolveSUCI resolves a SUCI to a SUPI. This method satisfies the
// ueau.SUCIResolver interface, enabling direct integration without an adapter.
//
// Based on: docs/service-decomposition.md §2.1, §2.10 (UEAU ↔ UEID integration)
// 3GPP: TS 33.501 §6.12 — SUCI de-concealment
func (s *Service) ResolveSUCI(ctx context.Context, suci string) (string, error) {
	resp, err := s.Deconceal(ctx, &SuciDeconcealRequest{Suci: suci})
	if err != nil {
		return "", err
	}
	return resp.Supi, nil
}

// Deconceal performs SUCI de-concealment to recover the SUPI.
//
// The procedure:
//  1. Parse and validate the SUCI format
//  2. Extract scheme_id and HN_pub_key_id
//  3. Look up the HPLMN key profile from suci_profiles
//  4. Perform ECIES decryption via HSM (Profile A or B)
//  5. Reconstruct and validate the SUPI from decrypted MSIN + MCC/MNC
//
// Based on: docs/security.md §4.4 (SUCI Deconceal Process Security)
// Based on: docs/sequence-diagrams.md §2 (SUCI resolution in registration flow)
// 3GPP: TS 33.501 §6.12 — SUCI de-concealment
// 3GPP: TS 29.503 Nudm_UEID — Deconceal operation
func (s *Service) Deconceal(ctx context.Context, req *SuciDeconcealRequest) (*SuciDeconcealResponse, error) {
	if req == nil {
		return nil, errors.NewBadRequest("missing request body", errors.CauseMandatoryIEMissing)
	}

	if req.Suci == "" {
		return nil, errors.NewBadRequest("suci is required", errors.CauseMandatoryIEMissing)
	}

	// Parse and validate the SUCI
	components, err := identifiers.ParseSUCI(req.Suci)
	if err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("invalid SUCI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	// Null scheme (scheme_id=0) means MSIN is not encrypted
	if components.SchemeID == 0 {
		supi := fmt.Sprintf("imsi-%s%s%s", components.MCC, components.MNC, components.EncryptedMSIN)
		// Validate the reconstructed SUPI to avoid returning malformed identifiers.
		if err := identifiers.ValidateSUPI(supi); err != nil {
			return nil, errors.NewBadRequest(
				fmt.Sprintf("invalid SUPI constructed from SUCI: %s", err),
				errors.CauseMandatoryIEIncorrect,
			)
		}
		return &SuciDeconcealResponse{Supi: supi}, nil
	}

	// Map scheme ID to profile type
	profileType, err := schemeIDToProfileType(components.SchemeID)
	if err != nil {
		return nil, errors.NewBadRequest(err.Error(), errors.CauseMandatoryIEIncorrect)
	}

	// Look up the HPLMN key profile (metadata only — private key stays in HSM)
	profile, err := s.getSUCIProfile(ctx, components.HNKeyID)
	if err != nil {
		return nil, err
	}

	// Verify profile type matches
	if profile.ProfileType != profileType {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("scheme_id %d requires profile %s but key %d is profile %s",
				components.SchemeID, profileType, components.HNKeyID, profile.ProfileType),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	// Decode the hex-encoded encrypted MSIN
	cipherData, err := hex.DecodeString(components.EncryptedMSIN)
	if err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("invalid encrypted MSIN hex: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	// Extract ephemeral public key and ciphertext+MAC from the cipher data
	ephemeralPubKey, cipherText, err := splitCipherData(profileType, cipherData)
	if err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("invalid SUCI cipher data: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	// Perform ECIES de-concealment via HSM (private key never in app memory).
	// Map deconcealment failures to client error without exposing crypto details.
	// Based on: docs/sbi-api-design.md §7 (error mapping), TS 29.503 ProblemDetails causes.
	plainMSIN, err := s.hsm.DeconcealMSIN(profile.HSMKeyRef, profileType, ephemeralPubKey, cipherText)
	if err != nil {
		return nil, errors.NewBadRequest("invalid SUCI cipher text", errors.CauseMandatoryIEIncorrect)
	}

	// Reconstruct the SUPI: imsi-<MCC><MNC><plaintext MSIN>
	supi := fmt.Sprintf("imsi-%s%s%s", components.MCC, components.MNC, string(plainMSIN))

	// Validate the reconstructed SUPI to avoid returning malformed identifiers.
	if err := identifiers.ValidateSUPI(supi); err != nil {
		return nil, errors.NewBadRequest(
			fmt.Sprintf("invalid SUPI constructed from SUCI: %s", err),
			errors.CauseMandatoryIEIncorrect,
		)
	}

	return &SuciDeconcealResponse{Supi: supi}, nil
}

// getSUCIProfile retrieves SUCI profile metadata from the database.
// Per docs/security.md §4.3/§4.4, private key material must not leave the HSM
// boundary, so this function loads only the hsm_key_ref (not raw private key bytes).
func (s *Service) getSUCIProfile(ctx context.Context, hnKeyID int) (*SUCIProfile, error) {
	row := s.db.QueryRow(ctx,
		"SELECT hn_key_id, profile_type, public_key, hsm_key_ref, is_active FROM udm.suci_profiles WHERE hn_key_id = $1",
		hnKeyID,
	)

	var profile SUCIProfile
	err := row.Scan(&profile.HNKeyID, &profile.ProfileType, &profile.PublicKey, &profile.HSMKeyRef, &profile.IsActive)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NewNotFound(
				fmt.Sprintf("SUCI profile not found for HN key ID: %d", hnKeyID),
				errors.CauseDataNotFound,
			)
		}
		return nil, errors.NewInternalError(
			fmt.Sprintf("failed to query SUCI profile for key ID %d: %s", hnKeyID, err),
		)
	}

	if !profile.IsActive {
		return nil, errors.NewNotFound(
			fmt.Sprintf("SUCI profile for HN key ID %d is not active", hnKeyID),
			errors.CauseDataNotFound,
		)
	}

	return &profile, nil
}

// schemeIDToProfileType maps a 3GPP scheme_id to an ECIES profile type.
//
// 3GPP: TS 33.501 Annex C:
//
//	scheme_id 1 → Profile A (X25519)
//	scheme_id 2 → Profile B (secp256r1)
func schemeIDToProfileType(schemeID int) (string, error) {
	switch schemeID {
	case 1:
		return ProfileA, nil
	case 2:
		return ProfileB, nil
	default:
		return "", fmt.Errorf("unsupported SUCI scheme_id: %d (must be 1 or 2)", schemeID)
	}
}

// splitCipherData extracts the ephemeral public key and ciphertext+MAC
// from the combined cipher data field in the SUCI.
//
// For Profile A: ephemeral key is 32 bytes (X25519 public key)
// For Profile B: ephemeral key is 33 bytes (compressed P-256 point)
//
// 3GPP: TS 33.501 Annex C — SUCI cipher data structure
func splitCipherData(profileType string, data []byte) (ephemeralPubKey, cipherText []byte, err error) {
	var keyLen int
	switch profileType {
	case ProfileA:
		keyLen = 32
	case ProfileB:
		keyLen = 33
	default:
		return nil, nil, fmt.Errorf("unsupported profile type: %s", profileType)
	}

	// Data must contain at least: ephemeral key + 1 byte plaintext + 8 bytes MAC
	minLen := keyLen + 1 + macLen
	if len(data) < minLen {
		return nil, nil, fmt.Errorf("cipher data too short: need at least %d bytes, got %d", minLen, len(data))
	}

	return data[:keyLen], data[keyLen:], nil
}
