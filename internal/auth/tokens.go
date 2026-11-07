package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenBytes é o tamanho dos tokens gerados (sessão, confirmação de e-mail e
// recuperação de senha). 32 bytes = 256 bits de entropia, considerados
// criptograficamente suficientes para tokens de uso único e sessões.
const tokenBytes = 32

// NewToken gera um token criptograficamente aleatório (crypto/rand) e o hash
// SHA-256 correspondente, que é o único valor persistido.
//
// O token cru (base64url, 43 caracteres) é entregue apenas ao usuário final;
// a base armazena somente o hash. Com isso, um vazamento do banco não expõe
// tokens utilizáveis.
func NewToken() (token, hash string, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("gerar token aleatório: %w", err)
	}

	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken devolve o hash SHA-256 (hex) de um token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidTokenFormat informa se o token tem o formato esperado: base64url que
// decodifica para exatamente tokenBytes. Impede tokens absurdamente longos de
// chegarem ao banco.
func ValidTokenFormat(token string) bool {
	if token == "" || len(token) > 128 {
		return false
	}

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}

	return len(decoded) == tokenBytes
}
