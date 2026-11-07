//go:build integration

// Testes de integração do módulo de autenticação.
//
// Execução:
//
//	go test -tags=integration ./internal/auth/...
//
// Requer DATABASE_URL apontando para um PostgreSQL descartável: os testes
// criam o esquema (espelhando db/migrations) e apagam os dados entre casos.
// NUNCA rode contra um banco com dados reais.
package auth

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/joaosantos86/applya-backend/internal/database"
	"github.com/joaosantos86/applya-backend/internal/email"
)

var testPool *pgxpool.Pool

// schemaCriado garante que o esquema é criado uma única vez.
var schemaCriado sync.Once

func TestMain(m *testing.M) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Println("DATABASE_URL não definida; testes de integração ignorados")
		os.Exit(0)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	pool, err := database.NewPool(context.Background(), databaseURL, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conectar ao postgres: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	schemaCriado.Do(func() {
		if err := createSchema(context.Background(), pool); err != nil {
			fmt.Fprintf(os.Stderr, "criar esquema: %v\n", err)
			pool.Close()
			os.Exit(1)
		}
	})

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// createSchema espelha db/migrations/20261106000000_create_auth_tables.sql.
func createSchema(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
			email             TEXT NOT NULL UNIQUE CHECK (char_length(email) <= 254),
			password_hash     TEXT NOT NULL,
			email_verified_at TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			expires_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS auth_tokens (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			purpose     TEXT NOT NULL CHECK (purpose IN ('email_verification', 'password_reset')),
			token_hash  TEXT NOT NULL UNIQUE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			expires_at  TIMESTAMPTZ NOT NULL,
			consumed_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			key          TEXT NOT NULL,
			window_start TIMESTAMPTZ NOT NULL,
			count        INTEGER NOT NULL DEFAULT 1 CHECK (count >= 0),
			PRIMARY KEY (key, window_start)
		)`,
	}

	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("executar DDL: %w", err)
		}
	}

	return nil
}

// truncateAll limpa as tabelas entre os testes.
func truncateAll(t *testing.T) {
	t.Helper()

	if _, err := testPool.Exec(context.Background(),
		`TRUNCATE rate_limits, auth_tokens, sessions, users`); err != nil {
		t.Fatalf("truncar tabelas: %v", err)
	}
}

// captureSender guarda os e-mails enviados para extrair tokens dos links.
type captureSender struct {
	msgs []email.Message
}

func (c *captureSender) Send(_ context.Context, msg email.Message) error {
	c.msgs = append(c.msgs, msg)
	return nil
}

// tokenFromLast extrai o token do link do último e-mail capturado.
func tokenFromLast(t *testing.T, c *captureSender) string {
	t.Helper()

	if len(c.msgs) == 0 {
		t.Fatal("nenhum e-mail capturado")
	}

	body := c.msgs[len(c.msgs)-1].TextBody
	marker := "token="
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatalf("e-mail sem link de token: %s", body)
	}

	token := body[idx+len(marker):]
	// O token termina no primeiro espaço/quebra de linha do corpo.
	if end := strings.IndexAny(token, " \t\r\n"); end >= 0 {
		token = token[:end]
	}
	if !ValidTokenFormat(token) {
		t.Fatalf("token extraído inválido: %q", token)
	}

	return token
}

func newTestService(sender email.Sender) *Service {
	store := NewStore(testPool)
	sessions := NewSessionManager(store, time.Hour, time.Now)

	return NewService(ServiceOptions{
		Store:    store,
		Sessions: sessions,
		Mailer:   email.NewMailer(sender, "Applya <no-reply@applya.dev>", "http://localhost:5173"),
		Hasher:   TestPasswordHasher(),
		Logger:   discardLogger(),
		Now:      time.Now,
	})
}

func registerAndVerify(t *testing.T, service *Service, capture *captureSender, name, emailAddress, password string) PublicUser {
	t.Helper()

	user, _, err := service.Register(context.Background(), name, emailAddress, password, "127.0.0.1")
	if err != nil {
		t.Fatalf("Register() erro inesperado: %v", err)
	}

	if err := service.VerifyEmail(context.Background(), tokenFromLast(t, capture), "127.0.0.1"); err != nil {
		t.Fatalf("VerifyEmail() erro inesperado: %v", err)
	}

	return user
}

// ---------------------------------------------------------------------------

func TestRegisterAndLoginFlow(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	user, sent, err := service.Register(ctx, "Ana", "Ana@Example.com", "SenhaForte#123", "127.0.0.1")
	if err != nil {
		t.Fatalf("Register() erro inesperado: %v", err)
	}
	if !sent {
		t.Error("e-mail de confirmação deveria ter sido enviado")
	}
	if user.Email != "ana@example.com" {
		t.Errorf("e-mail normalizado = %q, esperado %q", user.Email, "ana@example.com")
	}
	if user.EmailVerified {
		t.Error("usuário recém-cadastrado não deveria estar verificado")
	}

	// Login antes da confirmação é recusado, mesmo com senha correta.
	if _, _, err := service.Login(ctx, "ana@example.com", "SenhaForte#123", "127.0.0.1"); err != ErrEmailNotVerified {
		t.Errorf("Login() antes da confirmação = %v, esperado ErrEmailNotVerified", err)
	}

	// Confirmação pelo token do e-mail.
	if err := service.VerifyEmail(ctx, tokenFromLast(t, capture), "127.0.0.1"); err != nil {
		t.Fatalf("VerifyEmail() erro inesperado: %v", err)
	}

	// Senha errada devolve credenciais inválidas.
	if _, _, err := service.Login(ctx, "ana@example.com", "senha-errada", "127.0.0.1"); err != ErrInvalidCredentials {
		t.Errorf("Login() com senha errada = %v, esperado ErrInvalidCredentials", err)
	}

	// E-mail inexistente tem a MESMA resposta (anti-enumeração).
	if _, _, err := service.Login(ctx, "nobody@example.com", "SenhaForte#123", "127.0.0.1"); err != ErrInvalidCredentials {
		t.Errorf("Login() com e-mail inexistente = %v, esperado ErrInvalidCredentials", err)
	}

	// Login correto cria sessão válida.
	_, sessionToken, err := service.Login(ctx, "ana@example.com", "SenhaForte#123", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() erro inesperado: %v", err)
	}

	found, err := service.sessions.FindUser(ctx, sessionToken)
	if err != nil {
		t.Fatalf("FindUser() erro inesperado: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("usuário da sessão = %q, esperado %q", found.ID, user.ID)
	}
	if !found.EmailVerified() {
		t.Error("usuário confirmado deveria ter e-mail verificado")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	truncateAll(t)

	service := newTestService(&captureSender{})
	ctx := context.Background()

	if _, _, err := service.Register(ctx, "Ana", "ana@example.com", "SenhaForte#123", "127.0.0.1"); err != nil {
		t.Fatalf("Register() erro inesperado: %v", err)
	}

	// Mesmo e-mail com outra capitalização também conflita (normalização).
	if _, _, err := service.Register(ctx, "Outra Ana", "ANA@example.com", "outra-senha-123", "127.0.0.1"); err != ErrEmailTaken {
		t.Errorf("Register() duplicado = %v, esperado ErrEmailTaken", err)
	}
}

func TestVerifyEmailTokenExpiredAndReused(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	user, _, err := service.Register(ctx, "Ana", "ana@example.com", "SenhaForte#123", "127.0.0.1")
	if err != nil {
		t.Fatalf("Register() erro inesperado: %v", err)
	}

	// Token válido é consumido uma única vez.
	token := tokenFromLast(t, capture)
	if err := service.VerifyEmail(ctx, token, "127.0.0.1"); err != nil {
		t.Fatalf("VerifyEmail() erro inesperado: %v", err)
	}
	if err := service.VerifyEmail(ctx, token, "127.0.0.1"); err != ErrTokenUsed {
		t.Errorf("VerifyEmail() reutilizado = %v, esperado ErrTokenUsed", err)
	}

	// Token expirado é rejeitado.
	expired, hash, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken() erro inesperado: %v", err)
	}
	if err := service.store.CreateEmailVerificationToken(ctx, user.ID, hash, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateEmailVerificationToken() erro inesperado: %v", err)
	}
	if err := service.VerifyEmail(ctx, expired, "127.0.0.1"); err != ErrTokenExpired {
		t.Errorf("VerifyEmail() expirado = %v, esperado ErrTokenExpired", err)
	}

	// Token desconhecido é inválido.
	if err := service.VerifyEmail(ctx, "token-que-nao-existe", "127.0.0.1"); err != ErrInvalidToken {
		t.Errorf("VerifyEmail() desconhecido = %v, esperado ErrInvalidToken", err)
	}
}

func TestResendVerificationInvalidatesPreviousToken(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	if _, _, err := service.Register(ctx, "Ana", "ana@example.com", "SenhaForte#123", "127.0.0.1"); err != nil {
		t.Fatalf("Register() erro inesperado: %v", err)
	}

	oldToken := tokenFromLast(t, capture)

	if err := service.ResendVerification(ctx, "ana@example.com"); err != nil {
		t.Fatalf("ResendVerification() erro inesperado: %v", err)
	}

	newToken := tokenFromLast(t, capture)
	if newToken == oldToken {
		t.Fatal("reenvio deveria gerar token novo")
	}

	// O token antigo foi invalidado pelo reenvio.
	if err := service.VerifyEmail(ctx, oldToken, "127.0.0.1"); err != ErrInvalidToken {
		t.Errorf("VerifyEmail() com token antigo = %v, esperado ErrInvalidToken", err)
	}
	if err := service.VerifyEmail(ctx, newToken, "127.0.0.1"); err != nil {
		t.Errorf("VerifyEmail() com token novo = %v, esperado nil", err)
	}

	// Reenvio para conta desconhecida é silencioso (sem erro).
	if err := service.ResendVerification(ctx, "nobody@example.com"); err != nil {
		t.Errorf("ResendVerification() para conta desconhecida = %v, esperado nil", err)
	}
}

func TestForgotPasswordAndResetFlow(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	user := registerAndVerify(t, service, capture, "Ana", "ana@example.com", "SenhaAntiga#123")

	// Sessão criada antes da redefinição, para testar a revogação.
	sessionToken, err := service.sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create() erro inesperado: %v", err)
	}

	// Conta desconhecida: resposta genérica, sem erro e sem e-mail.
	before := len(capture.msgs)
	if err := service.ForgotPassword(ctx, "nobody@example.com", "127.0.0.1"); err != nil {
		t.Errorf("ForgotPassword() conta desconhecida = %v, esperado nil", err)
	}
	if len(capture.msgs) != before {
		t.Error("conta desconhecida não deveria gerar e-mail")
	}

	// Conta existente: e-mail de recuperação com token.
	if err := service.ForgotPassword(ctx, "ana@example.com", "127.0.0.1"); err != nil {
		t.Fatalf("ForgotPassword() erro inesperado: %v", err)
	}
	resetToken := tokenFromLast(t, capture)

	// Token inválido não passa.
	if err := service.ResetPassword(ctx, "token-invalido", "NovaSenha#456", "127.0.0.1"); err != ErrInvalidToken {
		t.Errorf("ResetPassword() token inválido = %v, esperado ErrInvalidToken", err)
	}

	// Redefinição válida.
	if err := service.ResetPassword(ctx, resetToken, "NovaSenha#456", "127.0.0.1"); err != nil {
		t.Fatalf("ResetPassword() erro inesperado: %v", err)
	}

	// Token não pode ser reutilizado.
	if err := service.ResetPassword(ctx, resetToken, "OutraSenha#789", "127.0.0.1"); err != ErrTokenUsed {
		t.Errorf("ResetPassword() reutilizado = %v, esperado ErrTokenUsed", err)
	}

	// Senha antiga não funciona mais; a nova funciona.
	if _, _, err := service.Login(ctx, "ana@example.com", "SenhaAntiga#123", "127.0.0.1"); err != ErrInvalidCredentials {
		t.Errorf("Login() com senha antiga = %v, esperado ErrInvalidCredentials", err)
	}
	if _, _, err := service.Login(ctx, "ana@example.com", "NovaSenha#456", "127.0.0.1"); err != nil {
		t.Errorf("Login() com nova senha = %v, esperado nil", err)
	}

	// Sessões anteriores foram revogadas.
	if _, err := service.sessions.FindUser(ctx, sessionToken); err != ErrSessionNotFound {
		t.Errorf("FindUser() sessão pré-redefinição = %v, esperado ErrSessionNotFound", err)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	user := registerAndVerify(t, service, capture, "Ana", "ana@example.com", "SenhaForte#123")

	_, sessionToken, err := service.Login(ctx, "ana@example.com", "SenhaForte#123", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() erro inesperado: %v", err)
	}

	if err := service.Logout(ctx, sessionToken); err != nil {
		t.Fatalf("Logout() erro inesperado: %v", err)
	}

	if _, err := service.sessions.FindUser(ctx, sessionToken); err != ErrSessionNotFound {
		t.Errorf("FindUser() após logout = %v, esperado ErrSessionNotFound", err)
	}

	// Logout é idempotente.
	if err := service.Logout(ctx, sessionToken); err != nil {
		t.Errorf("Logout() repetido = %v, esperado nil", err)
	}

	_ = user
}

func TestLoginRateLimitEndToEnd(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	registerAndVerify(t, service, capture, "Ana", "ana@example.com", "SenhaForte#123")

	// loginRateLimit = 10 tentativas por janela; a 11ª é bloqueada.
	for i := range loginRateLimit {
		if _, _, err := service.Login(ctx, "ana@example.com", "senha-errada", "127.0.0.1"); err != ErrInvalidCredentials {
			t.Fatalf("tentativa %d = %v, esperado ErrInvalidCredentials", i+1, err)
		}
	}

	_, _, err := service.Login(ctx, "ana@example.com", "senha-errada", "127.0.0.1")
	if _, ok := err.(RateLimitError); !ok {
		t.Errorf("11ª tentativa = %v, esperado RateLimitError", err)
	}
}

func TestRateLimitCounterPersisted(t *testing.T) {
	truncateAll(t)

	store := NewStore(testPool)
	ctx := context.Background()
	windowStart := time.Now().Truncate(time.Minute)

	count, err := store.Increment(ctx, "teste:chave", windowStart)
	if err != nil {
		t.Fatalf("Increment() erro inesperado: %v", err)
	}
	if count != 1 {
		t.Errorf("primeira contagem = %d, esperado 1", count)
	}

	count, err = store.Increment(ctx, "teste:chave", windowStart)
	if err != nil {
		t.Fatalf("Increment() erro inesperado: %v", err)
	}
	if count != 2 {
		t.Errorf("segunda contagem = %d, esperado 2", count)
	}

	// Janela diferente e chave diferente são independentes.
	count, err = store.Increment(ctx, "teste:chave", windowStart.Add(time.Minute))
	if err != nil {
		t.Fatalf("Increment() erro inesperado: %v", err)
	}
	if count != 1 {
		t.Errorf("janela nova deveria reiniciar: contagem = %d", count)
	}
}

func TestResetPasswordRateLimitEndToEnd(t *testing.T) {
	truncateAll(t)

	service := newTestService(&captureSender{})
	ctx := context.Background()

	// Token bem formado, mas inexistente: cada tentativa passa pelo rate
	// limiter antes da consulta ao banco.
	validShape, _, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken() erro inesperado: %v", err)
	}

	for i := range resetRateLimit {
		err := service.ResetPassword(ctx, validShape, "NovaSenha#456", "127.0.0.1")
		if err != ErrInvalidToken {
			t.Fatalf("tentativa %d = %v, esperado ErrInvalidToken", i+1, err)
		}
	}

	err = service.ResetPassword(ctx, validShape, "NovaSenha#456", "127.0.0.1")
	if _, ok := err.(RateLimitError); !ok {
		t.Errorf("tentativa %d = %v, esperado RateLimitError", resetRateLimit+1, err)
	}
}

func TestVerifyEmailConcurrentConsumption(t *testing.T) {
	truncateAll(t)

	capture := &captureSender{}
	service := newTestService(capture)
	ctx := context.Background()

	if _, _, err := service.Register(ctx, "Ana", "ana@example.com", "senhaForte#123", "127.0.0.1"); err != nil {
		t.Fatalf("Register() erro inesperado: %v", err)
	}
	token := tokenFromLast(t, capture)

	// 10 goroutines consomem o MESMO token ao mesmo tempo. Exatamente uma
	// deve vencer; as demais recebem ErrTokenUsed — o consumo é atômico.
	const attempts = 10

	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- service.VerifyEmail(ctx, token, "127.0.0.1")
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	used := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case err == ErrTokenUsed:
			used++
		default:
			t.Errorf("erro inesperado na confirmação concorrente: %v", err)
		}
	}

	if successes != 1 {
		t.Errorf("confirmações bem-sucedidas = %d, esperado exatamente 1", successes)
	}
	if used != attempts-1 {
		t.Errorf("tokens rejeitados como usados = %d, esperado %d", used, attempts-1)
	}
}
