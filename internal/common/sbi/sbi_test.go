package sbi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WriteJSON tests
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	tests := []struct {
		name       string
		status     int
		body       any
		wantStatus int
		wantCT     string
	}{
		{
			name:       "200 OK",
			status:     http.StatusOK,
			body:       payload{Name: "alice", Age: 30},
			wantStatus: http.StatusOK,
			wantCT:     ContentTypeJSON,
		},
		{
			name:       "201 Created",
			status:     http.StatusCreated,
			body:       map[string]string{"id": "abc"},
			wantStatus: http.StatusCreated,
			wantCT:     ContentTypeJSON,
		},
		{
			name:       "404 Not Found",
			status:     http.StatusNotFound,
			body:       map[string]string{"error": "not found"},
			wantStatus: http.StatusNotFound,
			wantCT:     ContentTypeJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			if err := WriteJSON(w, tt.status, tt.body); err != nil {
				t.Fatalf("WriteJSON returned unexpected error: %v", err)
			}

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if got := w.Header().Get("Content-Type"); got != tt.wantCT {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantCT)
			}

			// Verify the body is valid JSON that round-trips correctly.
			raw, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			// json.Encoder appends a newline, so trim before comparison.
			gotBody := strings.TrimSpace(w.Body.String())
			if gotBody != string(raw) {
				t.Errorf("body = %s, want %s", gotBody, raw)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ReadJSON tests
// ---------------------------------------------------------------------------

func TestReadJSON_Valid(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	body := `{"name":"bob","age":25}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", ContentTypeJSON)

	var got payload
	if err := ReadJSON(r, &got); err != nil {
		t.Fatalf("ReadJSON returned unexpected error: %v", err)
	}
	if got.Name != "bob" || got.Age != 25 {
		t.Errorf("ReadJSON decoded = %+v, want {Name:bob Age:25}", got)
	}
}

func TestReadJSON_EmptyBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	if err := ReadJSON(r, &struct{}{}); err == nil {
		t.Fatal("ReadJSON with empty body should return error")
	}
}

func TestReadJSON_NilBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = nil
	if err := ReadJSON(r, &struct{}{}); err == nil {
		t.Fatal("ReadJSON with nil body should return error")
	}
}

func TestReadJSON_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad"))
	var v map[string]string
	if err := ReadJSON(r, &v); err == nil {
		t.Fatal("ReadJSON with invalid JSON should return error")
	}
}

func TestReadJSON_UnknownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"a","extra":1}`))
	var v payload
	if err := ReadJSON(r, &v); err == nil {
		t.Fatal("ReadJSON with unknown fields should return error")
	}
}

func TestReadJSON_ExceedsMaxSize(t *testing.T) {
	// Create a body that exceeds MaxRequestBodySize (1 MB).
	bigBody := bytes.Repeat([]byte("a"), MaxRequestBodySize+512)

	// Wrap it in valid JSON so the decoder tries to read the full stream.
	var buf bytes.Buffer
	buf.WriteString(`{"v":"`)
	buf.Write(bigBody)
	buf.WriteString(`"}`)

	r := httptest.NewRequest(http.MethodPost, "/", &buf)

	var v map[string]string
	err := ReadJSON(r, &v)
	if err == nil {
		t.Fatal("ReadJSON with oversized body should return error")
	}
}

// ---------------------------------------------------------------------------
// GetCorrelationInfo tests
// ---------------------------------------------------------------------------

func TestGetCorrelationInfo(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"present", "corr-id-123", "corr-id-123"},
		{"absent", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.value != "" {
				r.Header.Set(HeaderCorrelationInfo, tt.value)
			}
			if got := GetCorrelationInfo(r); got != tt.want {
				t.Errorf("GetCorrelationInfo = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetMessagePriority tests
// ---------------------------------------------------------------------------

func TestGetMessagePriority(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{"valid priority", "5", 5},
		{"zero priority", "0", 0},
		{"missing header", "", -1},
		{"non-numeric", "high", -1},
		{"float value", "1.5", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.value != "" {
				r.Header.Set(HeaderMessagePriority, tt.value)
			}
			if got := GetMessagePriority(r); got != tt.want {
				t.Errorf("GetMessagePriority = %d, want %d", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Header constant value tests
// ---------------------------------------------------------------------------

func TestHeaderConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"HeaderTargetAPIRoot", HeaderTargetAPIRoot, "3gpp-Sbi-Target-apiRoot"},
		{"HeaderCallback", HeaderCallback, "3gpp-Sbi-Callback"},
		{"HeaderCorrelationInfo", HeaderCorrelationInfo, "3gpp-Sbi-Correlation-Info"},
		{"HeaderOCI", HeaderOCI, "3gpp-Sbi-Oci"},
		{"HeaderLCI", HeaderLCI, "3gpp-Sbi-Lci"},
		{"HeaderMessagePriority", HeaderMessagePriority, "3gpp-Sbi-Message-Priority"},
		{"HeaderMaxRspTime", HeaderMaxRspTime, "3gpp-Sbi-Max-Rsp-Time"},
		{"HeaderRoutingBinding", HeaderRoutingBinding, "3gpp-Sbi-Routing-Binding"},
		{"ContentTypeJSON", ContentTypeJSON, "application/json"},
		{"ContentTypeProblemJSON", ContentTypeProblemJSON, "application/problem+json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Timeout constant value tests
// ---------------------------------------------------------------------------

func TestTimeoutConstants(t *testing.T) {
	if DefaultConnectTimeout != 1*time.Second {
		t.Errorf("DefaultConnectTimeout = %v, want 1s", DefaultConnectTimeout)
	}
	if DefaultReadTimeout != 3*time.Second {
		t.Errorf("DefaultReadTimeout = %v, want 3s", DefaultReadTimeout)
	}
	if DefaultRequestTimeout != 5*time.Second {
		t.Errorf("DefaultRequestTimeout = %v, want 5s", DefaultRequestTimeout)
	}
	if MaxRequestBodySize != 1<<20 {
		t.Errorf("MaxRequestBodySize = %d, want %d", MaxRequestBodySize, 1<<20)
	}
}
