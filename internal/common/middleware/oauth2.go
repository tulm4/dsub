// Package middleware provides HTTP middleware for the 5G UDM network function,
// including OAuth2 token validation, rate limiting, input validation, and
// audit logging.
//
// Based on: docs/security.md §2 (Authentication and Authorization)
// 3GPP: TS 29.500 §13.4 — OAuth2 for NF service consumers
package middleware

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
)

// OAuth2 per-service scope definitions.
//
// Based on: docs/security.md §2.2 (Per-Service Scope Definitions)
// 3GPP: TS 29.510 — NRF-issued access tokens contain these scopes
const (
	ScopeNudmSDM    = "nudm-sdm"
	ScopeNudmUEAU   = "nudm-ueau"
	ScopeNudmUECM   = "nudm-uecm"
	ScopeNudmEE     = "nudm-ee"
	ScopeNudmPP     = "nudm-pp"
	ScopeNudmNIDD   = "nudm-niddau"
	ScopeNudmMT     = "nudm-mt"
	ScopeNudmSDMSub = "nudm-sdm-sub"
	ScopeNudmReport = "nudm-report"
	ScopeNudmSDEC   = "nudm-sdec"
)

// TokenValidator is an interface for validating OAuth2 bearer tokens.
// Implementations include NRF-based JWT validation and static token
// validation for testing.
//
// Based on: docs/security.md §2.3 (Token Validation Flow)
// 3GPP: TS 29.510 §6.2.6 — Access Token Request/Response
type TokenValidator interface {
	// ValidateToken verifies the bearer token and returns the claims.
	// Returns an error if the token is invalid, expired, or revoked.
	ValidateToken(ctx context.Context, token string) (*TokenClaims, error)
}

// TokenClaims represents the claims extracted from an OAuth2 access token.
//
// Based on: docs/security.md §2.2 (Token Claims)
// 3GPP: TS 29.510 §6.2.6.2.3 — AccessTokenClaims
type TokenClaims struct {
	// Issuer is the NRF instance that issued the token (nrfInstanceId).
	Issuer string
	// Subject is the NF instance ID of the consumer.
	Subject string
	// Audience is the target NF type (e.g., "UDM").
	Audience string
	// Scope is the authorized service scope (e.g., "nudm-ueau").
	Scope string
	// Expiry is the token expiration time.
	Expiry time.Time
	// NFType is the NF type of the consumer (e.g., "AUSF", "AMF").
	NFType string
}

// IsExpired reports whether the token has expired.
func (tc *TokenClaims) IsExpired() bool {
	return time.Now().After(tc.Expiry)
}

// HasScope reports whether the token includes the given scope.
func (tc *TokenClaims) HasScope(scope string) bool {
	for _, s := range strings.Fields(tc.Scope) {
		if s == scope {
			return true
		}
	}
	return false
}

// OAuth2Middleware validates OAuth2 bearer tokens on incoming SBI requests.
//
// Based on: docs/security.md §2.3 (Token Validation Flow)
// 3GPP: TS 29.500 §13.4 — OAuth2 based authorization
type OAuth2Middleware struct {
	validator     TokenValidator
	requiredScope string
	logger        *slog.Logger
}

// NewOAuth2Middleware creates middleware that enforces OAuth2 bearer token
// validation and scope checks for a specific Nudm service.
//
// Based on: docs/security.md §2.2 (Per-Service Scope Definitions)
func NewOAuth2Middleware(validator TokenValidator, requiredScope string, logger *slog.Logger) *OAuth2Middleware {
	return &OAuth2Middleware{
		validator:     validator,
		requiredScope: requiredScope,
		logger:        logger,
	}
}

// contextKey is an unexported type for context keys in this package.
type contextKey int

const (
	// tokenClaimsKey is the context key for storing validated TokenClaims.
	tokenClaimsKey contextKey = iota
)

// ClaimsFromContext extracts the validated TokenClaims from the request context.
// Returns nil if no claims are present (e.g., when OAuth2 middleware is disabled).
func ClaimsFromContext(ctx context.Context) *TokenClaims {
	claims, _ := ctx.Value(tokenClaimsKey).(*TokenClaims)
	return claims
}

// Handler wraps the given handler with OAuth2 token validation.
// It extracts the Bearer token from the Authorization header, validates it
// using the configured TokenValidator, and checks for the required scope.
//
// On success, the validated TokenClaims are stored in the request context
// and the next handler is called.
//
// On failure, it returns the appropriate 3GPP ProblemDetails:
//   - 401 Unauthorized for missing/invalid/expired tokens
//   - 403 Forbidden for insufficient scope
//
// Based on: docs/security.md §2.3 (Token Validation Flow)
// 3GPP: TS 29.500 §13.4.1 — Authorization of NF Service Access
func (m *OAuth2Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			m.logger.Warn("missing Authorization header",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			pd := udmerrors.NewUnauthorized("missing Authorization header")
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		// Extract bearer token.
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			pd := udmerrors.NewUnauthorized("Authorization header must use Bearer scheme")
			udmerrors.WriteProblemDetails(w, pd)
			return
		}
		token := authHeader[len(bearerPrefix):]
		if token == "" {
			pd := udmerrors.NewUnauthorized("empty bearer token")
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		// Validate token.
		claims, err := m.validator.ValidateToken(r.Context(), token)
		if err != nil {
			m.logger.Warn("token validation failed",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("error", err.Error()),
			)
			pd := udmerrors.NewUnauthorized("invalid access token")
			pd.AccessTokenError = "invalid_token"
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		// Check expiry.
		if claims.IsExpired() {
			m.logger.Warn("expired access token",
				slog.String("subject", claims.Subject),
				slog.String("scope", claims.Scope),
			)
			pd := udmerrors.NewUnauthorized("access token has expired")
			pd.AccessTokenError = "invalid_token"
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		// Check scope.
		if m.requiredScope != "" && !claims.HasScope(m.requiredScope) {
			m.logger.Warn("insufficient scope",
				slog.String("subject", claims.Subject),
				slog.String("required_scope", m.requiredScope),
				slog.String("token_scope", claims.Scope),
			)
			pd := udmerrors.NewForbidden("insufficient scope for this operation")
			pd.AccessTokenError = "insufficient_scope"
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		// Store claims in context and proceed.
		ctx := context.WithValue(r.Context(), tokenClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------------------------------------------------------------------------
// StaticTokenValidator — simple token validator for testing / dev
// ---------------------------------------------------------------------------

// StaticTokenValidator validates tokens against a pre-configured map.
// This is suitable for development and testing only.
//
// Production deployments must use NRF-backed JWT validation.
type StaticTokenValidator struct {
	mu     sync.RWMutex
	tokens map[string]*TokenClaims
}

// NewStaticTokenValidator creates a validator with the given token-to-claims mapping.
func NewStaticTokenValidator(tokens map[string]*TokenClaims) *StaticTokenValidator {
	m := make(map[string]*TokenClaims, len(tokens))
	for k, v := range tokens {
		m[k] = v
	}
	return &StaticTokenValidator{tokens: m}
}

// ValidateToken looks up the token in the static map. Uses constant-time
// comparison for each candidate to prevent timing side-channels.
func (v *StaticTokenValidator) ValidateToken(_ context.Context, token string) (*TokenClaims, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	for k, claims := range v.tokens {
		if subtle.ConstantTimeCompare([]byte(k), []byte(token)) == 1 {
			return claims, nil
		}
	}
	return nil, &udmerrors.ProblemDetails{
		Status: http.StatusUnauthorized,
		Title:  "Unauthorized",
		Detail: "unknown or invalid access token",
	}
}
