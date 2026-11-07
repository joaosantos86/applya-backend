package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/http/response"
	"github.com/joaosantos86/applya-backend/internal/resume"
)

// fakeResumeService implementa ResumeService em memória. Cada método pode ser
// sobrescrito por teste.
type fakeResumeService struct {
	ensureBaseFn    func(ctx context.Context, userID string) (resume.Resume, error)
	updateBaseFn    func(ctx context.Context, userID string, patch resume.Patch) (resume.Resume, error)
	listVersionsFn  func(ctx context.Context, userID string) ([]resume.Summary, error)
	createVersionFn func(ctx context.Context, userID, name string) (resume.Resume, error)
	getVersionFn    func(ctx context.Context, userID, versionID string) (resume.Resume, error)
	updateVersionFn func(ctx context.Context, userID, versionID string, patch resume.Patch) (resume.Resume, error)
	deleteVersionFn func(ctx context.Context, userID, versionID string) error
	workspaceFn     func(ctx context.Context, userID string) (resume.Resume, []resume.Summary, error)
}

func (f *fakeResumeService) EnsureBase(ctx context.Context, userID string) (resume.Resume, error) {
	if f.ensureBaseFn == nil {
		return testResume(), nil
	}
	return f.ensureBaseFn(ctx, userID)
}

func (f *fakeResumeService) UpdateBase(ctx context.Context, userID string, patch resume.Patch) (resume.Resume, error) {
	if f.updateBaseFn == nil {
		return testResume(), nil
	}
	return f.updateBaseFn(ctx, userID, patch)
}

func (f *fakeResumeService) ListVersions(ctx context.Context, userID string) ([]resume.Summary, error) {
	if f.listVersionsFn == nil {
		return []resume.Summary{}, nil
	}
	return f.listVersionsFn(ctx, userID)
}

func (f *fakeResumeService) CreateVersion(ctx context.Context, userID, name string) (resume.Resume, error) {
	if f.createVersionFn == nil {
		return testResume(), nil
	}
	return f.createVersionFn(ctx, userID, name)
}

func (f *fakeResumeService) GetVersion(ctx context.Context, userID, versionID string) (resume.Resume, error) {
	if f.getVersionFn == nil {
		return testResume(), nil
	}
	return f.getVersionFn(ctx, userID, versionID)
}

func (f *fakeResumeService) UpdateVersion(ctx context.Context, userID, versionID string, patch resume.Patch) (resume.Resume, error) {
	if f.updateVersionFn == nil {
		return testResume(), nil
	}
	return f.updateVersionFn(ctx, userID, versionID, patch)
}

func (f *fakeResumeService) DeleteVersion(ctx context.Context, userID, versionID string) error {
	if f.deleteVersionFn == nil {
		return nil
	}
	return f.deleteVersionFn(ctx, userID, versionID)
}

func (f *fakeResumeService) Workspace(ctx context.Context, userID string) (resume.Resume, []resume.Summary, error) {
	if f.workspaceFn == nil {
		return testResume(), []resume.Summary{}, nil
	}
	return f.workspaceFn(ctx, userID)
}

func newTestResumeHandler(service ResumeService) *ResumeHandler {
	return NewResumeHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func testResume() resume.Resume {
	return resume.Resume{
		ID:        "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		Type:      resume.KindBase,
		Name:      resume.DefaultBaseName,
		Revision:  1,
		Data:      resume.NewData(),
		CreatedAt: time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC),
	}
}

func testAuthUser() auth.User {
	return auth.User{ID: "user-1", Name: "Ana", Email: "ana@example.com"}
}

func performResume(t *testing.T, h http.HandlerFunc, method, path string, body any, user *auth.User) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal do corpo: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	if user != nil {
		req = req.WithContext(auth.WithUser(req.Context(), *user))
	}
	req = withRouteID(req)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func performResumeRaw(t *testing.T, h http.HandlerFunc, method, path, raw string, user *auth.User) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(raw))
	if user != nil {
		req = req.WithContext(auth.WithUser(req.Context(), *user))
	}
	req = withRouteID(req)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// withRouteID preenche o parâmetro de rota "id" a partir do path, já que os
