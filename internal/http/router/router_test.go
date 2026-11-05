package router

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

const testOrigin = "http://localhost:5173"

func newTestRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return New(Options{
		Logger:         logger,
		AllowedOrigins: []string{testOrigin},
	})
}

type errorResponse struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func TestHealthRoute(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != response.ContentTypeJSON {
		t.Errorf("Content-Type = %q, esperado %q", got, response.ContentTypeJSON)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, esperado "ok"`, body["status"])
	}
}

func TestNotFoundReturnsJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusNotFound)
	}

	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.Error.Code != response.CodeNotFound {
		t.Errorf("código = %q, esperado %q", body.Error.Code, response.CodeNotFound)
	}
	if body.Error.Message == "" {
		t.Error("mensagem de erro não deve ser vazia")
	}
}

func TestMethodNotAllowedReturnsJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusMethodNotAllowed)
	}

	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.Error.Code != response.CodeMethodNotAllowed {
		t.Errorf("código = %q, esperado %q", body.Error.Code, response.CodeMethodNotAllowed)
	}
}

func TestRequestIDHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", "test-request-id")

	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-Id"); got != "test-request-id" {
		t.Errorf("X-Request-Id = %q, esperado %q", got, "test-request-id")
	}
}

func TestCORSPreflightAllowedOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, esperado %q", got, testOrigin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, esperado %q", got, "true")
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("origem não permitida recebeu Access-Control-Allow-Origin = %q", got)
	}
}
