package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// fakeAuthService implementa AuthService em memória para os testes dos
// handlers. Cada método pode ser sobrescrito por teste.
type fakeAuthService struct {
	registerFn func(ctx context.Context, name, email, password, clientIP string) (auth.PublicUser, bool, error)
	verifyFn   func(ctx context.Context, token, clientIP string) error
	resendFn   func(ctx context.Context, email string) error
	loginFn    func(ctx context.Context, email, password, clientIP string) (auth.PublicUser, string, error)
	logoutFn   func(ctx context.Context, sessionToken string) error
	forgotFn   func(ctx context.Context, email, clientIP string) error
	resetFn    func(ctx context.Context, token, newPassword, clientIP string) error
}

func (f *fakeAuthService) Register(ctx context.Context, name, email, password, clientIP string) (auth.PublicUser, bool, error) {
	return f.registerFn(ctx, name, email, password, clientIP)
}

func (f *fakeAuthService) VerifyEmail(ctx context.Context, token, clientIP string) error {
	return f.verifyFn(ctx, token, clientIP)
}

func (f *fakeAuthService) ResendVerification(ctx context.Context, email string) error {
	return f.resendFn(ctx, email)
}

func (f *fakeAuthService) Login(ctx context.Context, email, password, clientIP string) (auth.PublicUser, string, error) {
	return f.loginFn(ctx, email, password, clientIP)
}

func (f *fakeAuthService) Logout(ctx context.Context, sessionToken string) error {
	return f.logoutFn(ctx, sessionToken)
}

func (f *fakeAuthService) ForgotPassword(ctx context.Context, email, clientIP string) error {
	return f.forgotFn(ctx, email, clientIP)
}

func (f *fakeAuthService) ResetPassword(ctx context.Context, token, newPassword, clientIP string) error {
	return f.resetFn(ctx, token, newPassword, clientIP)
}

func newTestAuthHandler(service AuthService) *AuthHandler {
	return NewAuthHandler(service, auth.CookieConfig{}, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func performJSON(t *testing.T, h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal do corpo: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, reader))
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) response.ErrorPayload {
	t.Helper()

	var body struct {
		Error response.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v; corpo: %s", err, rec.Body.String())
	}
	return body.Error
}

func testUser() auth.PublicUser {
	return auth.PublicUser{ID: "id-1", Name: "Ana", Email: "ana@example.com", EmailVerified: false}
}

