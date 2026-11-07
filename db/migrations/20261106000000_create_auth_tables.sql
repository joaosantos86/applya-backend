-- +goose Up
-- Tabelas do módulo de autenticação: usuários, sessões, tokens de uso único
-- (confirmação de e-mail e recuperação de senha) e janelas de rate limiting.

-- Usuários. O e-mail é armazenado já normalizado (minúsculas, sem espaços) e
-- possui restrição UNIQUE; a senha é armazenada apenas como hash Argon2id.
CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    email             TEXT NOT NULL UNIQUE CHECK (char_length(email) <= 254),
    password_hash     TEXT NOT NULL,
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessões de usuário. Apenas o hash SHA-256 do token é armazenado; o token em
-- si vive somente no cookie do navegador. Uma sessão revogada (revoked_at)
-- nunca volta a ser válida.
CREATE TABLE sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- Tokens de uso único para confirmação de e-mail e recuperação de senha.
-- Uma única tabela com coluna de finalidade evita duplicação de estrutura.
-- Apenas o hash SHA-256 do token é armazenado; o consumo é atômico
-- (UPDATE condicionado a consumed_at IS NULL).
CREATE TABLE auth_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose     TEXT NOT NULL CHECK (purpose IN ('email_verification', 'password_reset')),
    token_hash  TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);

CREATE INDEX idx_auth_tokens_user_purpose ON auth_tokens (user_id, purpose);

-- Contadores de rate limiting em janela fixa. Armazenados no PostgreSQL para
-- que os limites valham para todas as instâncias da aplicação (ver
-- docs/decisions.md).
CREATE TABLE rate_limits (
    key          TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    count        INTEGER NOT NULL DEFAULT 1 CHECK (count >= 0),
    PRIMARY KEY (key, window_start)
);

-- +goose Down
DROP TABLE rate_limits;
DROP TABLE auth_tokens;
DROP TABLE sessions;
DROP TABLE users;
