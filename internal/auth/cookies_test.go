package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetSessionCookieAttributes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		cfg         CookieConfig
		wantSecure  bool
		wantSamesit http.SameSite
	}{
		{
			name:        "desenvolvimento com lax",
			cfg:         CookieConfig{Secure: false, SameSite: http.SameSiteLaxMode},
			wantSecure:  false,
			wantSamesit: http.SameSiteLaxMode,
		},
		{
			name:        "produção com lax",
			cfg:         CookieConfig{Secure: true, SameSite: http.SameSiteLaxMode},
			wantSecure:  true,
			wantSamesit: http.SameSiteLaxMode,
		},
		{
			name:        "samesite none força secure",
			cfg:         CookieConfig{Secure: false, SameSite: http.SameSiteNoneMode},
			wantSecure:  true,
			wantSamesit: http.SameSiteNoneMode,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			SetSessionCookie(rec, "token-abc", tt.cfg, 24*time.Hour)

			resp := rec.Result()
			cookies := resp.Cookies()
			if len(cookies) != 1 {
				t.Fatalf("esperava 1 cookie, recebeu %d", len(cookies))
			}

			c := cookies[0]
			if c.Name != SessionCookieName {
				t.Errorf("Name = %q, esperado %q", c.Name, SessionCookieName)
			}
			if c.Value != "token-abc" {
				t.Errorf("Value = %q, esperado %q", c.Value, "token-abc")
			}
			if !c.HttpOnly {
				t.Error("cookie de sessão deve ser HttpOnly")
			}
			if c.Secure != tt.wantSecure {
				t.Errorf("Secure = %v, esperado %v", c.Secure, tt.wantSecure)
			}
			if c.SameSite != tt.wantSamesit {
				t.Errorf("SameSite = %v, esperado %v", c.SameSite, tt.wantSamesit)
			}
			if c.Path != CookiePath {
				t.Errorf("Path = %q, esperado %q", c.Path, CookiePath)
			}
			if c.MaxAge != int((24 * time.Hour).Seconds()) {
				t.Errorf("MaxAge = %d, esperado %d", c.MaxAge, int((24 * time.Hour).Seconds()))
			}
		})
	}
}

func TestClearSessionCookie(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	ClearSessionCookie(rec, CookieConfig{})

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("esperava 1 cookie, recebeu %d", len(cookies))
	}
	if cookies[0].MaxAge != -1 {
		t.Errorf("MaxAge = %d, esperado -1 (remoção)", cookies[0].MaxAge)
	}
	if cookies[0].Value != "" {
		t.Errorf("Value = %q, esperado vazio", cookies[0].Value)
	}
	if !cookies[0].HttpOnly {
		t.Error("cookie de remoção deve manter HttpOnly")
	}
}

func TestSessionTokenFromRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token-sessao"})

	token, ok := SessionTokenFromRequest(req)
	if !ok || token != "token-sessao" {
		t.Errorf("SessionTokenFromRequest() = (%q, %v), esperado (token-sessao, true)", token, ok)
	}

	empty := httptest.NewRequest(http.MethodGet, "/", nil)
	if token, ok := SessionTokenFromRequest(empty); ok || token != "" {
		t.Errorf("requisição sem cookie deveria devolver ok=false")
	}
}

func TestCSRFCookieReadableByJS(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	SetCSRFCookie(rec, "token-csrf", CookieConfig{})

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("esperava 1 cookie, recebeu %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != CSRFCookieName {
		t.Errorf("Name = %q, esperado %q", c.Name, CSRFCookieName)
	}
	if c.HttpOnly {
		t.Error("cookie CSRF deve ser legível pelo JavaScript do frontend")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)

	token, ok := CSRFTokenFromRequest(req)
	if !ok || token != "token-csrf" {
		t.Errorf("CSRFTokenFromRequest() = (%q, %v), esperado (token-csrf, true)", token, ok)
	}
}
