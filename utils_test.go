package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"123", "***"},
		{"123456", "***"},
		{"123456789", "123***789"},
		{"sk-abc123def456", "sk-***456"},
	}

	for _, tt := range tests {
		result := maskSecret(tt.input)
		if result != tt.expected {
			t.Errorf("maskSecret(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, `{"status":"ok"}`)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	body := strings.TrimSpace(w.Body.String())
	expected := `{"status":"ok"}`
	if body != expected {
		t.Errorf("Expected body %s, got %s", expected, body)
	}
}

func TestRequirePost(t *testing.T) {
	tests := []struct {
		method   string
		expected bool
		status   int
	}{
		{"POST", true, http.StatusOK},
		{"GET", false, http.StatusMethodNotAllowed},
		{"PUT", false, http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tt.method, "/test", nil)
		result := requirePost(w, r)

		if result != tt.expected {
			t.Errorf("requirePost with method %s returned %v, expected %v", tt.method, result, tt.expected)
		}

		if !tt.expected && w.Code != tt.status {
			t.Errorf("requirePost with method %s returned status %d, expected %d", tt.method, w.Code, tt.status)
		}
	}
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		input    map[string]interface{}
		expected float64
	}{
		{nil, 0},
		{map[string]interface{}{}, 0},
		{map[string]interface{}{"errcode": nil}, 0},
		{map[string]interface{}{"errcode": 0}, 0},
		{map[string]interface{}{"errcode": 42001}, 42001},
		{map[string]interface{}{"errcode": 40001}, 40001},
	}

	for _, tt := range tests {
		result := getErrorCode(tt.input)
		if result != tt.expected {
			t.Errorf("getErrorCode(%v) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestNormalizeAppMsgType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "text"},
		{"text", "text"},
		{"TEXT", "text"},
		{"  markdown  ", "markdown"},
		{"image", "image"},
	}

	for _, tt := range tests {
		result := normalizeAppMsgType(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeAppMsgType(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestValidateExternalRequestBody(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    ExternalRequestBody
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "empty external_userids",
			requestBody:    ExternalRequestBody{ExternalUserIds: []string{}},
			expectedStatus: 400,
			expectedMsg:    `{"errcode":40003,"errmsg":"external_userid is required"}`,
		},
		{
			name:           "too many external_userids",
			requestBody:    ExternalRequestBody{ExternalUserIds: make([]string, 1001)},
			expectedStatus: 400,
			expectedMsg:    `{"errcode":40005,"errmsg":"external_userid exceeds limit 1000"}`,
		},
		{
			name:           "empty sender",
			requestBody:    ExternalRequestBody{ExternalUserIds: []string{"wxid_123"}, Sender: ""},
			expectedStatus: 400,
			expectedMsg:    `{"errcode":40004,"errmsg":"sender is required"}`,
		},
		{
			name:           "valid request with text",
			requestBody:    ExternalRequestBody{ExternalUserIds: []string{"wxid_123"}, Sender: "zhangsan", MsgType: "text", Text: &Msg{Content: "hello"}},
			expectedStatus: 0,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := validateExternalRequestBody(&tt.requestBody)
			if status != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, status)
			}
			if msg != tt.expectedMsg {
				t.Errorf("expected msg %q, got %q", tt.expectedMsg, msg)
			}
		})
	}
}

func TestRecoverMiddleware(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	wrapped := recoverMiddleware(panicHandler)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)

	wrapped(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != `{"errcode":50000,"errmsg":"internal server error"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestRequirePostWithMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	wrapped := recoverMiddleware(handler)

	t.Run("POST request", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/test", nil)
		wrapped(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("GET request", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/test", nil)
		wrapped(w, r)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}

func BenchmarkMaskSecret(b *testing.B) {
	for i := 0; i < b.N; i++ {
		maskSecret("sk-abc123def456ghi789")
	}
}

func BenchmarkNormalizeAppMsgType(b *testing.B) {
	for i := 0; i < b.N; i++ {
		normalizeAppMsgType("  MARKDOWN  ")
	}
}

func BenchmarkValidateExternalRequestBody(b *testing.B) {
	reqBody := ExternalRequestBody{
		ExternalUserIds: []string{"wxid_123", "wxid_456"},
		Sender:          "zhangsan",
		MsgType:         "text",
		Text:            &Msg{Content: "benchmark test"},
	}

	for i := 0; i < b.N; i++ {
		validateExternalRequestBody(&reqBody)
	}
}
