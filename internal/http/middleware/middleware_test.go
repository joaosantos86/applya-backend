package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}

	return slog.New(slog.NewJSONHandler(buf, nil)), buf
}

func TestLoggerRecordsRequest(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger()

	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/resumes", nil))

	logged := buf.String()
	for _, want := range []string{`"method":"POST"`, `"path":"/resumes"`, `"status":201`, "http request"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log não contém %q; recebido: %s", want, logged)
		}
	}
}

func TestRecovererHandlesPanic(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger()

	handler := Recoverer(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusInternalServerError)
	}

	var body struct {
		Error response.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.Error.Code != response.CodeInternal {
		t.Errorf("código = %q, esperado %q", body.Error.Code, response.CodeInternal)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("resposta não deve expor detalhes internos do panic")
	}

	if !strings.Contains(buf.String(), "panic recuperado") {
		t.Errorf("panic não foi registrado no log: %s", buf.String())
	}
}

func TestCORSAllowedOrigin(t *testing.T) {
	t.Parallel()

	handler := CORS([]string{"http://localhost:5173"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, esperado %q", got, "http://localhost:5173")
	}
}
