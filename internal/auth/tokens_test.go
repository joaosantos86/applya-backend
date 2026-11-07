package auth

import (
	"encoding/hex"
	"testing"
)

func TestNewTokenFormat(t *testing.T) {
	t.Parallel()

	token, hash, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken() erro inesperado: %v", err)
	}

	if !ValidTokenFormat(token) {
		t.Errorf("token gerado %q não passa em ValidTokenFormat", token)
	}

	if len(hash) != 64 {
		t.Errorf("hash tem %d caracteres, esperado 64 (sha256 hex)", len(hash))
	}
	if _, err := hex.DecodeString(hash); err != nil {
		t.Errorf("hash não é hexadecimal válido: %v", err)
	}
}

func TestNewTokenUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})
	for range 500 {
		token, hash, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken() erro inesperado: %v", err)
		}
		if _, ok := seen[token]; ok {
			t.Fatal("tokens gerados não são únicos")
		}
		if _, ok := seen[hash]; ok {
			t.Fatal("hashes gerados não são únicos")
		}
		seen[token] = struct{}{}
		seen[hash] = struct{}{}
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	t.Parallel()

	const token = "abc123"

	first := HashToken(token)
	second := HashToken(token)
	if first != second {
		t.Error("HashToken() não é determinístico para a mesma entrada")
	}
	if first == HashToken("outra-entrada") {
		t.Error("entradas diferentes geraram o mesmo hash")
	}
}

func TestValidTokenFormatRejects(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		"não-base64!@#",
		"aGVsbG8",                 // decodifica para 5 bytes
		"%%%",                     // base64url inválido
		string(make([]byte, 200)), // grande demais
	}

	for _, token := range invalid {
		if ValidTokenFormat(token) {
			t.Errorf("ValidTokenFormat(%q) = true, esperado false", token)
		}
	}
}
