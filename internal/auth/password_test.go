package auth

import (
	"strings"
	"testing"
)

func TestPasswordHasherHashAndVerify(t *testing.T) {
	t.Parallel()

	h := TestPasswordHasher()

	hash, err := h.Hash("senha-segura-123")
	if err != nil {
		t.Fatalf("Hash() erro inesperado: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("hash = %q, esperado prefixo argon2id PHC", hash)
	}

	ok, err := h.Verify("senha-segura-123", hash)
	if err != nil {
		t.Fatalf("Verify() erro inesperado: %v", err)
	}
	if !ok {
		t.Error("Verify() = false para a senha correta")
	}

	ok, err = h.Verify("senha-errada", hash)
	if err != nil {
		t.Fatalf("Verify() erro inesperado: %v", err)
	}
	if ok {
		t.Error("Verify() = true para senha incorreta")
	}
}

func TestPasswordHasherProducesDistinctSalts(t *testing.T) {
	t.Parallel()

	h := TestPasswordHasher()

	first, err := h.Hash("mesma-senha")
	if err != nil {
		t.Fatalf("Hash() erro inesperado: %v", err)
	}
	second, err := h.Hash("mesma-senha")
	if err != nil {
		t.Fatalf("Hash() erro inesperado: %v", err)
	}

	if first == second {
		t.Error("hashes iguais para a mesma senha: o salt não está sendo aleatório")
	}

	if ok, _ := h.Verify("mesma-senha", first); !ok {
		t.Error("primeiro hash não verifica a senha")
	}
	if ok, _ := h.Verify("mesma-senha", second); !ok {
		t.Error("segundo hash não verifica a senha")
	}
}

func TestPasswordHasherRejectsTamperedHash(t *testing.T) {
	t.Parallel()

	h := TestPasswordHasher()

	hash, err := h.Hash("senha-original")
	if err != nil {
		t.Fatalf("Hash() erro inesperado: %v", err)
	}

	// Troca um caractere no meio do hash codificado (o último caractere de um
	// bloco base64 pode estar dentro dos bits de padding e não alterar o
	// valor decodificado).
	tampered := hash[:len(hash)/2] + "x" + hash[len(hash)/2+1:]

	if ok, err := h.Verify("senha-original", tampered); err != nil || ok {
		t.Errorf("Verify() com hash adulterado = (%v, %v), esperado (false, nil)", ok, err)
	}
}

func TestPasswordHasherRejectsForeignAlgorithm(t *testing.T) {
	t.Parallel()

	h := TestPasswordHasher()

	// Hash bcrypt: sintaticamente PHC de outro algoritmo.
	foreign := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

	if _, err := h.Verify("qualquer-senha", foreign); err == nil {
		t.Error("Verify() com hash bcrypt esperava erro, recebeu nil")
	}
}

func TestPasswordHasherRejectsMalformedHash(t *testing.T) {
	t.Parallel()

	h := TestPasswordHasher()

	for _, malformed := range []string{
		"",
		"senha-em-texto-puro",
		"$argon2id$v=19$m=8192,t=1,p=1",
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdA==",
	} {
		if _, err := h.Verify("senha", malformed); err == nil {
			t.Errorf("Verify(%q) esperava erro, recebeu nil", malformed)
		}
	}
}

func TestVerifyAgainstDummyAlwaysFalse(t *testing.T) {
	t.Parallel()

	h := TestPasswordHasher()

	// Não deve panic nem retornar; apenas consumir tempo de CPU comparável.
	h.VerifyAgainstDummy("qualquer-senha")
}

func TestDefaultPasswordHasherRoundtrip(t *testing.T) {
	t.Parallel()

	// Testa o fluxo com os parâmetros reais de produção uma única vez.
	h := DefaultPasswordHasher()

	hash, err := h.Hash("senha-de-producao-123")
	if err != nil {
		t.Fatalf("Hash() erro inesperado: %v", err)
	}

	if ok, err := h.Verify("senha-de-producao-123", hash); err != nil || !ok {
		t.Errorf("Verify() = (%v, %v), esperado (true, nil)", ok, err)
	}
}
