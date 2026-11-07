# Applya API

Backend do Applya, um projeto pessoal para criar, personalizar e organizar
currículos adaptados a vagas. O frontend é uma aplicação Vue separada. Este
repositório fornece a API REST, a autenticação e o workspace de currículos.

## Stack

- Go 1.27
- `net/http` e Chi
- PostgreSQL com pgx
- Migrations com Goose
- Hash de senha com Argon2id
- Sessões com cookies HTTP-only

## Funcionalidades

- health check;
- cadastro e confirmação de e-mail;
- login, logout e consulta do usuário atual;
- recuperação e redefinição de senha;
- sessões e rate limiting persistidos no PostgreSQL;
- proteção CSRF, CORS e logging estruturado;
- envio de e-mail por SMTP, Resend ou log local;
- currículo-base único por usuário, criado automaticamente;
- versões independentes criadas a partir do currículo-base;
- edição de dados pessoais, resumo, experiências, formação, habilidades,
  projetos e idiomas;
- endpoint de workspace para o carregamento inicial da interface.

## Requisitos

- Go 1.27 ou mais recente;
- PostgreSQL 16 ou mais recente;
- [Goose](https://github.com/pressly/goose) para migrations.

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

No Windows, adicione `%USERPROFILE%\go\bin` ao `PATH` para chamar `goose`
diretamente.

## Setup local

Copie o template de ambiente:

```powershell
Copy-Item .env.example .env
```

Ajuste `DATABASE_URL` se necessário. O valor local padrão é:

```text
postgres://applya:applya@localhost:5432/applya?sslmode=disable
```

Carregue o ambiente e aplique as migrations:

```powershell
Get-Content .env | Where-Object { $_ -match "^\s*[^#].*=" } | ForEach-Object {
  $name, $value = $_ -split "=", 2
  Set-Item -Path "Env:$($name.Trim())" -Value $value.Trim()
}

goose -dir db/migrations postgres $env:DATABASE_URL up
```

Inicie a API:

```powershell
.\scripts\run.ps1
```

O endereço padrão é `http://localhost:8080`.

## Ambiente

O serviço lê as variáveis na inicialização. `scripts/run.ps1` carrega `.env`
localmente; plataformas de hospedagem devem fornecer os mesmos valores pela
interface de variáveis de ambiente.

| Variável | Obrigatória | Descrição |
| --- | --- | --- |
| `APP_ENV` | Não | `development`, `test` ou `production` |
| `HTTP_PORT` | Não | Porta HTTP; padrão `8080` |
| `DATABASE_URL` | Sim | String de conexão do PostgreSQL |
| `LOG_LEVEL` | Não | `debug`, `info`, `warn` ou `error` |
| `CORS_ALLOWED_ORIGINS` | Sim | Origens do frontend, explícitas e separadas por vírgula |
| `FRONTEND_URL` | Sim | URL base usada nos links de e-mail |
| `SESSION_DURATION` | Não | Duração da sessão; padrão `720h` |
| `EMAIL_PROVIDER` | Não | `log`, `resend` ou `smtp` |
| `EMAIL_FROM` | SMTP/Resend | Endereço remetente |
| `EMAIL_API_KEY` | Resend | Chave da API do Resend |
| `SMTP_HOST` | SMTP | Host SMTP |
| `SMTP_PORT` | SMTP | Porta com STARTTLS, normalmente `587` |
| `SMTP_USERNAME` | SMTP | Usuário SMTP |
| `SMTP_PASSWORD` | SMTP | Senha de app ou credencial do provedor |
| `COOKIE_SAMESITE` | Não | `lax`, `strict` ou `none` |
| `TRUST_PROXY_HEADERS` | Não | Habilite apenas atrás de proxy confiável |

Origens devem ser explícitas. `CORS_ALLOWED_ORIGINS=*` é rejeitado.

### E-mail

`EMAIL_PROVIDER=log` é destinado ao desenvolvimento local. Ele escreve o e-mail
completo e o link de confirmação no log da API em vez de enviar a mensagem.

SMTP exige STARTTLS e TLS 1.2 ou mais recente. Exemplo com Gmail:

```text
EMAIL_PROVIDER=smtp
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=seu-endereco@gmail.com
SMTP_PASSWORD=sua-senha-de-app-de-16-caracteres
EMAIL_FROM=Applya <seu-endereco@gmail.com>
```

Use uma senha de app do Google, não a senha de login da conta.

Para o Resend:

```text
EMAIL_PROVIDER=resend
EMAIL_API_KEY=re_...
EMAIL_FROM=Applya <no-reply@seu-dominio.com>
```

## API

```text
GET /health
```

```json
{"status":"ok"}
```

### Autenticação

| Método | Rota | Descrição |
| --- | --- | --- |
| `POST` | `/api/v1/auth/register` | Cria uma conta não confirmada |
| `POST` | `/api/v1/auth/verify-email` | Confirma o e-mail com token de uso único |
| `POST` | `/api/v1/auth/resend-verification` | Reenvia o link de confirmação |
| `POST` | `/api/v1/auth/login` | Cria uma sessão |
| `POST` | `/api/v1/auth/logout` | Invalida a sessão |
| `GET` | `/api/v1/auth/me` | Retorna o usuário autenticado |
| `GET` | `/api/v1/auth/csrf` | Retorna e define um token CSRF |
| `POST` | `/api/v1/auth/forgot-password` | Solicita um link de recuperação |
| `POST` | `/api/v1/auth/reset-password` | Redefine a senha com token de uso único |

`GET /api/v1/auth/me` responde `401 authentication_required` sem sessão válida —
esse é o estado normal de usuário deslogado.

### Currículos

Todas as rotas exigem sessão autenticada. O proprietário é sempre o usuário da
sessão; a API nunca aceita `user_id` do cliente.

| Método | Rota | Descrição |
| --- | --- | --- |
| `GET` | `/api/v1/resumes/base` | Obtém (criando se necessário) o currículo-base |
| `PATCH` | `/api/v1/resumes/base` | Atualiza o currículo-base |
| `GET` | `/api/v1/resumes/versions` | Lista as versões do usuário |
| `POST` | `/api/v1/resumes/versions` | Cria uma versão a partir do base |
| `GET` | `/api/v1/resumes/versions/{id}` | Obtém uma versão completa |
| `PATCH` | `/api/v1/resumes/versions/{id}` | Atualiza uma versão |
| `DELETE` | `/api/v1/resumes/versions/{id}` | Exclui uma versão |
| `GET` | `/api/v1/workspace` | Carrega usuário, base e versões de uma vez |

Regras principais:

- cada usuário tem exatamente um currículo-base lógico, criado na primeira
  chamada e garantido por índice único no PostgreSQL;
- uma versão nasce como cópia independente do currículo-base;
- alterar a versão não altera o base e alterar o base não altera versões;
- o currículo-base não pode ser excluído pelas rotas de versão;
- `PATCH` é parcial por seção: seções omitidas são preservadas; seções enviadas
  substituem a seção inteira; `[]` limpa uma coleção;
- `revision` permite concorrência otimista e responde `409 conflict` quando
  desatualizada.

Exemplo de criação de versão:

```bash
curl -X POST http://localhost:8080/api/v1/resumes/versions \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  --cookie "applya_session=$SESSION; applya_csrf=$CSRF_TOKEN" \
  -d '{"name":"Backend Go"}'
```

### Cookies e CSRF

A API usa o cookie HTTP-only `applya_session`. Toda requisição que muda estado
precisa de proteção CSRF:

1. chame `GET /api/v1/auth/csrf` com `credentials: 'include'`;
2. mantenha o cookie `applya_csrf`;
3. envie o valor no header `X-CSRF-Token` em `POST`, `PUT`, `PATCH` e `DELETE`.

A API também valida `Origin` ou `Referer` quando presentes. Requisições CSRF
inválidas recebem `403 csrf_failed`. CSRF nunca é desabilitado para facilitar a
integração.

```ts
await fetch("http://localhost:8080/api/v1/resumes/versions", {
  method: "POST",
  credentials: "include",
  headers: {
    "Content-Type": "application/json",
    "X-CSRF-Token": csrfToken,
  },
  body: JSON.stringify({ name: "Backend Go" }),
})
```

Frontend e API no mesmo site (subdomínios do mesmo domínio registrável) com
`COOKIE_SAMESITE=lax` é a configuração recomendada, especialmente em
dispositivos móveis. Em sites diferentes, `COOKIE_SAMESITE=none` exige cookies
de terceiros, que navegadores móveis podem bloquear.

### Formato de erro

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Verifique os campos enviados.",
    "request_id": "..."
  }
}
```

Códigos comuns: `validation_failed`, `authentication_required`,
`invalid_credentials`, `email_not_verified`, `csrf_failed`, `rate_limited`,
`not_found`, `conflict`, `payload_too_large` e `internal_error`.

## CORS

A configuração local do frontend é:

```text
CORS_ALLOWED_ORIGINS=http://localhost:5173
```

O middleware CORS permite credenciais, métodos da API, `Content-Type`,
`X-CSRF-Token` e preflight `OPTIONS`. Ele roda antes da autenticação e do CSRF
das rotas.

## Migrations

As migrations ficam em `db/migrations` e nunca são executadas automaticamente
pela API.

```bash
goose -dir db/migrations create create_profiles sql
goose -dir db/migrations postgres "$DATABASE_URL" up
goose -dir db/migrations postgres "$DATABASE_URL" status
goose -dir db/migrations postgres "$DATABASE_URL" down
```

Migrations existentes:

- `20261106000000_create_auth_tables.sql` — `users`, `sessions`, `auth_tokens`
  e `rate_limits`;
- `20261107000000_create_resumes.sql` — `resumes`, com o índice único de
  currículo-base por usuário e as constraints de relação base/versão.

## Verificações

```bash
gofmt -l .
go vet ./...
staticcheck ./...
govulncheck ./...
go test ./...
go build ./cmd/api
gitleaks git --log-opts=--all
```

Testes unitários não precisam de PostgreSQL. Testes de integração exigem um
banco descartável e são executados separadamente por pacote (ambos os pacotes
compartilham a tabela `users` em um banco de teste):

```bash
go test -tags=integration ./internal/auth
go test -tags=integration ./internal/resume
```

## Deploy

A API pode rodar no Render com PostgreSQL (por exemplo, Neon). Mantenha o
`.env` local e configure o deploy pelo painel da plataforma:

```text
APP_ENV=production
HTTP_PORT=10000
DATABASE_URL=postgres://...?sslmode=require
CORS_ALLOWED_ORIGINS=https://seu-frontend.vercel.app
FRONTEND_URL=https://seu-frontend.vercel.app
COOKIE_SAMESITE=none
TRUST_PROXY_HEADERS=true
```

Para frontend e API em sites diferentes, `COOKIE_SAMESITE=none` é necessário.
Cookies de produção recebem `Secure` automaticamente.

## Estrutura do projeto

```text
cmd/api/                 bootstrap da aplicação
internal/auth/           sessões, tokens, CSRF e rate limiting
internal/config/         validação das variáveis de ambiente
internal/database/       ciclo de vida do pool PostgreSQL
internal/email/          templates e envio por SMTP/Resend
internal/http/           handlers, middlewares, respostas e rotas
internal/resume/         currículos: domínio, validação e persistência
db/migrations/           migrations Goose
```

## Segurança

- Senhas usam Argon2id.
- Tokens de sessão, confirmação e recuperação vêm de `crypto/rand` e são
  armazenados como hash.
- Tokens de confirmação expiram em 24 horas; os de recuperação, em 30 minutos.
- Todas as consultas de currículos filtram pelo usuário da sessão.
- Nunca faça commit de `.env`, URLs de banco, credenciais SMTP ou chaves de
  provedor.

## Licença

Projeto pessoal de portfólio e aprendizado.
