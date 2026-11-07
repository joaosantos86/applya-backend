package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// fakeFinder implementa SessionFinder para os testes do middleware.
type fakeFinder struct {
	findUser func(ctx context.Context, token string) (User, error)
}

func (f fakeFinder) FindUser(ctx context.Context, token string) (User, error) {
	return f.findUser(ctx, token)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequireAuthMissingCookie(t *testing.T) {
	t.Parallel()

	handler := RequireAuth(fakeFinder{}, CookieConfig{}, discardLogger())(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

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

func TestRequireAuthValidSession(t *testing.T) {
	t.Parallel()

	wantUser := User{ID: "user-1", Name: "Ana", Email: "ana@example.com"}

	finder := fakeFinder{findUser: func(_ context.Context, token string) (User, error) {
		if token != "token-valido" {
			t.Errorf("FindUser recebeu token %q, esperado %q", token, "token-valido")
		}
		return wantUser, nil
	}}

	var gotUser User
	var gotOK bool

	handler := RequireAuth(finder, CookieConfig{}, discardLogger())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser, gotOK = UserFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token-valido"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	if !gotOK || gotUser.ID != wantUser.ID {
		t.Errorf("usuário no contexto = (%+v, %v), esperado %+v", gotUser, gotOK, wantUser)
	}
}

func TestRequireAuthInvalidSessionClearsCookie(t *testing.T) {
	t.Parallel()

	finder := fakeFinder{findUser: func(context.Context, string) (User, error) {
		return User{}, ErrSessionNotFound
	}}

	handler := RequireAuth(finder, CookieConfig{}, discardLogger())(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("handler protegido não deveria ser chamado")
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token-revogado"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("cookie inválido deveria ser removido (MaxAge=-1), recebeu %d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("cookie de sessão inválida não foi removido da resposta")
	}
}

func TestRequireAuthUnexpectedErrorIsUnauthorized(t *testing.T) {
	t.Parallel()

	// Erros inesperados do finder também viram 401 (sem detalhes ao cliente).
	finder := fakeFinder{findUser: func(context.Context, string) (User, error) {
		return User{}, errors.New("banco indisponível")
	}}

	handler := RequireAuth(finder, CookieConfig{}, discardLogger())(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("handler protegido não deveria ser chamado")
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "qualquer"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}
	if strings.Contains(rec.Body.String(), "banco indisponível") {
		t.Error("resposta não deve expor detalhes do erro interno")
	}
}

func TestUserContextRoundtrip(t *testing.T) {
	t.Parallel()

	user := User{ID: "id-1", Name: "Ana"}
	ctx := WithUser(context.Background(), user)

	got, ok := UserFromContext(ctx)
	if !ok || got.ID != "id-1" {
		t.Errorf("UserFromContext() = (%+v, %v), esperado (%+v, true)", got, ok, user)
	}

	if _, ok := UserFromContext(context.Background()); ok {
		t.Error("contexto vazio não deveria conter usuário")
	}
}
