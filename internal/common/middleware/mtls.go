// mTLS middleware for the 5G UDM network function.
//
// Based on: docs/security.md §3 (Transport Security)
// Based on: docs/security.md §2.4 (NF Identity Cross-Check)
// 3GPP: TS 29.500 §5.2.3 — TLS mutual authentication for SBI
// 3GPP: TS 33.501 §13.1 — Mutual authentication between NFs
package middleware

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"os"
	"strings"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
)

// TLSMinVersion is the minimum TLS version allowed for SBI interfaces.
// Per docs/security.md §3.1, TLS 1.3 is mandatory.
//
// 3GPP: TS 33.501 §13.1 — TLS 1.3 requirement
const TLSMinVersion = tls.VersionTLS13

// mTLSContextKey is the context key for storing the verified peer identity.
const (
	peerIdentityKey contextKey = iota + 10
)

// PeerIdentity holds the verified identity of the mTLS client (NF consumer).
//
// Based on: docs/security.md §2.4 (NF Identity Cross-Check)
// 3GPP: TS 29.510 §6.2.6 — NF identity from certificate SAN
type PeerIdentity struct {
	// NFInstanceID is the NF instance identifier extracted from the client
	// certificate SAN (e.g., "ausf-01.5gc.mnc001.mcc001.3gppnetwork.org").
	NFInstanceID string
	// CommonName is the CN from the client certificate subject.
	CommonName string
	// DNSNames lists all SAN DNS names from the client certificate.
	DNSNames []string
	// SerialNumber is the certificate serial number (hex-encoded).
	SerialNumber string
}

// PeerIdentityFromContext extracts the verified PeerIdentity from the request
// context. Returns nil if mTLS middleware is not active or no client cert was
// presented.
func PeerIdentityFromContext(ctx context.Context) *PeerIdentity {
	id, _ := ctx.Value(peerIdentityKey).(*PeerIdentity)
	return id
}

// MTLSConfig holds configuration for the mTLS middleware.
//
// Based on: docs/security.md §3 (Transport Security)
type MTLSConfig struct {
	// CACertFile is the path to the CA certificate bundle used to verify
	// client certificates. In production, this is the operator's private PKI CA.
	CACertFile string
	// RequireClientCert controls whether a client certificate is mandatory.
	// Set to true in production; may be false in development.
	RequireClientCert bool
}

// MTLSMiddleware verifies client TLS certificates on incoming SBI requests
// and extracts the NF identity from the certificate SAN.
//
// Based on: docs/security.md §3 (Transport Security)
// Based on: docs/security.md §2.4 (mTLS SAN validation)
// 3GPP: TS 29.500 §5.2.3 — TLS mutual authentication
type MTLSMiddleware struct {
	requireClientCert bool
	logger            *slog.Logger
}

// NewMTLSMiddleware creates mTLS verification middleware.
//
// Based on: docs/security.md §3 (Transport Security)
func NewMTLSMiddleware(requireClientCert bool, logger *slog.Logger) *MTLSMiddleware {
	return &MTLSMiddleware{
		requireClientCert: requireClientCert,
		logger:            logger,
	}
}

// Handler wraps the given handler with mTLS client certificate verification.
// It extracts the NF identity from the peer certificate SAN and stores it in
// the request context for downstream handlers to consume.
//
// When RequireClientCert is true, requests without a verified client certificate
// receive a 401 Unauthorized response.
//
// Based on: docs/security.md §3 (Transport Security)
// 3GPP: TS 33.501 §13.1 — Mutual authentication between NFs
func (m *MTLSMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if TLS connection exists with peer certificates.
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			if m.requireClientCert {
				m.logger.Warn("missing client certificate",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				pd := udmerrors.NewUnauthorized("client certificate required for mTLS")
				udmerrors.WriteProblemDetails(w, pd)
				return
			}
			// Client cert not required — proceed without peer identity.
			next.ServeHTTP(w, r)
			return
		}

		// Extract identity from the first (leaf) peer certificate.
		cert := r.TLS.PeerCertificates[0]
		identity := &PeerIdentity{
			CommonName:   cert.Subject.CommonName,
			DNSNames:     cert.DNSNames,
			SerialNumber: cert.SerialNumber.Text(16),
		}

		// Extract NF instance ID from SAN DNS names.
		// Per docs/security.md §3.2, the SAN format is:
		//   <nfInstanceId>.5gc.mnc<MNC>.mcc<MCC>.3gppnetwork.org
		for _, dns := range cert.DNSNames {
			if strings.Contains(dns, ".5gc.") && strings.HasSuffix(dns, ".3gppnetwork.org") {
				identity.NFInstanceID = dns
				break
			}
		}

		m.logger.Debug("mTLS peer verified",
			slog.String("nf_instance_id", identity.NFInstanceID),
			slog.String("common_name", identity.CommonName),
			slog.String("serial_number", identity.SerialNumber),
		)

		// Store identity in context.
		ctx := context.WithValue(r.Context(), peerIdentityKey, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// NewTLSConfig creates a *tls.Config suitable for a UDM SBI server with mTLS.
// The returned config enforces TLS 1.3 minimum and optionally requires client
// certificates verified against the provided CA bundle.
//
// Based on: docs/security.md §3.1 (TLS 1.3 Mandate)
// Based on: docs/security.md §3.3 (Cipher Suite Selection)
// 3GPP: TS 33.501 §13.1 — TLS configuration for SBI
func NewTLSConfig(cfg MTLSConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: TLSMinVersion,
	}

	if cfg.CACertFile != "" {
		caCert, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, &os.PathError{Op: "read CA cert", Path: cfg.CACertFile, Err: err}
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, &os.PathError{Op: "parse CA cert", Path: cfg.CACertFile, Err: os.ErrInvalid}
		}
		tlsCfg.ClientCAs = caPool
	}

	if cfg.RequireClientCert {
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		tlsCfg.ClientAuth = tls.VerifyClientCertIfGiven
	}

	return tlsCfg, nil
}
