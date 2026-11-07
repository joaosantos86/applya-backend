package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Limites de tamanho dos campos de cadastro. Eles espelham as constraints das
// migrations (users.name, users.email) e evitam que entradas gigantes cheguem
// ao hash de senha (custoso) ou ao banco.
const (
	MaxNameLength     = 100
	MaxEmailLength    = 254
	MinPasswordLength = 8
	MaxPasswordLength = 256 // limite de saneamento, não de segurança (Argon2id não tem limite de 72 bytes como o bcrypt)
)

// ValidateName valida o nome informado no cadastro.
func ValidateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return NewValidationError("o nome é obrigatório")
	}
	if utf8.RuneCountInString(trimmed) > MaxNameLength {
		return NewValidationError("o nome deve ter no máximo %d caracteres", MaxNameLength)
	}

	return nil
}

// NormalizeEmail normaliza e valida um e-mail.
//
// A normalização remove espaços e converte para minúsculas. Embora a parte
// local (antes do @) seja tecnicamente sensível a maiúsculas, provedores de
// e-mail reais não distinguem; normalizar evita contas duplicadas como
// "Ana@x.com" e "ana@x.com".
func NormalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return "", NewValidationError("o e-mail é obrigatório")
	}
	if len(trimmed) > MaxEmailLength {
		return "", NewValidationError("o e-mail deve ter no máximo %d caracteres", MaxEmailLength)
	}

	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", NewValidationError("o e-mail informado é inválido")
	}

	// ParseAddress aceita "Nome <a@b.com>"; aqui só a forma pura "a@b.com".
	if addr.Address != trimmed {
		return "", NewValidationError("o e-mail informado é inválido")
	}

	normalized := strings.ToLower(addr.Address)
	return normalized, nil
}

// ValidatePassword valida a senha informada no cadastro e na redefinição.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return NewValidationError("a senha deve ter no mínimo %d caracteres", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return NewValidationError("a senha deve ter no máximo %d caracteres", MaxPasswordLength)
	}

	return nil
}

// ValidateRegisterInput aplica todas as validações do cadastro e devolve o
// e-mail normalizado quando tudo estiver correto.
func ValidateRegisterInput(name, email, password string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}

	normalized, err := NormalizeEmail(email)
	if err != nil {
		return "", err
	}

	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	return normalized, nil
}

// ValidateLoginInput aplica as validações do login. A senha só tem limite de
// saneamento (nunca um mínimo): a checagem real é contra o hash.
func ValidateLoginInput(email, password string) (string, error) {
	if password == "" {
		return "", NewValidationError("a senha é obrigatória")
	}
	if len(password) > MaxPasswordLength {
		return "", NewValidationError("a senha deve ter no máximo %d caracteres", MaxPasswordLength)
	}

	return NormalizeEmail(email)
}

// ValidateEmailInput valida apenas o e-mail, para endpoints que recebem
// somente esse campo (reenvio de confirmação, recuperação de senha).
func ValidateEmailInput(email string) (string, error) {
	return NormalizeEmail(email)
}

// ValidateTokenFormat valida tokens vindos de JSON (confirmação e redefinição).
func ValidateTokenFormat(token string) error {
	if !ValidTokenFormat(token) {
		return NewValidationError("o token informado é inválido")
	}
	return nil
}
