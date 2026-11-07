package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store concentra a persistência do módulo de autenticação (pgx). Toda query
// das tabelas users, sessions, auth_tokens e rate_limits vive aqui.
//
// As queries são parametrizadas; nenhuma entrada do usuário é concatenada em
// SQL. UUIDs gerados pelo banco são lidos como texto e reutilizados como
// parâmetros via pgtype.UUID.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore cria o Store sobre o pool informado.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// userColumns é a projeção comum a todas as leituras de usuários. A ordem
// precisa corresponder a scanUser.
const userColumns = "id, name, email, password_hash, email_verified_at, created_at, updated_at"

// rowScanner descreve o contrato mínimo de pgx.Row e pgx.Rows necessário para
// ler um usuário.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanUser lê um usuário a partir de uma linha já projetada com userColumns.
func scanUser(row rowScanner) (User, error) {
	var u User
	var id pgtype.UUID
	var verifiedAt *time.Time

	if err := row.Scan(&id, &u.Name, &u.Email, &u.PasswordHash, &verifiedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return User{}, err
	}

	u.ID = id.String()
	u.EmailVerifiedAt = verifiedAt

	return u, nil
}

// userColumnsQualified devolve userColumns com o alias informado, para
// queries com JOIN (evita colunas ambíguas como created_at).
func userColumnsQualified(alias string) string {
	cols := strings.Split(userColumns, ", ")
	for i, col := range cols {
		cols[i] = alias + "." + col
	}
	return strings.Join(cols, ", ")
}

// uuidParam converte o ID textual (lido do banco) em parâmetro pgtype.UUID.
func uuidParam(id string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(id); err != nil {
		return u, fmt.Errorf("auth: id %q não é um uuid válido: %w", id, err)
	}
	return u, nil
}

// isUniqueViolation identifica violações de unicidade do PostgreSQL.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == constraint
}

// ---------------------------------------------------------------------------
// Usuários
// ---------------------------------------------------------------------------

// CreateUserWithVerificationToken cria o usuário e o token de confirmação de
// e-mail em uma única transação: nunca existe usuário sem token pendente (e
// vice-versa). Em corrida de cadastro com o mesmo e-mail devolve ErrEmailTaken.
func (s *Store) CreateUserWithVerificationToken(ctx context.Context, name, email, passwordHash, tokenHash string, tokenExpiresAt time.Time) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("iniciar transação de cadastro: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx,
		`INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3) RETURNING `+userColumns,
		name, email, passwordHash)
	user, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err, "users_email_key") {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("inserir usuário: %w", err)
	}

	userID, err := uuidParam(user.ID)
	if err != nil {
		return User{}, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auth_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, PurposeEmailVerification, tokenHash, tokenExpiresAt); err != nil {
		return User{}, fmt.Errorf("inserir token de confirmação: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("confirmar transação de cadastro: %w", err)
	}

	return user, nil
}

// FindUserByEmail busca o usuário pelo e-mail normalizado.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (User, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email)
	user, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("buscar usuário por e-mail: %w", err)
	}
	return user, nil
}

// ---------------------------------------------------------------------------
// Tokens de uso único
// ---------------------------------------------------------------------------

// tokenRow é a projeção das leituras de auth_tokens.
const tokenColumns = "id, user_id, purpose, token_hash, created_at, expires_at, consumed_at"

func scanToken(row rowScanner) (PurposeToken, error) {
	var t PurposeToken
	var id pgtype.UUID
	var userID pgtype.UUID

	if err := row.Scan(&id, &userID, &t.Purpose, &t.TokenHash, &t.CreatedAt, &t.ExpiresAt, &t.ConsumedAt); err != nil {
		return PurposeToken{}, err
	}

	t.ID = id.String()
	t.UserID = userID.String()

	return t, nil
}

// findToken lê um token pelo hash e finalidade dentro da transação informada.
func (s *Store) findToken(ctx context.Context, tx pgx.Tx, tokenHash, purpose string) (PurposeToken, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+tokenColumns+` FROM auth_tokens WHERE token_hash = $1 AND purpose = $2`,
		tokenHash, purpose)
	t, err := scanToken(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurposeToken{}, ErrInvalidToken
	}
	if err != nil {
		return PurposeToken{}, fmt.Errorf("buscar token: %w", err)
	}
	return t, nil
}

// consumeToken marca o token como consumido de forma atômica. Se outro
// processo já o consumiu (0 linhas afetadas), devolve ErrTokenUsed.
func consumeToken(ctx context.Context, tx pgx.Tx, tokenID string) error {
	ct, err := tx.Exec(ctx,
		`UPDATE auth_tokens SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`, tokenID)
	if err != nil {
		return fmt.Errorf("consumir token: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrTokenUsed
	}
	return nil
}

// CreateEmailVerificationToken remove os tokens de confirmação pendentes do
// usuário e insere um novo, em transação: apenas o token mais recente de cada
// finalidade é válido; os anteriores deixam de existir (uso posterior devolve
// ErrInvalidToken).
func (s *Store) CreateEmailVerificationToken(ctx context.Context, userID, tokenHash string, tokenExpiresAt time.Time) error {
	return s.createPurposeToken(ctx, userID, PurposeEmailVerification, tokenHash, tokenExpiresAt)
}

// CreatePasswordResetToken remove os tokens de recuperação pendentes do
// usuário e insere um novo, em transação (mesma semântica de unicidade).
func (s *Store) CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, tokenExpiresAt time.Time) error {
	return s.createPurposeToken(ctx, userID, PurposePasswordReset, tokenHash, tokenExpiresAt)
}

