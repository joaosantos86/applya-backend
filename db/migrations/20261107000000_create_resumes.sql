-- +goose Up
-- Currículos: um currículo-base por usuário e versões independentes
-- derivadas dele.
--
-- Decisão de modelagem: os dados profissionais ficam em um único documento
-- JSONB, enquanto identidade, propriedade, tipo e ciclo de vida ficam em
-- colunas relacionais. O documento é sempre lido/gravado inteiro, as seções são
-- listas ordenadas e não há consultas relacionais dentro delas neste MVP.
-- Justificativa completa em docs/decisions.md.
CREATE TABLE resumes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    base_resume_id UUID,
    kind           TEXT NOT NULL CHECK (kind IN ('base', 'version')),
    name           TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    data           JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    revision       BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),

    -- base_kind é sempre 'base' nas versões e NULL no currículo-base. Ela
    -- participa da chave estrangeira abaixo para garantir, no banco, que uma
    -- versão só aponta para um currículo que também é do tipo base.
    base_kind      TEXT,

    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Currículo-base não tem pai; versão sempre tem.
    CONSTRAINT resumes_base_consistency CHECK (
        (kind = 'base' AND base_resume_id IS NULL AND base_kind IS NULL) OR
        (kind = 'version' AND base_resume_id IS NOT NULL AND base_kind = 'base')
    ),

    -- Uma versão não pode referenciar a si mesma.
    CONSTRAINT resumes_no_self_reference CHECK (base_resume_id IS NULL OR base_resume_id <> id),

    -- Chave necessária para a FK composta abaixo.
    CONSTRAINT resumes_id_user_kind_key UNIQUE (id, user_id, kind),

    -- Garante que o pai pertence ao mesmo usuário e é do tipo base. A FK só é
    -- verificada quando base_resume_id não é NULL (currículo-base).
    CONSTRAINT resumes_base_same_user_kind
        FOREIGN KEY (base_resume_id, user_id, base_kind)
        REFERENCES resumes (id, user_id, kind)
        ON DELETE CASCADE
);

-- No máximo um currículo-base por usuário. O índice parcial é o alvo do
-- INSERT ... ON CONFLICT usado na criação automática e segura sob concorrência.
CREATE UNIQUE INDEX resumes_one_base_per_user
    ON resumes (user_id)
    WHERE kind = 'base';

-- Consultas da sidebar: versões de um usuário ordenadas pela última alteração.
CREATE INDEX resumes_user_kind_updated_idx
    ON resumes (user_id, kind, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS resumes;
