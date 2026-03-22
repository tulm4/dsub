package errors

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProblemDetails_Error(t *testing.T) {
	tests := []struct {
		name string
		pd   *ProblemDetails
		want string
	}{
		{
			name: "with cause",
			pd: &ProblemDetails{
				Status: 404,
				Title:  "Not Found",
				Detail: "subscriber not found",
				Cause:  CauseUserNotFound,
			},
			want: "HTTP 404 Not Found: subscriber not found (cause: USER_NOT_FOUND)",
		},
		{
			name: "without cause",
			pd: &ProblemDetails{
				Status: 500,
				Title:  "Internal Server Error",
				Detail: "database error",
			},
			want: "HTTP 500 Internal Server Error: database error",
		},
		{
			name: "empty detail",
			pd: &ProblemDetails{
				Status: 400,
				Title:  "Bad Request",
			},
			want: "HTTP 400 Bad Request: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pd.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProblemDetails_ImplementsError(t *testing.T) {
	pd := NewNotFound("not found", CauseUserNotFound)
	var err error = pd
	if pd == nil {
		t.Fatal("expected non-nil ProblemDetails")
	}
	if !strings.Contains(err.Error(), "USER_NOT_FOUND") {
		t.Errorf("error string should contain cause code, got %q", err.Error())
	}
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name       string
		pd         *ProblemDetails
		wantStatus int
		wantTitle  string
		wantCause  string
		wantDetail string
	}{
		{
			name:       "NewBadRequest",
			pd:         NewBadRequest("invalid param", CauseMandatoryIEIncorrect),
			wantStatus: 400,
			wantTitle:  "Bad Request",
			wantCause:  CauseMandatoryIEIncorrect,
			wantDetail: "invalid param",
		},
		{
			name:       "NewUnauthorized",
			pd:         NewUnauthorized("missing token"),
			wantStatus: 401,
			wantTitle:  "Unauthorized",
			wantDetail: "missing token",
		},
		{
			name:       "NewForbidden",
			pd:         NewForbidden("access denied"),
			wantStatus: 403,
			wantTitle:  "Forbidden",
			wantDetail: "access denied",
		},
		{
			name:       "NewNotFound",
			pd:         NewNotFound("subscriber not found", CauseUserNotFound),
			wantStatus: 404,
			wantTitle:  "Not Found",
			wantCause:  CauseUserNotFound,
			wantDetail: "subscriber not found",
		},
		{
			name:       "NewConflict",
			pd:         NewConflict("registration exists", CauseModificationNotAllowed),
			wantStatus: 409,
			wantTitle:  "Conflict",
			wantCause:  CauseModificationNotAllowed,
			wantDetail: "registration exists",
		},
		{
			name:       "NewTooManyRequests",
			pd:         NewTooManyRequests("rate limit exceeded"),
			wantStatus: 429,
			wantTitle:  "Too Many Requests",
			wantDetail: "rate limit exceeded",
		},
		{
			name:       "NewInternalError",
			pd:         NewInternalError("unexpected failure"),
			wantStatus: 500,
			wantTitle:  "Internal Server Error",
			wantDetail: "unexpected failure",
		},
		{
			name:       "NewNotImplemented",
			pd:         NewNotImplemented("EAP-AKA' not supported"),
			wantStatus: 501,
			wantTitle:  "Not Implemented",
			wantDetail: "EAP-AKA' not supported",
		},
		{
			name:       "NewBadGateway",
			pd:         NewBadGateway("upstream error"),
			wantStatus: 502,
			wantTitle:  "Bad Gateway",
			wantDetail: "upstream error",
		},
		{
			name:       "NewServiceUnavailable",
			pd:         NewServiceUnavailable("overloaded"),
			wantStatus: 503,
			wantTitle:  "Service Unavailable",
			wantDetail: "overloaded",
		},
		{
			name:       "NewGatewayTimeout",
			pd:         NewGatewayTimeout("upstream timeout"),
			wantStatus: 504,
			wantTitle:  "Gateway Timeout",
			wantDetail: "upstream timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pd.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", tt.pd.Status, tt.wantStatus)
			}
			if tt.pd.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", tt.pd.Title, tt.wantTitle)
			}
			if tt.pd.Detail != tt.wantDetail {
				t.Errorf("Detail = %q, want %q", tt.pd.Detail, tt.wantDetail)
			}
			if tt.pd.Cause != tt.wantCause {
				t.Errorf("Cause = %q, want %q", tt.pd.Cause, tt.wantCause)
			}
		})
	}
}

