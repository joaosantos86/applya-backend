package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// maxBodyBytes limita o tamanho do corpo JSON aceito nas rotas de autenticação.
const maxBodyBytes = 1 << 20 // 1 MiB

// AuthService é a interface que os handlers consomem do módulo de autenticação.
//
// Definida aqui (no pacote que consome), como manda a convenção do projeto:
// auth.Service a implementa em produção e os testes usam um fake.
type AuthService interface {
	Register(ctx context.Context, name, email, password, clientIP string) (auth.PublicUser, bool, error)
	VerifyEmail(ctx context.Context, token, clientIP string) error
	ResendVerification(ctx context.Context, email string) error
	Login(ctx context.Context, email, password, clientIP string) (auth.PublicUser, string, error)
	Logout(ctx context.Context, sessionToken string) error
	ForgotPassword(ctx context.Context, email, clientIP string) error
	ResetPassword(ctx context.Context, token, newPassword, clientIP string) error
}

// AuthHandler expõe os endpoints REST de autenticação.
//
// Cada handler é fino: decodifica o JSON, chama o serviço e escreve a
// resposta. As regras de negócio vivem em internal/auth.
type AuthHandler struct {
	service AuthService
	cookies auth.CookieConfig
	// sessionDuration define o Max-Age do cookie de sessão (igual ao TTL no
	// banco, fornecido pelo SessionManager).
	sessionDuration time.Duration
	logger          *slog.Logger
}

// NewAuthHandler cria o handler de autenticação.
func NewAuthHandler(service AuthService, cookies auth.CookieConfig, sessionDuration time.Duration, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{service: service, cookies: cookies, sessionDuration: sessionDuration, logger: logger}
}

// ---------------------------------------------------------------------------
// Corpos de requisição e resposta
// ---------------------------------------------------------------------------

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type emailRequest struct {
	Email string `json:"email"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type userResponse struct {
	User auth.PublicUser `json:"user"`
}

type registerResponse struct {
	User                  auth.PublicUser `json:"user"`
	VerificationEmailSent bool            `json:"verification_email_sent"`
}

type messageResponse struct {
	Message string `json:"message"`
}

type csrfResponse struct {
	CSRFToken string `json:"csrf_token"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Register trata POST /api/v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadRequest(w, r, err.Error())
		return
	}

	user, emailSent, err := h.service.Register(r.Context(), req.Name, req.Email, req.Password, clientIP(r))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusCreated, registerResponse{User: user, VerificationEmailSent: emailSent})
}

// VerifyEmail trata POST /api/v1/auth/verify-email.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadRequest(w, r, err.Error())
		return
	}

	if err := h.service.VerifyEmail(r.Context(), req.Token, clientIP(r)); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, messageResponse{Message: "E-mail confirmado com sucesso."})
}

// ResendVerification trata POST /api/v1/auth/resend-verification.
//
// A resposta é sempre 202 e genérica, existindo ou não a conta, para não
// permitir enumeração de e-mails.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadRequest(w, r, err.Error())
		return
	}

	if err := h.service.ResendVerification(r.Context(), req.Email); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusAccepted, messageResponse{
		Message: "Se o e-mail estiver cadastrado e pendente de confirmação, enviaremos um novo link.",
	})
}

// Login trata POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadRequest(w, r, err.Error())
		return
	}

	user, token, err := h.service.Login(r.Context(), req.Email, req.Password, clientIP(r))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	auth.SetSessionCookie(w, token, h.cookies, h.sessionDuration)

	// O token CSRF é rotacionado no login: o contexto de segurança mudou.
	h.rotateCSRF(w)

	_ = response.JSON(w, http.StatusOK, userResponse{User: user})
}

// Logout trata POST /api/v1/auth/logout.
//
// Revoga a sessão atual e remove o cookie. É idempotente: funciona mesmo sem
// sessão válida.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token, _ := auth.SessionTokenFromRequest(r)

	auth.ClearSessionCookie(w, h.cookies)

	if err := h.service.Logout(r.Context(), token); err != nil {
		response.InternalError(w, r, h.logger, err, middleware.GetReqID(r.Context()))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Me trata GET /api/v1/auth/me. Requer autenticação (RequireAuth).
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		_ = response.Error(w, http.StatusUnauthorized, response.CodeAuthenticationRequired,
			"autenticação necessária", middleware.GetReqID(r.Context()))
		return
	}

	_ = response.JSON(w, http.StatusOK, userResponse{User: user.Public()})
}

// ForgotPassword trata POST /api/v1/auth/forgot-password.
//
// A resposta é sempre 202 e genérica, existindo ou não a conta, para não
// permitir enumeração de e-mails.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadRequest(w, r, err.Error())
		return
	}

	if err := h.service.ForgotPassword(r.Context(), req.Email, clientIP(r)); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusAccepted, messageResponse{
		Message: "Se existir uma conta associada a este e-mail, enviaremos um link de recuperação.",
	})
}