// handlers são chamados diretamente nos testes (sem o roteador do chi).
func withRouteID(r *http.Request) *http.Request {
	const marker = "/versions/"
	idx := strings.LastIndex(r.URL.Path, marker)
	if idx < 0 {
		return r
	}
	id := r.URL.Path[idx+len(marker):]
	if id == "" || strings.Contains(id, "/") {
		return r
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestGetBaseRequiresUser(t *testing.T) {
	t.Parallel()

	rec := performResume(t, newTestResumeHandler(&fakeResumeService{}).GetBase, http.MethodGet, "/api/v1/resumes/base", nil, nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if code := decodeError(t, rec).Code; code != response.CodeAuthenticationRequired {
		t.Errorf("código = %q, esperado %q", code, response.CodeAuthenticationRequired)
	}
}

func TestGetBaseReturnsCreatedBase(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	var receivedUserID string
	service := &fakeResumeService{
		ensureBaseFn: func(_ context.Context, userID string) (resume.Resume, error) {
			receivedUserID = userID
			return testResume(), nil
		},
	}

	rec := performResume(t, newTestResumeHandler(service).GetBase, http.MethodGet, "/api/v1/resumes/base", nil, &user)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200; corpo: %s", rec.Code, rec.Body.String())
	}
	if receivedUserID != user.ID {
		t.Errorf("userID = %q, esperado %q", receivedUserID, user.ID)
	}

	var body struct {
		Resume resume.Resume `json:"resume"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.Resume.ID != testResume().ID {
		t.Errorf("id = %q, esperado %q", body.Resume.ID, testResume().ID)
	}
}

func TestUpdateBaseValidationError(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	service := &fakeResumeService{
		updateBaseFn: func(context.Context, string, resume.Patch) (resume.Resume, error) {
			return resume.Resume{}, resume.NewValidationError("nome inválido")
		},
	}

	rec := performResume(t, newTestResumeHandler(service).UpdateBase, http.MethodPatch, "/api/v1/resumes/base",
		map[string]any{"name": ""}, &user)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
	if code := decodeError(t, rec).Code; code != response.CodeValidationFail {
		t.Errorf("código = %q, esperado %q", code, response.CodeValidationFail)
	}
}

func TestUpdateBaseConflict(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	service := &fakeResumeService{
		updateBaseFn: func(context.Context, string, resume.Patch) (resume.Resume, error) {
			return resume.Resume{}, resume.ErrConflict
		},
	}

	rec := performResume(t, newTestResumeHandler(service).UpdateBase, http.MethodPatch, "/api/v1/resumes/base",
		map[string]any{"revision": 1, "name": "Novo"}, &user)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, esperado 409", rec.Code)
	}
	if code := decodeError(t, rec).Code; code != response.CodeConflict {
		t.Errorf("código = %q, esperado %q", code, response.CodeConflict)
	}
}

func TestGetVersionNotFound(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	service := &fakeResumeService{
		getVersionFn: func(context.Context, string, string) (resume.Resume, error) {
			return resume.Resume{}, resume.ErrNotFound
		},
	}

	rec := performResume(t, newTestResumeHandler(service).GetVersion, http.MethodGet,
		"/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301", nil, &user)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}
	if code := decodeError(t, rec).Code; code != response.CodeNotFound {
		t.Errorf("código = %q, esperado %q", code, response.CodeNotFound)
	}
}

func TestGetVersionUsesAuthenticatedUser(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	versionID := "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	var gotUserID, gotVersionID string

	service := &fakeResumeService{
		getVersionFn: func(_ context.Context, userID, id string) (resume.Resume, error) {
			gotUserID, gotVersionID = userID, id
			return testResume(), nil
		},
	}

	rec := performResume(t, newTestResumeHandler(service).GetVersion, http.MethodGet,
		"/api/v1/resumes/versions/"+versionID, nil, &user)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	if gotUserID != user.ID || gotVersionID != versionID {
		t.Errorf("recebido userID=%q versionID=%q", gotUserID, gotVersionID)
	}
}

func TestDeleteVersionReturnsNoContent(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	called := false
	service := &fakeResumeService{
		deleteVersionFn: func(context.Context, string, string) error {
			called = true
			return nil
		},
	}

	rec := performResume(t, newTestResumeHandler(service).DeleteVersion, http.MethodDelete,
		"/api/v1/resumes/versions/3f2504e0-4f89-11d3-9a0c-0305e82c3301", nil, &user)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("corpo = %q, esperado vazio", rec.Body.String())
	}
	if !called {
		t.Error("serviço não foi chamado")
	}
}

func TestCreateVersionReturnsCreated(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	service := &fakeResumeService{
		createVersionFn: func(_ context.Context, _, name string) (resume.Resume, error) {
			version := testResume()
			version.Type = resume.KindVersion
			version.Name = name
			return version, nil
		},
	}

	rec := performResume(t, newTestResumeHandler(service).CreateVersion, http.MethodPost,
		"/api/v1/resumes/versions", map[string]any{"name": "Backend Go"}, &user)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201; corpo: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	rec := performResumeRaw(t, newTestResumeHandler(&fakeResumeService{}).UpdateBase, http.MethodPatch,
		"/api/v1/resumes/base", `{"name":`, &user)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
	if code := decodeError(t, rec).Code; code != response.CodeValidationFail {
		t.Errorf("código = %q, esperado %q", code, response.CodeValidationFail)
	}
}

func TestUpdateRejectsUnknownField(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	rec := performResumeRaw(t, newTestResumeHandler(&fakeResumeService{}).UpdateBase, http.MethodPatch,
		"/api/v1/resumes/base", `{"owner_id":"outro-usuario"}`, &user)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
}

func TestUpdateRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	body := `{"name":"` + strings.Repeat("a", resume.MaxResumeBodyBytes) + `"}`

	rec := performResumeRaw(t, newTestResumeHandler(&fakeResumeService{}).UpdateBase, http.MethodPatch,
		"/api/v1/resumes/base", body, &user)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperado 413", rec.Code)
	}
	if code := decodeError(t, rec).Code; code != response.CodePayloadTooLarge {
		t.Errorf("código = %q, esperado %q", code, response.CodePayloadTooLarge)
	}
}

func TestWorkspaceReturnsUserBaseAndVersions(t *testing.T) {
	t.Parallel()

	user := testAuthUser()
	version := testResume()
	version.Type = resume.KindVersion
	version.ID = "4f2504e0-4f89-11d3-9a0c-0305e82c3302"

	service := &fakeResumeService{
		workspaceFn: func(context.Context, string) (resume.Resume, []resume.Summary, error) {
			return testResume(), []resume.Summary{version.Summary()}, nil
		},
	}

	rec := performResume(t, newTestResumeHandler(service).Workspace, http.MethodGet, "/api/v1/workspace", nil, &user)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200; corpo: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		User     auth.PublicUser  `json:"user"`
		Base     resume.Resume    `json:"base"`
		Versions []resume.Summary `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.User.ID != user.ID {
		t.Errorf("user.id = %q, esperado %q", body.User.ID, user.ID)
	}
	if body.Base.ID == "" {
		t.Error("base não deveria ser vazia")
	}
	if len(body.Versions) != 1 {
		t.Errorf("versions = %d, esperado 1", len(body.Versions))
	}
}