func TestRegisterSuccess(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		registerFn: func(_ context.Context, name, email, _, _ string) (auth.PublicUser, bool, error) {
			if name != "Ana" || email != "ana@example.com" {
				t.Errorf("Register recebeu (%q, %q)", name, email)
			}
			return testUser(), true, nil
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Register, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"name": "Ana", "email": "ana@example.com", "password": "SenhaForte#123",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado %d; corpo: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var body struct {
		User                  auth.PublicUser `json:"user"`
		VerificationEmailSent bool            `json:"verification_email_sent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.User.ID != "id-1" {
		t.Errorf("user.id = %q", body.User.ID)
	}
	if !body.VerificationEmailSent {
		t.Error("verification_email_sent = false, esperado true")
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Error("resposta não deve conter hash ou campos de senha")
	}
}

func TestRegisterValidationError(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		registerFn: func(context.Context, string, string, string, string) (auth.PublicUser, bool, error) {
			return auth.PublicUser{}, false, auth.NewValidationError("o e-mail informado é inválido")
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Register, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"name": "Ana", "email": "invalido", "password": "SenhaForte#123",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeValidationFail {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeValidationFail)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		registerFn: func(context.Context, string, string, string, string) (auth.PublicUser, bool, error) {
			return auth.PublicUser{}, false, auth.ErrEmailTaken
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Register, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"name": "Ana", "email": "ana@example.com", "password": "SenhaForte#123",
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusConflict)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeEmailAlreadyRegistered {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeEmailAlreadyRegistered)
	}
}

func TestLoginSuccessSetsSessionCookie(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		loginFn: func(_ context.Context, email, _, _ string) (auth.PublicUser, string, error) {
			if email != "ana@example.com" {
				t.Errorf("Login recebeu e-mail %q", email)
			}
			return testUser(), "token-de-sessao", nil
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "ana@example.com", "password": "senha",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}

	cookies := rec.Result().Cookies()
	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case auth.SessionCookieName:
			sessionCookie = c
		case auth.CSRFCookieName:
			csrfCookie = c
		}
	}
	if sessionCookie == nil || sessionCookie.Value != "token-de-sessao" {
		t.Error("cookie de sessão ausente ou com token errado")
	}
	if sessionCookie != nil && !sessionCookie.HttpOnly {
		t.Error("cookie de sessão deve ser HttpOnly")
	}
	if csrfCookie == nil {
		t.Error("login deveria rotacionar o cookie CSRF")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		loginFn: func(context.Context, string, string, string) (auth.PublicUser, string, error) {
			return auth.PublicUser{}, "", auth.ErrInvalidCredentials
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "ana@example.com", "password": "senha-errada",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeInvalidCredentials {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeInvalidCredentials)
	}
}

func TestLoginEmailNotVerified(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		loginFn: func(context.Context, string, string, string) (auth.PublicUser, string, error) {
			return auth.PublicUser{}, "", auth.ErrEmailNotVerified
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "ana@example.com", "password": "senha",
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusForbidden)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeEmailNotVerified {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeEmailNotVerified)
	}
}

func TestVerifyEmailSuccess(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		verifyFn: func(_ context.Context, token, _ string) error {
			if token != "token-valido" {
				t.Errorf("VerifyEmail recebeu token %q", token)
			}
			return nil
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).VerifyEmail, http.MethodPost, "/api/v1/auth/verify-email", map[string]string{
		"token": "token-valido",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
}

func TestVerifyEmailTokenErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"token inválido", auth.ErrInvalidToken, http.StatusBadRequest, response.CodeInvalidToken},
		{"token expirado", auth.ErrTokenExpired, http.StatusBadRequest, response.CodeExpiredToken},
		{"token reutilizado", auth.ErrTokenUsed, http.StatusBadRequest, response.CodeTokenUsed},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := &fakeAuthService{
				verifyFn: func(context.Context, string, string) error { return tt.err },
			}

			rec := performJSON(t, newTestAuthHandler(service).VerifyEmail, http.MethodPost, "/api/v1/auth/verify-email", map[string]string{
				"token": "qualquer-token",
			})

			if rec.Code != tt.status {
				t.Fatalf("status = %d, esperado %d", rec.Code, tt.status)
			}
			if errBody := decodeError(t, rec); errBody.Code != tt.code {
				t.Errorf("código = %q, esperado %q", errBody.Code, tt.code)
			}
		})
	}
}

func TestResendVerificationGenericResponse(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		resendFn: func(context.Context, string) error { return nil },
	}

	rec := performJSON(t, newTestAuthHandler(service).ResendVerification, http.MethodPost, "/api/v1/auth/resend-verification", map[string]string{
		"email": "nao-existe@example.com",
	})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusAccepted)
	}
	if strings.Contains(rec.Body.String(), "não existe") {
		t.Error("resposta não deve revelar existência da conta")
	}
}

func TestForgotPasswordGenericResponse(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		forgotFn: func(context.Context, string, string) error { return nil },
	}

	rec := performJSON(t, newTestAuthHandler(service).ForgotPassword, http.MethodPost, "/api/v1/auth/forgot-password", map[string]string{
		"email": "nao-existe@example.com",
	})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusAccepted)
	}
	if !strings.Contains(rec.Body.String(), "Se existir uma conta") {
		t.Errorf("resposta deveria ser genérica; corpo: %s", rec.Body.String())
	}
}

func TestResetPasswordSuccess(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		resetFn: func(_ context.Context, token, newPassword, _ string) error {
			if token != "token-reset" || newPassword != "nova-senha-forte" {
				t.Errorf("ResetPassword recebeu (%q, %q)", token, newPassword)
			}
			return nil
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).ResetPassword, http.MethodPost, "/api/v1/auth/reset-password", map[string]string{
		"token": "token-reset", "new_password": "nova-senha-forte",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
}

func TestResetPasswordInvalidToken(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		resetFn: func(context.Context, string, string, string) error { return auth.ErrInvalidToken },
	}

	rec := performJSON(t, newTestAuthHandler(service).ResetPassword, http.MethodPost, "/api/v1/auth/reset-password", map[string]string{
		"token": "token-errado", "new_password": "nova-senha-forte",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeInvalidToken {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeInvalidToken)
	}
}

func TestLogoutRevokesAndClearsCookie(t *testing.T) {
	t.Parallel()

	var revoked string
	service := &fakeAuthService{
		logoutFn: func(_ context.Context, token string) error {
			revoked = token
			return nil
		},
	}

	h := newTestAuthHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "token-sessao"})

	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusNoContent)
	}
	if revoked != "token-sessao" {
		t.Errorf("Logout recebeu token %q", revoked)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName && c.MaxAge == -1 {
			found = true
		}
	}
	if !found {
		t.Error("logout deveria remover o cookie de sessão")
	}
}

func TestMeReturnsAuthenticatedUser(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{
		ID: "id-1", Name: "Ana", Email: "ana@example.com",
	}))

	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}

	var body userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.User.ID != "id-1" || body.User.Email != "ana@example.com" {
		t.Errorf("user = %+v", body.User)
	}
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Error("resposta não deve expor campos internos do usuário")
	}
}

func TestMeWithoutUser(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	rec := httptest.NewRecorder()
	h.Me(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCSRFHandlerReturnsToken(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "token-csrf"})

	rec := httptest.NewRecorder()
	h.CSRF(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}

	var body csrfResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON válido: %v", err)
	}
	if body.CSRFToken != "token-csrf" {
		t.Errorf("csrf_token = %q, esperado %q", body.CSRFToken, "token-csrf")
	}
}

func TestCSRFHandlerCreatesCookieWhenMissing(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	rec := httptest.NewRecorder()
	h.CSRF(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == auth.CSRFCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("endpoint deveria criar o cookie CSRF quando ausente")
	}
}

func TestRateLimitedReturns429WithRetryAfter(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		loginFn: func(context.Context, string, string, string) (auth.PublicUser, string, error) {
			return auth.PublicUser{}, "", auth.RateLimitError{RetryAfter: time.Minute}
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "ana@example.com", "password": "senha",
	})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, esperado %q", got, "60")
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeRateLimited {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeRateLimited)
	}
}

func TestInternalErrorIsGeneric(t *testing.T) {
	t.Parallel()

	service := &fakeAuthService{
		loginFn: func(context.Context, string, string, string) (auth.PublicUser, string, error) {
			return auth.PublicUser{}, "", errors.New("segredo interno: senha-do-banco")
		},
	}

	rec := performJSON(t, newTestAuthHandler(service).Login, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "ana@example.com", "password": "senha",
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "segredo interno") {
		t.Error("resposta não deve expor detalhes do erro interno")
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeInternal {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeInternal)
	}
}

func TestMalformedJSONBody(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader("{não é json"))
	rec := httptest.NewRecorder()
	h.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeValidationFail {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeValidationFail)
	}
}

func TestMultipleJSONDocumentsRejected(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"name":"A"}{"name":"B"}`))
	rec := httptest.NewRecorder()
	h.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUnknownJSONFieldsRejected(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	// Campo inesperado (typo de contrato) não pode ser aceito silenciosamente.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"name":"Ana","email":"ana@example.com","password":"senhaForte#123","is_admin":true}`))
	rec := httptest.NewRecorder()
	h.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
	if errBody := decodeError(t, rec); errBody.Code != response.CodeValidationFail {
		t.Errorf("código = %q, esperado %q", errBody.Code, response.CodeValidationFail)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler(&fakeAuthService{})

	// Corpo acima de maxBodyBytes (1 MiB) deve ser rejeitado sem estourar
	// memória do servidor.
	huge := strings.Repeat("a", maxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"`+huge+`","password":"x"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
}
