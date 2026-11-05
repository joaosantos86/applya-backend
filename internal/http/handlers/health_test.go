package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	Health(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, esperado %d", res.StatusCode, http.StatusOK)
	}

	if got := res.Header.Get("Content-Type"); got != response.ContentTypeJSON {
		t.Errorf("Content-Type = %q, esperado %q", got, response.ContentTypeJSON)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decodificar corpo JSON: %v", err)
	}

	if body.Status != "ok" {
		t.Errorf("status no corpo = %q, esperado %q", body.Status, "ok")
	}
}