// ResetPassword trata POST /api/v1/auth/reset-password.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadRequest(w, r, err.Error())
		return
	}

	if err := h.service.ResetPassword(r.Context(), req.Token, req.NewPassword, clientIP(r)); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, messageResponse{Message: "Senha redefinida com sucesso."})
}

// CSRF trata GET /api/v1/auth/csrf: devolve o token CSRF atual (criando o
// cookie quando necessário) para o frontend usar no header X-CSRF-Token.
func (h *AuthHandler) CSRF(w http.ResponseWriter, r *http.Request) {
	token, ok := auth.CSRFTokenFromRequest(r)
	if !ok {
		// O middleware CSRF normalmente já garante o cookie; este fallback
		// cobre o uso do endpoint como primeira chamada da aplicação.
		var err error
		token, _, err = auth.NewToken()
		if err != nil {
			response.InternalError(w, r, h.logger, err, middleware.GetReqID(r.Context()))
			return
		}
		auth.SetCSRFCookie(w, token, h.cookies)
	}

	_ = response.JSON(w, http.StatusOK, csrfResponse{CSRFToken: token})
}

// ---------------------------------------------------------------------------
// Apoio
// ---------------------------------------------------------------------------

// rotateCSRF gera e define um novo token CSRF no cookie.
func (h *AuthHandler) rotateCSRF(w http.ResponseWriter) {
	token, _, err := auth.NewToken()
	if err != nil {
		// Falha em gerar um token aleatório é raríssima; sem cookie novo o
		// token antigo continua válido e a resposta não é interrompida.
		if h.logger != nil {
			h.logger.Error("falha ao rotacionar token CSRF", slog.Any("error", err))
		}
		return
	}
	auth.SetCSRFCookie(w, token, h.cookies)
}

// decodeJSON decodifica o corpo JSON da requisição, com limite de tamanho.
//
// Campos desconhecidos são rejeitados: o contrato dos endpoints de
// autenticação é fechado e um campo inesperado quase sempre indica bug do
// cliente (typo) — aceitá-lo silenciosamente esconderia o erro.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("corpo da requisição inválido ou em formato JSON incorreto")
	}

	// Garante que não há um segundo documento JSON no corpo.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("corpo da requisição inválido ou em formato JSON incorreto")
	}

	return nil
}

// clientIP devolve o IP do cliente para as chaves de rate limiting, sem a
// porta efêmera da conexão (que mudaria a chave a cada requisição e
// anularia o limite).
//
// O middleware RealIP (chi) já reescreveu r.RemoteAddr a partir de
// X-Forwarded-For/X-Real-IP quando atrás de um proxy (Render).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeBadRequest responde 400 com o código validation_failed.
func writeBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	_ = response.Error(w, http.StatusBadRequest, response.CodeValidationFail,
		message, middleware.GetReqID(r.Context()))
}

// writeServiceError traduz erros do serviço para status e códigos estáveis.
func (h *AuthHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := middleware.GetReqID(r.Context())

	var validationErr auth.ValidationError
	if errors.As(err, &validationErr) {
		_ = response.Error(w, http.StatusBadRequest, response.CodeValidationFail, validationErr.Error(), requestID)
		return
	}

	var rateLimitErr auth.RateLimitError
	if errors.As(err, &rateLimitErr) {
		retryAfter := int(rateLimitErr.RetryAfter.Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		_ = response.Error(w, http.StatusTooManyRequests, response.CodeRateLimited,
			"Muitas tentativas. Aguarde alguns minutos e tente novamente.", requestID)
		return
	}

	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		_ = response.Error(w, http.StatusConflict, response.CodeEmailAlreadyRegistered,
			"Este e-mail já está cadastrado.", requestID)
	case errors.Is(err, auth.ErrInvalidCredentials):
		_ = response.Error(w, http.StatusUnauthorized, response.CodeInvalidCredentials,
			"E-mail ou senha inválidos.", requestID)
	case errors.Is(err, auth.ErrEmailNotVerified):
		_ = response.Error(w, http.StatusForbidden, response.CodeEmailNotVerified,
			"Confirme seu e-mail antes de entrar.", requestID)
	case errors.Is(err, auth.ErrInvalidToken):
		_ = response.Error(w, http.StatusBadRequest, response.CodeInvalidToken,
			"O token informado é inválido.", requestID)
	case errors.Is(err, auth.ErrTokenExpired):
		_ = response.Error(w, http.StatusBadRequest, response.CodeExpiredToken,
			"O token expirou. Solicite um novo link.", requestID)
	case errors.Is(err, auth.ErrTokenUsed):
		_ = response.Error(w, http.StatusBadRequest, response.CodeTokenUsed,
			"O token já foi utilizado.", requestID)
	default:
		response.InternalError(w, r, h.logger, err, requestID)
	}
}
