package auth

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	valid := []string{"Ana", "João da Silva", strings.Repeat("a", MaxNameLength)}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) erro inesperado: %v", name, err)
		}
	}

	invalid := []string{"", "   ", strings.Repeat("a", MaxNameLength+1)}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) esperava erro, recebeu nil", name)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"ana@example.com", "ana@example.com"},
		{"  Ana@Example.COM ", "ana@example.com"},
		{"user+tag@sub.domain.com.br", "user+tag@sub.domain.com.br"},
	}

	for _, tt := range tests {
		got, err := NormalizeEmail(tt.input)
		if err != nil {
			t.Errorf("NormalizeEmail(%q) erro inesperado: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeEmail(%q) = %q, esperado %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeEmailInvalid(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		"sem-arroba",
		"a@",
		"@example.com",
		"espaço no@meio.com",
		"Nome <ana@example.com>",
		strings.Repeat("a", MaxEmailLength) + "@example.com",
	}

	for _, email := range invalid {
		if _, err := NormalizeEmail(email); err == nil {
			t.Errorf("NormalizeEmail(%q) esperava erro, recebeu nil", email)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	if err := ValidatePassword("senha-com-mais-de-8"); err != nil {
		t.Errorf("senha válida rejeitada: %v", err)
	}

	invalid := []string{
		"",
		"curta",
		strings.Repeat("a", MaxPasswordLength+1),
	}
	for _, password := range invalid {
		if err := ValidatePassword(password); err == nil {
			t.Errorf("ValidatePassword(%q) esperava erro, recebeu nil", password[:min(len(password), 8)])
		}
	}
}

func TestValidateRegisterInput(t *testing.T) {
	t.Parallel()

	if _, err := ValidateRegisterInput("Ana", "Ana@Example.com", "senha-valida"); err != nil {
		t.Errorf("cadastro válido rejeitado: %v", err)
	}

	cases := []struct {
		name     string
		email    string
		password string
	}{
		{"", "ana@example.com", "senha-valida"},
		{"Ana", "invalido", "senha-valida"},
		{"Ana", "ana@example.com", "curta"},
	}
	for _, c := range cases {
		if _, err := ValidateRegisterInput(c.name, c.email, c.password); err == nil {
			t.Errorf("ValidateRegisterInput(%q, %q, ...) esperava erro", c.name, c.email)
		}
	}
}

func TestValidateLoginInput(t *testing.T) {
	t.Parallel()

	if _, err := ValidateLoginInput("Ana@Example.com", "qualquer"); err != nil {
		t.Errorf("login válido rejeitado: %v", err)
	}

	// Senha vazia é erro de validação; senha curta não (o mínimo só vale no
	// cadastro/redefinição).
	if _, err := ValidateLoginInput("ana@example.com", ""); err == nil {
		t.Error("senha vazia deveria ser rejeitada")
	}
	if _, err := ValidateLoginInput("ana@example.com", "abc"); err != nil {
		t.Errorf("senha curta no login não deveria ser rejeitada: %v", err)
	}
	if _, err := ValidateLoginInput("invalido", "qualquer"); err == nil {
		t.Error("e-mail inválido deveria ser rejeitado")
	}
}

func TestValidateTokenFormat(t *testing.T) {
	t.Parallel()

	token, _, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken() erro inesperado: %v", err)
	}

	if err := ValidateTokenFormat(token); err != nil {
		t.Errorf("token válido rejeitado: %v", err)
	}
	if err := ValidateTokenFormat("malformado!!"); err == nil {
		t.Error("token malformado deveria ser rejeitado")
	}
}

// min evita indexar strings curtas nos testes de senha.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