func TestJSON_Serialization(t *testing.T) {
	pd := &ProblemDetails{
		Type:   "https://example.com/errors/not-found",
		Status: 404,
		Title:  "Not Found",
		Detail: "subscriber imsi-001010123456789 not found",
		Cause:  CauseUserNotFound,
		InvalidParams: []InvalidParam{
			{Param: "supi", Reason: "invalid SUPI format"},
		},
		Instance:          "/nudm-sdm/v2/imsi-001010123456789",
		AccessTokenError:  "invalid_token",
		SupportedFeatures: "0A",
	}

	data, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	checks := map[string]interface{}{
		"type":              "https://example.com/errors/not-found",
		"status":            float64(404),
		"title":             "Not Found",
		"detail":            "subscriber imsi-001010123456789 not found",
		"cause":             "USER_NOT_FOUND",
		"instance":          "/nudm-sdm/v2/imsi-001010123456789",
		"accessTokenError":  "invalid_token",
		"supportedFeatures": "0A",
	}

	for key, want := range checks {
		val, ok := got[key]
		if !ok {
			t.Errorf("missing key %q in JSON output", key)
			continue
		}
		if val != want {
			t.Errorf("JSON[%q] = %v, want %v", key, val, want)
		}
	}

	params, ok := got["invalidParams"].([]interface{})
	if !ok || len(params) != 1 {
		t.Fatalf("invalidParams: expected array of length 1, got %v", got["invalidParams"])
	}
	p, ok := params[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invalidParams[0]: expected map, got %T", params[0])
	}
	if p["param"] != "supi" || p["reason"] != "invalid SUPI format" {
		t.Errorf("invalidParams[0] = %v, want param=supi, reason=invalid SUPI format", p)
	}
}

func TestJSON_Roundtrip(t *testing.T) {
	original := NewNotFound("subscriber not found", CauseUserNotFound)
	original.Instance = "/nudm-sdm/v2/imsi-001010123456789"

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded ProblemDetails
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Status != original.Status {
		t.Errorf("Status = %d, want %d", decoded.Status, original.Status)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, original.Title)
	}
	if decoded.Detail != original.Detail {
		t.Errorf("Detail = %q, want %q", decoded.Detail, original.Detail)
	}
	if decoded.Cause != original.Cause {
		t.Errorf("Cause = %q, want %q", decoded.Cause, original.Cause)
	}
	if decoded.Instance != original.Instance {
		t.Errorf("Instance = %q, want %q", decoded.Instance, original.Instance)
	}
}

func TestJSON_OmitEmpty(t *testing.T) {
	pd := &ProblemDetails{
		Status: 500,
		Title:  "Internal Server Error",
		Detail: "database error",
	}

	data, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// These optional fields should be absent from the JSON output.
	absent := []string{"type", "instance", "cause", "invalidParams", "accessTokenError", "supportedFeatures"}
	for _, key := range absent {
		if _, ok := got[key]; ok {
			t.Errorf("expected key %q to be omitted, but it was present", key)
		}
	}

	// These fields should be present.
	present := []string{"status", "title", "detail"}
	for _, key := range present {
		if _, ok := got[key]; !ok {
			t.Errorf("expected key %q to be present, but it was missing", key)
		}
	}
}

