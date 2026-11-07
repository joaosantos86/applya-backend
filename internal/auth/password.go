package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// PasswordHasher cria e verifica hashes de senha com Argon2id.
//
// O hash é armazenado no formato PHC (https://github.com/P-H-C/phc-string-format):
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt b64>$<hash b64>
//
// Como os parâmetros viajam junto do hash, é possível endurecê-los no futuro
// sem invalidar senhas existentes: a verificação lê os parâmetros do próprio
// hash e senhas antigas continuam funcionando até a próxima troca.
type PasswordHasher struct {
	memory      uint32 // memória em KiB
	iterations  uint32
	parallelism uint8
	saltLen     uint32
	keyLen      uint32
}

// Parâmetros Argon2id de produção, alinhados com as recomendações da OWASP
// (RFC 9106, perfil de baixa memória): 64 MiB de memória, 3 iterações e 2
// graus de paralelismo. Em hardware modesto cada hash leva centenas de ms —
// aceitável para login e troca de senha de um projeto pessoal.
const (
	defaultMemory      = 64 * 1024 // KiB
	defaultIterations  = 3
	defaultParallelism = 2
	defaultSaltLen     = 16
	defaultKeyLen      = 32
)

// DefaultPasswordHasher devolve o hasher com os parâmetros de produção.
func DefaultPasswordHasher() PasswordHasher {
	return PasswordHasher{
		memory:      defaultMemory,
		iterations:  defaultIterations,
		parallelism: defaultParallelism,
		saltLen:     defaultSaltLen,
		keyLen:      defaultKeyLen,
	}
}

// TestPasswordHasher devolve um hasher com parâmetros reduzidos para testes
// unitários, mantendo o mesmo formato e fluxo de código. NUNCA use em
// produção: a segurança fica incomparavelmente menor.
func TestPasswordHasher() PasswordHasher {
	return PasswordHasher{
		memory:      8 * 1024,
		iterations:  1,
		parallelism: 1,
		saltLen:     16,
		keyLen:      32,
	}
}

// Hash cria o hash Argon2id da senha no formato PHC.
func (h PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("gerar salt do hash: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, h.iterations, h.memory, h.parallelism, h.keyLen)

	return encodePHC(h.memory, h.iterations, h.parallelism, salt, key), nil
}

// Verify compara a senha com um hash no formato PHC. Nunca devolve o erro
// "senha incorreta": devolve (false, nil), reservando o erro para hashes
// malformados, que indicam dado corrompido.
func (h PasswordHasher) Verify(password, encoded string) (bool, error) {
	params, salt, key, err := decodePHC(encoded)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(key)))

	// Comparação em tempo constante para não vazar posição de diferença.
	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

// VerifyAgainstDummy executa uma verificação Argon2id com custo real contra um
// hash falso e devolve sempre false. É usada quando o e-mail não existe, para
// que o tempo de resposta do login não revele se a conta existe ou não.
func (h PasswordHasher) VerifyAgainstDummy(password string) {
	// Hash fixo gerado com os parâmetros padrão; a senha nunca bate.
	_, _ = h.Verify(password, dummyPHC(h))
}

// dummyPHC monta um hash PHC sintaticamente válido a partir dos parâmetros do
// hasher. O conteúdo não corresponde a nenhuma senha real.
func dummyPHC(h PasswordHasher) string {
	salt := make([]byte, h.saltLen)
	key := make([]byte, h.keyLen)

	return encodePHC(h.memory, h.iterations, h.parallelism, salt, key)
}

// ---------------------------------------------------------------------------
// Codificação PHC
// ---------------------------------------------------------------------------

type phcParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

// encodePHC monta a string $argon2id$v=19$m=...,t=...,p=...$salt$hash.
func encodePHC(memory, iterations uint32, parallelism uint8, salt, key []byte) string {
	b64 := base64.RawStdEncoding.EncodeToString

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, iterations, parallelism, b64(salt), b64(key))
}

// decodePHC interpreta uma string PHC do Argon2id.
func decodePHC(encoded string) (phcParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return phcParams{}, nil, nil, fmt.Errorf("hash de senha em formato inesperado")
	}
	if parts[1] != "argon2id" || parts[2] != "v=19" {
		return phcParams{}, nil, nil, fmt.Errorf("hash de senha não é argon2id")
	}

	params, err := parsePHCParams(parts[3])
	if err != nil {
		return phcParams{}, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("salt do hash de senha inválido: %w", err)
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("hash de senha inválido: %w", err)
	}

	if params.memory < 8*1024 || params.iterations < 1 || params.parallelism < 1 {
		return phcParams{}, nil, nil, fmt.Errorf("parâmetros do hash de senha fora dos limites")
	}

	return params, salt, key, nil
}

// parsePHCParams interpreta "m=65536,t=3,p=2".
func parsePHCParams(raw string) (phcParams, error) {
	var params phcParams

	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return phcParams{}, fmt.Errorf("parâmetro %q do hash de senha inválido", part)
		}

		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed < 0 {
			return phcParams{}, fmt.Errorf("parâmetro %q do hash de senha inválido", part)
		}

		switch key {
		case "m":
			params.memory = uint32(parsed)
		case "t":
			params.iterations = uint32(parsed)
		case "p":
			params.parallelism = uint8(parsed)
		}
	}

	return params, nil
}