func (s *Store) createPurposeToken(ctx context.Context, userID, purpose, tokenHash string, tokenExpiresAt time.Time) error {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação de token: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM auth_tokens WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL`,
		userUUID, purpose); err != nil {
		return fmt.Errorf("invalidar tokens anteriores: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO auth_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userUUID, purpose, tokenHash, tokenExpiresAt); err != nil {
		return fmt.Errorf("inserir token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar transação de token: %w", err)
	}

	return nil
}

// ConsumeEmailVerificationToken consome o token e marca o e-mail do usuário
// como verificado, atomicamente. Devolve ErrInvalidToken, ErrTokenExpired ou
// ErrTokenUsed conforme o caso.
func (s *Store) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação de confirmação: %w", err)
	}
	defer tx.Rollback(ctx)

	t, err := s.findToken(ctx, tx, tokenHash, PurposeEmailVerification)
	if err != nil {
		return err
	}
	if t.Consumed() {
		return ErrTokenUsed
	}
	if t.Expired(now) {
		return ErrTokenExpired
	}

	if err := consumeToken(ctx, tx, t.ID); err != nil {
		return err
	}

	userUUID, err := uuidParam(t.UserID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET email_verified_at = now(), updated_at = now() WHERE id = $1 AND email_verified_at IS NULL`,
		userUUID); err != nil {
		return fmt.Errorf("marcar e-mail como verificado: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar transação de confirmação: %w", err)
	}

	return nil
}

// ConsumePasswordResetToken consome o token, troca a senha e revoga todas as
// sessões do usuário em uma única transação, devolvendo o usuário atualizado
// (para a notificação de senha alterada). Devolve ErrInvalidToken,
// ErrTokenExpired ou ErrTokenUsed conforme o caso.
func (s *Store) ConsumePasswordResetToken(ctx context.Context, tokenHash, newPasswordHash string, now time.Time) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("iniciar transação de redefinição: %w", err)
	}
	defer tx.Rollback(ctx)

	t, err := s.findToken(ctx, tx, tokenHash, PurposePasswordReset)
	if err != nil {
		return User{}, err
	}
	if t.Consumed() {
		return User{}, ErrTokenUsed
	}
	if t.Expired(now) {
		return User{}, ErrTokenExpired
	}

	if err := consumeToken(ctx, tx, t.ID); err != nil {
		return User{}, err
	}

	userUUID, err := uuidParam(t.UserID)
	if err != nil {
		return User{}, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		newPasswordHash, userUUID); err != nil {
		return User{}, fmt.Errorf("atualizar senha: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userUUID); err != nil {
		return User{}, fmt.Errorf("revogar sessões do usuário: %w", err)
	}

	user, err := scanUser(tx.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, userUUID))
	if err != nil {
		return User{}, fmt.Errorf("ler usuário após redefinição: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("confirmar transação de redefinição: %w", err)
	}

	return user, nil
}

// ---------------------------------------------------------------------------
// Sessões
// ---------------------------------------------------------------------------

// CreateSession persiste uma nova sessão para o usuário.
func (s *Store) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return err
	}

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userUUID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("criar sessão: %w", err)
	}

	return nil
}

// FindUserBySessionToken devolve o usuário dono da sessão, desde que ela
// exista, não esteja revogada e não tenha expirado.
func (s *Store) FindUserBySessionToken(ctx context.Context, tokenHash string) (User, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+userColumnsQualified("u")+` FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()`,
		tokenHash)
	user, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrSessionNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("buscar sessão: %w", err)
	}
	return user, nil
}

// RevokeSession revoga a sessão identificada pelo hash do token. A operação é
// idempotente: revogar uma sessão inexistente ou já revogada não é erro.
func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash); err != nil {
		return fmt.Errorf("revogar sessão: %w", err)
	}
	return nil
}

// RevokeAllUserSessions revoga todas as sessões ativas do usuário. Usada na
// redefinição de senha para encerrar qualquer acesso anterior.
func (s *Store) RevokeAllUserSessions(ctx context.Context, userID string) error {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return err
	}

	if _, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userUUID); err != nil {
		return fmt.Errorf("revogar sessões do usuário: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------------

// rateLimitPruneEvery controla a frequência (1 em N chamadas) da limpeza de
// janelas antigas da tabela rate_limits.
const rateLimitPruneEvery = 100

// rateLimitPruneOlderThan remove janelas mais antigas que este intervalo.
const rateLimitPruneOlderThan = 24 * time.Hour

// Increment incrementa o contador da janela fixa (key, windowStart) e
// devolve o total acumulado. Implementa a interface Counter do rate limiter:
// a janela fixa por chave permite que todas as instâncias da aplicação
// compartilhem os mesmos limites.
//
// Para evitar crescimento infinito da tabela, janelas antigas são removidas
// probabilisticamente.
func (s *Store) Increment(ctx context.Context, key string, windowStart time.Time) (int64, error) {
	var count int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO rate_limits (key, window_start, count) VALUES ($1, $2, 1)
		 ON CONFLICT (key, window_start) DO UPDATE SET count = rate_limits.count + 1
		 RETURNING count`,
		key, windowStart).Scan(&count); err != nil {
		return 0, fmt.Errorf("incrementar rate limit: %w", err)
	}

	s.pruneRateLimits(ctx)

	return count, nil
}

// pruneRateLimits remove janelas antigas com probabilidade 1/N, limitando o
// tamanho da tabela sem adicionar um job separado.
func (s *Store) pruneRateLimits(ctx context.Context) {
	if time.Now().UnixNano()%rateLimitPruneEvery != 0 {
		return
	}

	cutoff := time.Now().Add(-rateLimitPruneOlderThan)
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM rate_limits WHERE window_start < $1`, cutoff); err != nil {
		// A limpeza é best-effort; falhar aqui não afeta a requisição.
		return
	}
}
