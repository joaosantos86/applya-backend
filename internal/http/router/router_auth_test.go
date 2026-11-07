package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/http/handlers"
	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// fakeService implementa handlers.AuthService para os testes de roteamento.
type fakeService struct {
	registerFn func(ctx context.Context, name, email, password, clientIP string) (auth.PublicUser, bool, error)
}

func (f *fakeService) Register(ctx context.Context, name, email, password, clientIP string) (auth.PublicUser, bool, error) {
	return f.registerFn(ctx, name, email, password, clientIP)
}

func (f *fakeService) VerifyEmail(context.Context, string, string) error { return nil }
func (f *fakeService) ResendVerification(context.Context, string) error  { return nil }
func (f *fakeService) Login(context.Context, string, string, string) (auth.PublicUser, string, error) {
	return auth.PublicUser{}, "", nil
}
func (f *fakeService) Logout(context.Context, string) error                        { return nil }
func (f *fakeService) ForgotPassword(context.Context, string, string) error        { return nil }
func (f *fakeService) ResetPassword(context.Context, string, string, string) error { return nil }

// fakeFinder implementa auth.SessionFinder para o RequireAuth.
type fakeFinder struct {
	user auth.User
	err  error
}

func (f fakeFinder) FindUser(context.Context, string) (auth.User, error) { return f.user, f.err }

func newAuthRouter(service handlers.AuthService, finder auth.SessionFinder) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cookieCfg := auth.CookieConfig{}
	authHandler := handlers.NewAuthHandler(service, cookieCfg, time.Hour, logger)
	csrf := auth.NewCSRFMiddleware([]string{testOrigin}, cookieCfg)

	return New(Options{
		Logger:         logger,
		AllowedOrigins: []string{testOrigin},
		Auth: AuthOptions{
			Handler:     authHandler,
			RequireAuth: auth.RequireAuth(finder, cookieCfg, logger),
			CSRF:        csrf.Handler,
		},
	})
}

func TestAuthCSRFEndpoint(t *testing.T) {
	t.Parallel()

	r := newAuthRouter(&fakeService{}, fakeFinder{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}

	var body struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.CSRFToken == "" {
		t.Error("csrf_token não deveria ser vazio")
	}
}

func TestAuthMeRequiresSession(t *testing.T) {
	t.Parallel()

	r := newAuthRouter(&fakeService{}, fakeFinder{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}

	var body struct {
		Error response.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.Error.Code != response.CodeAuthenticationRequired {
		t.Errorf("código = %q, esperado %q", body.Error.Code, response.CodeAuthenticationRequired)
	}
}

func TestAuthMeWithValidSession(t *testing.T) {
	t.Parallel()

	r := newAuthRouter(&fakeService{}, fakeFinder{
		user: auth.User{ID: "id-1", Name: "Ana", Email: "ana@example.com"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "token-valido"})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d; corpo: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), `"email":"ana@example.com"`) {
		t.Errorf("resposta inesperada: %s", rec.Body.String())
	}
}

func TestAuthRegisterProtectedByCSRF(t *testing.T) {
	t.Parallel()

	called := false
	service := &fakeService{
		registerFn: func(context.Context, string, string, string, string) (auth.PublicUser, bool, error) {
			called = true
			return auth.PublicUser{ID: "id-1", Name: "Ana", Email: "ana@example.com"}, true, nil
		},
	}

	r := newAuthRouter(service, fakeFinder{})

	t.Run("sem token CSRF é rejeitado", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
			strings.NewReader(`{"name":"Ana","email":"ana@example.com","password":"senha-forte"}`))
		req.Header.Set("Origin", testOrigin)

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusForbidden)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
			t.Errorf("Access-Control-Allow-Origin = %q, esperado %q", got, testOrigin)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Access-Control-Allow-Credentials = %q, esperado %q", got, "true")
		}
		if called {
			t.Fatal("serviço não deveria ter sido chamado sem CSRF")
		}
	})

	t.Run("com token CSRF válido passa", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
			strings.NewReader(`{"name":"Ana","email":"ana@example.com","password":"senha-forte"}`))
		req.Header.Set("Origin", testOrigin)
		req.Header.Set("X-CSRF-Token", "token-csrf")
		req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "token-csrf"})

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, esperado %d; corpo: %s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		if !called {
			t.Fatal("serviço deveria ter sido chamado")
		}
	})
}

func TestAuthRegisterPreflightAllowsCSRFHeader(t *testing.T) {
	t.Parallel()

	r := newAuthRouter(&fakeService{}, fakeFinder{})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/register", nil)
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado 200 ou 204; corpo: %s", rec.Code, rec.Body.String())
	}

	wantHeaders := map[string]string{
		"Access-Control-Allow-Origin":      testOrigin,
		"Access-Control-Allow-Credentials": "true",
	}
	for header, want := range wantHeaders {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, esperado %q", header, got, want)
		}
	}

	allowedHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range []string{"content-type", "x-csrf-token"} {
		if !strings.Contains(allowedHeaders, header) {
			t.Errorf("Access-Control-Allow-Headers = %q; deveria conter %q", allowedHeaders, header)
		}
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), http.MethodPost) {
		t.Errorf("Access-Control-Allow-Methods = %q; deveria conter %q", rec.Header().Get("Access-Control-Allow-Methods"), http.MethodPost)
	}
}

func TestAuthRoutesAbsentWithoutAuthOptions(t *testing.T) {
	t.Parallel()

	// O roteador sem configuração de autenticação não expõe /api/v1/auth.
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusNotFound)
	}
}
