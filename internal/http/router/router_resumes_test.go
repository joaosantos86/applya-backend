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

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/http/handlers"
	"github.com/joaosantos86/applya-backend/internal/http/response"
	"github.com/joaosantos86/applya-backend/internal/resume"
)

// fakeResumeService implementa handlers.ResumeService para os testes de
// roteamento/segurança.
type fakeResumeService struct{}

func (fakeResumeService) EnsureBase(context.Context, string) (resume.Resume, error) {
	return resume.Resume{ID: "base-1", Type: resume.KindBase, Name: resume.DefaultBaseName}, nil
}

func (fakeResumeService) UpdateBase(context.Context, string, resume.Patch) (resume.Resume, error) {
	return resume.Resume{ID: "base-1", Type: resume.KindBase, Revision: 2}, nil
}

func (fakeResumeService) ListVersions(context.Context, string) ([]resume.Summary, error) {
	return []resume.Summary{}, nil
}

func (fakeResumeService) CreateVersion(context.Context, string, string) (resume.Resume, error) {
	return resume.Resume{ID: "version-1", Type: resume.KindVersion}, nil
}

func (fakeResumeService) GetVersion(context.Context, string, string) (resume.Resume, error) {
	return resume.Resume{ID: "version-1", Type: resume.KindVersion}, nil
}

func (fakeResumeService) UpdateVersion(context.Context, string, string, resume.Patch) (resume.Resume, error) {
	return resume.Resume{ID: "version-1", Type: resume.KindVersion, Revision: 2}, nil
}

func (fakeResumeService) DeleteVersion(context.Context, string, string) error { return nil }

func (fakeResumeService) Workspace(context.Context, string) (resume.Resume, []resume.Summary, error) {
	return resume.Resume{ID: "base-1", Type: resume.KindBase}, []resume.Summary{}, nil
}

func newResumeRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cookieCfg := auth.CookieConfig{}
	csrf := auth.NewCSRFMiddleware([]string{testOrigin}, cookieCfg)

	return New(Options{
		Logger:         logger,
		AllowedOrigins: []string{testOrigin},
		Auth: AuthOptions{
			RequireAuth: auth.RequireAuth(fakeFinder{
				user: auth.User{ID: "user-1", Name: "Ana", Email: "ana@example.com"},
			}, cookieCfg, logger),
			CSRF: csrf.Handler,
		},
		Resumes: ResumeOptions{
			Handler: handlers.NewResumeHandler(fakeResumeService{}, logger),
		},
	})
}

func withSession(req *http.Request) *http.Request {
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "session-token"})
	return req
}

func withCSRF(req *http.Request) *http.Request {
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "csrf-token"})
	req.Header.Set("X-CSRF-Token", "csrf-token")
	req.Header.Set("Origin", testOrigin)
	return req
}

func TestResumeRoutesRequireAuthentication(t *testing.T) {
	t.Parallel()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/resumes/base"},
		{http.MethodPatch, "/api/v1/resumes/base"},
		{http.MethodGet, "/api/v1/resumes/versions"},
		{http.MethodPost, "/api/v1/resumes/versions"},
		{http.MethodGet, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301"},
		{http.MethodPatch, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301"},
		{http.MethodDelete, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301"},
		{http.MethodGet, "/api/v1/workspace"},
	}

	for _, route := range routes {
		route := route
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			newResumeRouter().ServeHTTP(rec, httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`)))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, esperado 401", rec.Code)
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
		})
	}
}

func TestResumeMutationsRequireCSRF(t *testing.T) {
	t.Parallel()

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPatch, "/api/v1/resumes/base", `{"name":"x"}`},
		{http.MethodPost, "/api/v1/resumes/versions", `{"name":"x"}`},
		{http.MethodPatch, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301", `{"name":"x"}`},
		{http.MethodDelete, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301", ""},
	}

	for _, route := range routes {
		route := route
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			req := withSession(httptest.NewRequest(route.method, route.path, strings.NewReader(route.body)))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			newResumeRouter().ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, esperado 403", rec.Code)
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
		})
	}
}

func TestResumeMutationsWithValidSessionAndCSRF(t *testing.T) {
	t.Parallel()

	routes := []struct {
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{http.MethodPatch, "/api/v1/resumes/base", `{"name":"x"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/resumes/versions", `{"name":"x"}`, http.StatusCreated},
		{http.MethodPatch, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301", `{"name":"x"}`, http.StatusOK},
		{http.MethodDelete, "/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301", "", http.StatusNoContent},
	}

	for _, route := range routes {
		route := route
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			req := withCSRF(withSession(httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))))

			rec := httptest.NewRecorder()
			newResumeRouter().ServeHTTP(rec, req)

			if rec.Code != route.wantStatus {
				t.Fatalf("status = %d, esperado %d; corpo: %s", rec.Code, route.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestWorkspaceWithValidSession(t *testing.T) {
	t.Parallel()

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/workspace", nil))

	rec := httptest.NewRecorder()
	newResumeRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}

	var body struct {
		User auth.PublicUser `json:"user"`
		Base resume.Resume   `json:"base"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.User.ID != "user-1" {
		t.Errorf("user.id = %q, esperado user-1", body.User.ID)
	}
}

func TestCSRFCookieIsIssuedOnGET(t *testing.T) {
	t.Parallel()

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/resumes/base", nil))

	rec := httptest.NewRecorder()
	newResumeRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}

	found := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == auth.CSRFCookieName && cookie.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("cookie CSRF não foi emitido na resposta")
	}
}

var _ handlers.ResumeService = fakeResumeService{}
