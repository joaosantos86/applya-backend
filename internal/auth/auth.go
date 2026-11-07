// Package auth implementa a autenticação do Applya: cadastro, confirmação de
// e-mail, login, sessões com cookies, recuperação e redefinição de senha.
//
// O módulo é o único dono das tabelas users, sessions, auth_tokens e
// rate_limits. Handlers HTTP ficam em internal/http/handlers; este pacote
// contém as regras de negócio, a persistência (pgx) e os middlewares de
// autenticação e CSRF.
package auth

import (
	"errors"
	"fmt"
	"time"
)

// Finalidades dos tokens de uso único armazenados em auth_tokens.
const (
	PurposeEmailVerification = "email_verification"
	PurposePasswordReset     = "password_reset"
)

// Erros de negócio devolvidos pelo serviço. Os handlers os traduzem para
// status HTTP e códigos de erro estáveis.
var (
	// ErrInvalidCredentials indica e-mail ou senha incorretos. O handler usa
	// sempre a mesma mensagem para não revelar qual dos dois está errado.
	ErrInvalidCredentials = errors.New("e-mail ou senha inválidos")

	// ErrEmailNotVerified indica que a senha confere, mas o e-mail ainda não
	// foi confirmado; o login só é permitido após a confirmação.
	ErrEmailNotVerified = errors.New("e-mail ainda não confirmado")

	// ErrEmailTaken indica que o e-mail já está cadastrado.
	ErrEmailTaken = errors.New("e-mail já cadastrado")

	// ErrInvalidToken indica token inexistente ou malformado.
	ErrInvalidToken = errors.New("token inválido")

	// ErrTokenExpired indica token válido, porém expirado.
	ErrTokenExpired = errors.New("token expirado")

	// ErrTokenUsed indica token já consumido anteriormente.
	ErrTokenUsed = errors.New("token já utilizado")

	// ErrSessionNotFound indica sessão inexistente, expirada ou revogada.
	ErrSessionNotFound = errors.New("sessão inválida")

	// ErrUserNotFound é interno: indica que o usuário não existe no banco.
	// Nunca é exposto diretamente ao cliente; o serviço o traduz para
	// respostas genéricas (anti-enumeração).
	ErrUserNotFound = errors.New("usuário não encontrado")
)

// ValidationError é um erro de validação de entrada com mensagem própria para
// o usuário final. O handler responde 400 com o código validation_failed.
type ValidationError struct {
	msg string
}

// NewValidationError cria um ValidationError com a mensagem informada.
func NewValidationError(format string, args ...any) ValidationError {
	return ValidationError{msg: fmt.Sprintf(format, args...)}
}

// Error implementa a interface error.
func (e ValidationError) Error() string { return e.msg }

// RateLimitError indica que um limite de requisições foi atingido. RetryAfter
// orienta o cliente (e o header Retry-After) sobre quando tentar de novo.
type RateLimitError struct {
	RetryAfter time.Duration
}

// Error implementa a interface error.
func (e RateLimitError) Error() string {
	return "muitas tentativas; aguarde antes de tentar novamente"
}

// User é o usuário como persistido no banco. PasswordHash nunca é exposto em
// respostas HTTP; para isso existe PublicUser.
type User struct {
	ID              string
	Name            string
	Email           string
	PasswordHash    string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// EmailVerified informa se o e-mail do usuário já foi confirmado.
func (u User) EmailVerified() bool { return u.EmailVerifiedAt != nil }

// PublicUser é a projeção pública do usuário, retornada nas respostas da API.
// Nenhum campo sensível (hash de senha, timestamps internos) é exposto.
type PublicUser struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// Public devolve a projeção pública do usuário.
func (u User) Public() PublicUser {
	return PublicUser{
		ID:            u.ID,
		Name:          u.Name,
		Email:         u.Email,
		EmailVerified: u.EmailVerified(),
	}
}

// PurposeToken é um token de uso único do banco.
type PurposeToken struct {
	ID         string
	UserID     string
	Purpose    string
	TokenHash  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

// Expired informa se o token já expirou em relação a now.
func (t PurposeToken) Expired(now time.Time) bool { return !now.Before(t.ExpiresAt) }

// Consumed informa se o token já foi consumido.
func (t PurposeToken) Consumed() bool { return t.ConsumedAt != nil }
