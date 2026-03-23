// Input validation middleware for the 5G UDM network function.
//
// Based on: docs/security.md §8.1 (Input Validation)
// 3GPP: TS 29.500 §5.2.7 — Error handling for malformed requests

package middleware

import (
	"log/slog"
	"mime"
	"net/http"
	"strings"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/sbi"
)

// InputValidationMiddleware enforces content-type, request size limits, and
// basic structural validation on all incoming SBI requests.
//
// Based on: docs/security.md §8.1 (Input Validation)
// 3GPP: TS 29.500 §5.2.7 — Error Signaling
type InputValidationMiddleware struct {
	maxBodySize int64
	logger      *slog.Logger
}

// NewInputValidationMiddleware creates input validation middleware.
// maxBodySize specifies the maximum allowed request body size in bytes
// (0 uses the default from sbi.MaxRequestBodySize = 1 MB).
//
// Based on: docs/security.md §8.1 (Request Body Size Limits)
func NewInputValidationMiddleware(maxBodySize int64, logger *slog.Logger) *InputValidationMiddleware {
	if maxBodySize <= 0 {
		maxBodySize = sbi.MaxRequestBodySize
	}
	return &InputValidationMiddleware{
		maxBodySize: maxBodySize,
		logger:      logger,
	}
}

// Handler wraps the given handler with input validation checks:
//   - Content-Type must be application/json for requests with a body (POST/PUT/PATCH)
//   - Content-Length must not exceed maxBodySize
//   - Request body is limited to maxBodySize (defense in depth)
//
// Based on: docs/security.md §8.1 (Input Validation)
// 3GPP: TS 29.500 §5.2.3.2 — Content type requirements
func (m *InputValidationMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For methods that carry a body, validate Content-Type.
		if requiresBody(r.Method) && r.ContentLength != 0 {
			ct := r.Header.Get("Content-Type")
			if ct == "" {
				m.logger.Warn("missing Content-Type header",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				pd := udmerrors.NewBadRequest(
					"Content-Type header is required for "+r.Method+" requests",
					udmerrors.CauseMandatoryIEMissing,
				)
				udmerrors.WriteProblemDetails(w, pd)
				return
			}

			mediaType, _, err := mime.ParseMediaType(ct)
			if err != nil || !isJSONMediaType(mediaType) {
				m.logger.Warn("unsupported Content-Type",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("content_type", ct),
				)
				pd := &udmerrors.ProblemDetails{
					Status: http.StatusUnsupportedMediaType,
					Title:  "Unsupported Media Type",
					Detail: "Content-Type must be application/json",
					Cause:  udmerrors.CauseMandatoryIEIncorrect,
				}
				udmerrors.WriteProblemDetails(w, pd)
				return
			}
		}

		// Enforce Content-Length limit.
		if r.ContentLength > m.maxBodySize {
			m.logger.Warn("request body too large",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int64("content_length", r.ContentLength),
				slog.Int64("max_body_size", m.maxBodySize),
			)
			pd := &udmerrors.ProblemDetails{
				Status: http.StatusRequestEntityTooLarge,
				Title:  "Payload Too Large",
				Detail: "request body exceeds maximum allowed size",
				Cause:  udmerrors.CauseMandatoryIEIncorrect,
			}
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		// Limit the body reader as defense-in-depth (Content-Length can be
		// missing or spoofed).
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, m.maxBodySize)
		}

		next.ServeHTTP(w, r)
	})
}

// requiresBody returns true for HTTP methods that typically carry a request body.
func requiresBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	}
	return false
}

// isJSONMediaType returns true for JSON-compatible media types.
func isJSONMediaType(mediaType string) bool {
	return mediaType == "application/json" ||
		mediaType == "application/problem+json" ||
		strings.HasSuffix(mediaType, "+json")
}