func TestWriteProblemDetails(t *testing.T) {
	tests := []struct {
		name            string
		pd              *ProblemDetails
		wantStatus      int
		wantContains    []string
		wantAbsent      []string
		wantContentType string
	}{
		{
			name:            "basic 404",
			pd:              NewNotFound("subscriber not found", CauseUserNotFound),
			wantStatus:      404,
			wantContains:    []string{`"status":404`, `"title":"Not Found"`, `"cause":"USER_NOT_FOUND"`},
			wantAbsent:      []string{`"invalidParams"`},
			wantContentType: "application/problem+json",
		},
		{
			name:            "500 without cause",
			pd:              NewInternalError("unexpected failure"),
			wantStatus:      500,
			wantContains:    []string{`"status":500`, `"title":"Internal Server Error"`},
			wantAbsent:      []string{`"cause"`},
			wantContentType: "application/problem+json",
		},
		{
			name: "400 with invalid params",
			pd: func() *ProblemDetails {
				pd := NewBadRequest("invalid parameters", CauseMandatoryIEMissing)
				pd.InvalidParams = []InvalidParam{
					{Param: "supi", Reason: "missing required field"},
					{Param: "servingPlmnId", Reason: "invalid PLMN format"},
				}
				return pd
			}(),
			wantStatus:      400,
			wantContains:    []string{`"invalidParams"`, `"supi"`, `"servingPlmnId"`, `"MANDATORY_IE_MISSING"`},
			wantContentType: "application/problem+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteProblemDetails(rec, tt.pd)

			if rec.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatus)
			}

			ct := rec.Header().Get("Content-Type")
			if ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", ct, tt.wantContentType)
			}

			body := rec.Body.String()
			for _, s := range tt.wantContains {
				if !strings.Contains(body, s) {
					t.Errorf("body missing %q\nbody: %s", s, body)
				}
			}
			for _, s := range tt.wantAbsent {
				if strings.Contains(body, s) {
					t.Errorf("body should not contain %q\nbody: %s", s, body)
				}
			}

			// Verify body is valid JSON.
			var decoded ProblemDetails
			if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
				t.Errorf("response body is not valid JSON: %v\nbody: %s", err, body)
			}
		})
	}
}

func TestWriteProblemDetails_InvalidParamsFields(t *testing.T) {
	pd := NewBadRequest("validation failed", CauseMandatoryIEIncorrect)
	pd.InvalidParams = []InvalidParam{
		{Param: "authType", Reason: "unsupported authentication type"},
	}

	rec := httptest.NewRecorder()
	WriteProblemDetails(rec, pd)

	var decoded ProblemDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(decoded.InvalidParams) != 1 {
		t.Fatalf("InvalidParams length = %d, want 1", len(decoded.InvalidParams))
	}
	if decoded.InvalidParams[0].Param != "authType" {
		t.Errorf("InvalidParams[0].Param = %q, want %q", decoded.InvalidParams[0].Param, "authType")
	}
	if decoded.InvalidParams[0].Reason != "unsupported authentication type" {
		t.Errorf("InvalidParams[0].Reason = %q, want %q", decoded.InvalidParams[0].Reason, "unsupported authentication type")
	}
}

func TestCauseCodeConstants(t *testing.T) {
	// Verify cause codes match their expected string values.
	codes := map[string]string{
		"CauseAuthenticationRejected":      CauseAuthenticationRejected,
		"CauseServingNetworkNotAuthorized": CauseServingNetworkNotAuthorized,
		"CauseUserNotFound":                CauseUserNotFound,
		"CauseDataNotFound":                CauseDataNotFound,
		"CauseContextNotFound":             CauseContextNotFound,
		"CauseSubscriptionNotFound":        CauseSubscriptionNotFound,
		"CauseModificationNotAllowed":      CauseModificationNotAllowed,
		"CauseMandatoryIEIncorrect":        CauseMandatoryIEIncorrect,
		"CauseMandatoryIEMissing":          CauseMandatoryIEMissing,
		"CauseUnspecifiedNFFailure":        CauseUnspecifiedNFFailure,
		"CauseNFCongestion":                CauseNFCongestion,
		"CauseInsufficientResources":       CauseInsufficientResources,
	}

	for name, val := range codes {
		if val == "" {
			t.Errorf("cause code %s is empty", name)
		}
		// All cause codes should be uppercase with underscores.
		for _, r := range val {
			if !(r == '_' || (r >= 'A' && r <= 'Z')) {
				t.Errorf("cause code %s contains unexpected character %q in value %q", name, string(r), val)
				break
			}
		}
	}
}
