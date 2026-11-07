package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

const testAllowedOrigin = "http://localhost:5173"

// newTestCSRF monta o middleware com uma origem permitida.
func newTestCSRF() *CSRFMiddleware {
	return NewCSRFMiddleware([]string{testAllowedOrigin}, CookieConfig{})
}

// postRequest cria uma POST com origem, cookie e header CSRF opcionais.
func postRequest(origin, cookieToken, headerToken string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookieToken != "" {
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: cookieToken})
	}
	if headerToken != "" {
		req.Header.Set(csrfHeader, headerToken)
	}
	return req
}

func nextOK(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func assertCSRFFailure(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusForbidden)
	}

	var body struct {
		Error response.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.Error.Code != response.CodeCSRFFailed {
		t.Errorf("código = %q, esperado %q", body.Error.Code, response.CodeCSRFFailed)
	}
}

func TestCSRFAllowsGetWithoutToken(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET deveria passar: status = %d", rec.Code)
	}

	// O cookie do token CSRF deve ter sido criado para o frontend.
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == CSRFCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("middleware deveria criar o cookie CSRF na resposta")
	}
}

func TestCSRFAllowsMatchingOriginAndToken(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest(testAllowedOrigin, "cookie-token", "cookie-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d; corpo: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestCSRFRejectsUnknownOrigin(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest("https://evil.example.com", "token", "token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}

func TestCSRFRejectsNullOrigin(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest("null", "token", "token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}

func TestCSRFRejectsMissingHeader(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest(testAllowedOrigin, "cookie-token", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}

func TestCSRFRejectsMismatchedHeader(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest(testAllowedOrigin, "cookie-token", "outro-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}

func TestCSRFRejectsMissingCookie(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest(testAllowedOrigin, "", "header-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}

func TestCSRFAllowsNonBrowserClient(t *testing.T) {
	t.Parallel()

	// Clientes sem Origin e sem Referer (curl, SDKs) não estão sujeitos a
	// CSRF; o double-submit continua valendo.
	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest("", "token", "token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
}

func TestCSRFAcceptsAllowedReferer(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest("", "token", "token")
	req.Header.Set("Referer", testAllowedOrigin+"/login")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d; corpo: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestCSRFRejectsEvilReferer(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest("", "token", "token")
	req.Header.Set("Referer", "https://evil.example.com/page")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}

func TestCSRFRejectsMalformedReferer(t *testing.T) {
	t.Parallel()

	handler := newTestCSRF().Handler(nextOK(t))

	req := postRequest("", "token", "token")
	req.Header.Set("Referer", "isso não é uma url")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertCSRFFailure(t, rec)
}
