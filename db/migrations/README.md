# Migrations

As migrations de banco são versionadas nesta pasta e gerenciadas pelo
[Goose](https://github.com/pressly/goose) no formato SQL.

Nenhuma migration é executada automaticamente quando a API inicia: o
gerenciamento do esquema é sempre explícito.

## Convenção

- Um arquivo por migration: `AAAAAMMDDHHMMSS_descricao_curta.sql`.
- Sempre implemente `-- +goose Up` e `-- +goose Down`.
- O `Down` deve reverter exatamente o que o `Up` criou.
- Use `-- +goose StatementBegin` / `-- +goose StatementEnd` em blocos com
  funções ou procedures.

Exemplo de nome:

```text
20260101120000_create_profiles.sql
```

## Comandos

Com o Goose instalado (`go install github.com/pressly/goose/v3/cmd/goose@latest`)
e `DATABASE_URL` definida no ambiente:

```bash
# criar uma nova migration vazia
goose -dir db/migrations create create_profiles sql

# aplicar todas as migrations pendentes
goose -dir db/migrations postgres "$DATABASE_URL" up

# reverter a última migration aplicada
goose -dir db/migrations postgres "$DATABASE_URL" down

# reverter até uma versão específica
goose -dir db/migrations postgres "$DATABASE_URL" down-to 20260101120000

# consultar o status
goose -dir db/migrations postgres "$DATABASE_URL" status
```

No PowerShell, use `$env:DATABASE_URL` em vez de `$DATABASE_URL`.

## Estado atual

Nenhuma tabela foi criada ainda. As primeiras migrations serão adicionadas junto
com os módulos de usuários, perfis e currículos.
