package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tulm4/dsub/internal/common/logging"
)

// ---------------------------------------------------------------------------
// PeerIdentity tests
// ---------------------------------------------------------------------------

func TestPeerIdentityFromContext_NoPeer(t *testing.T) {
	ctx := context.Background()
	id := PeerIdentityFromContext(ctx)
	if id != nil {
		t.Errorf("expected nil, got %+v", id)
	}
}

func TestPeerIdentityFromContext_WithPeer(t *testing.T) {
	expected := &PeerIdentity{
		NFInstanceID: "ausf-01.5gc.mnc001.mcc001.3gppnetwork.org",
		CommonName:   "ausf-01",
	}
	ctx := context.WithValue(context.Background(), peerIdentityKey, expected)
	got := PeerIdentityFromContext(ctx)
	if got == nil {
		t.Fatal("expected PeerIdentity, got nil")
	}
	if got.NFInstanceID != expected.NFInstanceID {
		t.Errorf("NFInstanceID = %q, want %q", got.NFInstanceID, expected.NFInstanceID)
	}
}

// ---------------------------------------------------------------------------
// MTLSMiddleware tests
// ---------------------------------------------------------------------------

func TestMTLSMiddleware_NoTLS_RequiredFails(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewMTLSMiddleware(true, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No TLS connection state.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMTLSMiddleware_NoTLS_NotRequiredPasses(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewMTLSMiddleware(false, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMTLSMiddleware_WithCert_ExtractsIdentity(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewMTLSMiddleware(true, logger)

	var gotIdentity *PeerIdentity
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity = PeerIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Create a test certificate with 3GPP-style SAN.
	cert := createTestCert(t, "ausf-01", []string{
		"ausf-01.5gc.mnc001.mcc001.3gppnetwork.org",
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotIdentity == nil {
		t.Fatal("expected PeerIdentity in context, got nil")
	}
	if gotIdentity.NFInstanceID != "ausf-01.5gc.mnc001.mcc001.3gppnetwork.org" {
		t.Errorf("NFInstanceID = %q, want %q", gotIdentity.NFInstanceID,
			"ausf-01.5gc.mnc001.mcc001.3gppnetwork.org")
	}
	if gotIdentity.CommonName != "ausf-01" {
		t.Errorf("CommonName = %q, want %q", gotIdentity.CommonName, "ausf-01")
	}
}

func TestMTLSMiddleware_WithCert_NoSANMatch(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewMTLSMiddleware(true, logger)

	var gotIdentity *PeerIdentity
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity = PeerIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Certificate without 3GPP-style SAN.
	cert := createTestCert(t, "test-service", []string{"test-service.local"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotIdentity == nil {
		t.Fatal("expected PeerIdentity in context, got nil")
	}
	// NFInstanceID should be empty when no 3GPP SAN matches.
	if gotIdentity.NFInstanceID != "" {
		t.Errorf("NFInstanceID should be empty, got %q", gotIdentity.NFInstanceID)
	}
	if gotIdentity.CommonName != "test-service" {
		t.Errorf("CommonName = %q, want %q", gotIdentity.CommonName, "test-service")
	}
}

func TestMTLSMiddleware_EmptyPeerCerts_RequiredFails(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewMTLSMiddleware(true, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{},
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// TLS Config tests
// ---------------------------------------------------------------------------

func TestTLSMinVersion(t *testing.T) {
	if TLSMinVersion != tls.VersionTLS13 {
		t.Errorf("TLSMinVersion = %d, want TLS 1.3 (%d)", TLSMinVersion, tls.VersionTLS13)
	}
}

func TestNewTLSConfig_NoCACert(t *testing.T) {
	cfg, err := NewTLSConfig(MTLSConfig{
		RequireClientCert: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %d, want TLS 1.3", cfg.MinVersion)
	}
	if cfg.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Errorf("ClientAuth = %d, want VerifyClientCertIfGiven", cfg.ClientAuth)
	}
}

func TestNewTLSConfig_RequireClientCert(t *testing.T) {
	cfg, err := NewTLSConfig(MTLSConfig{
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
}

func TestNewTLSConfig_InvalidCAFile(t *testing.T) {
	_, err := NewTLSConfig(MTLSConfig{
		CACertFile: "/nonexistent/ca.pem",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent CA file")
	}
}

// ---------------------------------------------------------------------------
// Test helper: create a self-signed certificate with the given CN and SANs
// ---------------------------------------------------------------------------

func createTestCert(t *testing.T, cn string, dnsNames []string) *x509.Certificate {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	return cert
}
