// Package response centraliza a escrita de respostas JSON da API.
//
// O formato de erro é sempre o mesmo:
//
//	{"error": {"code": "not_found", "message": "recurso não encontrado"}}
//
// Códigos de erro são estáveis e podem ser usados pelo frontend para tratar
// casos específicos sem depender do texto da mensagem.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ContentTypeJSON é o Content-Type utilizado em todas as respostas da API.
const ContentTypeJSON = "application/json; charset=utf-8"

// Códigos de erro estáveis usados pelas respostas de erro.
const (
	CodeInternal         = "internal_error"
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"

	// Autenticação
	CodeAuthenticationRequired = "authentication_required"
	CodeInvalidCredentials     = "invalid_credentials"
	CodeEmailNotVerified       = "email_not_verified"
	CodeEmailAlreadyRegistered = "email_already_registered"

	// Tokens de uso único
	CodeInvalidToken = "invalid_token"
	CodeExpiredToken = "expired_token"
	CodeTokenUsed    = "token_used"

	// Segurança
	CodeCSRFFailed     = "csrf_failed"
	CodeRateLimited    = "rate_limited"
	CodeValidationFail = "validation_failed"

	// Currículos
	CodeConflict        = "conflict"
	CodePayloadTooLarge = "payload_too_large"
)

// ErrorPayload descreve um erro no corpo da resposta.
type ErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type errorBody struct {
	Error ErrorPayload `json:"error"`
}

// JSON escreve payload com o status informado no formato JSON.
//
// O Content-Type é definido antes do WriteHeader para que o cliente receba o
// cabeçalho correto mesmo em respostas de erro.
func JSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if payload == nil {
		return nil
	}

	return json.NewEncoder(w).Encode(payload)
}

// Error escreve uma resposta de erro padronizada.
func Error(w http.ResponseWriter, status int, code, message, requestID string) error {
	return JSON(w, status, errorBody{
		Error: ErrorPayload{
			Code:      code,
			Message:   message,
			RequestID: requestID,
		},
	})
}

// InternalError registra o erro real no log e devolve ao cliente uma mensagem
// genérica, sem stack trace, detalhes internos ou credenciais.
func InternalError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error, requestID string) {
	if logger != nil {
		logger.ErrorContext(r.Context(), "erro interno ao processar requisição",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("request_id", requestID),
			slog.Any("error", err),
		)
	}

	_ = Error(w, http.StatusInternalServerError, CodeInternal, "erro interno do servidor", requestID)
}
