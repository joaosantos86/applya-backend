# Applya — Backend

Backend do **Applya**, uma aplicação pessoal para criar, editar, personalizar,
organizar e exportar versões de currículo adaptadas a diferentes oportunidades
de emprego.

O projeto tem dois repositórios independentes:

| Repositório | Stack | Responsabilidade |
|---|---|---|
| [`applya-frontend`](https://github.com/joaosantos86/applya-frontend) | Vue 3 + TypeScript | Interface do usuário |
| `applya-backend` | Go | API REST, regras de negócio e persistência |

A comunicação entre eles será feita por uma API REST com JSON sobre HTTP/HTTPS.

> Este é um projeto pessoal, criado para aprendizado, portfólio e demonstração de
> habilidades técnicas. Não é um SaaS comercial: não há planos, assinaturas ou
> monetização.

---

## Objetivo do backend

Este repositório contém a **fundação técnica** da API e a **autenticação
completa**:

- servidor HTTP com timeouts e encerramento gracioso;
- roteamento com Chi;
- configuração e validação por variáveis de ambiente;
- logging estruturado com `log/slog`;
- pool de conexões PostgreSQL com `pgxpool`;
- estrutura de migrations com Goose;
- CORS configurável;
- tratamento padronizado de erros em JSON;
- healthcheck em `GET /health`;
- **autenticação**: cadastro, confirmação de e-mail, login, logout, sessões,
  recuperação e redefinição de senha, CSRF e rate limiting (ver
  [Autenticação](#autenticação));
- e-mails transacionais (Resend, com fake `log` para desenvolvimento);
- testes automatizados (unitários e de integração).

Currículos, perfis profissionais e exportação em PDF **ainda não foram
implementados** e serão adicionados em etapas futuras.

---

## Tecnologias

| Tecnologia | Uso |
|---|---|
| [Go](https://go.dev) | Linguagem principal |
| [Chi](https://github.com/go-chi/chi) | Roteamento HTTP |
| [pgx / pgxpool](https://github.com/jackc/pgx) | Driver e pool de conexões PostgreSQL |
| [Goose](https://github.com/pressly/goose) | Migrations SQL |
| [PostgreSQL](https://www.postgresql.org) | Banco de dados relacional |
| [Argon2id](https://golang.org/x/crypto/argon2) | Hash de senhas |
| [Resend](https://resend.com) | E-mails transacionais (API HTTP) |
| `log/slog` | Logging estruturado |
| `net/http` | Servidor HTTP e cliente HTTP do Resend |
| `testing` + `httptest` | Testes automatizados |

Versões das dependências Go estão fixadas em `go.mod` / `go.sum`.

---

## Pré-requisitos

- **Go 1.27** ou superior (`go version`)
- **PostgreSQL 16+** instalado e em execução localmente
- **Git**
- (opcional) **Goose CLI** para gerenciar migrations

Não é necessário Docker, Kubernetes ou qualquer serviço de infraestrutura.

---

## Instalação

```bash
git clone https://github.com/joaosantos86/applya-backend.git
cd applya-backend
go mod download
```

Para instalar ferramentas auxiliares de desenvolvimento (opcional, apenas
migrations):

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

O binário é instalado em `$(go env GOPATH)/bin`. No Windows, garanta que
`%USERPROFILE%\go\bin` esteja no `PATH` para usar `goose` diretamente.

---

## Configuração do PostgreSQL local

O PostgreSQL roda diretamente na máquina do desenvolvedor. Nenhum container é
usado.

### 1. Verificar se o serviço está em execução

Windows (PowerShell):

```powershell
Get-Service *postgres*
Test-NetConnection localhost -Port 5432
```

Linux/macOS:

```bash
pg_isready -h localhost -p 5432
```

### 2. Criar usuário e banco

Usando `psql` (no Windows ele costuma ficar em
`C:\Program Files\PostgreSQL\<versão>\bin\psql.exe`). O `psql` pedirá a senha do
usuário `postgres`:

```powershell
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -U postgres -h localhost -c "CREATE USER applya WITH PASSWORD 'applya';"
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -U postgres -h localhost -c "CREATE DATABASE applya OWNER applya;"
```

No Linux/macOS:

```bash
psql -U postgres -h localhost -c "CREATE USER applya WITH PASSWORD 'applya';"
psql -U postgres -h localhost -c "CREATE DATABASE applya OWNER applya;"
```

Também é possível criar usuário e banco pelo pgAdmin 4 (Login/Group Role e
Database).

> Use uma senha diferente em qualquer ambiente que não seja a sua máquina local.

### 3. Montar a `DATABASE_URL`

```text
postgres://applya:applya@localhost:5432/applya?sslmode=disable
```

`sslmode=disable` é aceitável apenas para conexões locais.

### 4. Testar a conexão

```bash
psql "postgres://applya:applya@localhost:5432/applya?sslmode=disable" -c "select 1;"
```

### 5. Diagnosticar problemas comuns

| Problema | Causa provável | Solução |
|---|---|---|
| `connection refused` | PostgreSQL não está rodando | Inicie o serviço do PostgreSQL |
| `password authentication failed` | Usuário/senha incorretos | Revise a `DATABASE_URL` |
| `database "applya" does not exist` | Banco não criado | Execute o `CREATE DATABASE` |
| `role "applya" does not exist` | Usuário não criado | Execute o `CREATE USER` |
| `pg_hba.conf` rejeitando | Regra de autenticação local | Ajuste o `pg_hba.conf` e reinicie o serviço |
| Porta 5432 ocupada | Outra instância rodando | Use outra porta e ajuste a URL |

A API **falha ao iniciar** com mensagem descritiva quando não consegue conectar.
Isso é intencional: sem banco, nenhuma funcionalidade real funcionaria.

---

## Variáveis de ambiente

Copie `.env.example` como referência (o arquivo `.env` é ignorado pelo Git):

```bash
cp .env.example .env
```

> O carregamento automático de `.env` **não** é feito pela aplicação. As
> variáveis precisam estar no ambiente do processo. Carregue-as no shell antes
> de iniciar a API.

### Variáveis

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `APP_ENV` | não | `development` | `development`, `test` ou `production` |
| `HTTP_PORT` | não | `8080` | Porta TCP do servidor HTTP (1–65535) |
| `DATABASE_URL` | **sim** | — | String de conexão do PostgreSQL |
| `LOG_LEVEL` | não | `info` | `debug`, `info`, `warn` ou `error` |
| `CORS_ALLOWED_ORIGINS` | **sim** | — | Origens permitidas, separadas por vírgula (`*` é rejeitado) |
| `FRONTEND_URL` | **sim** | — | Origem do frontend usada nos links de e-mail (ex.: `https://applya.dev`) |
| `SESSION_DURATION` | não | `720h` | Tempo de vida das sessões (ex.: `24h`, `168h`) |
| `EMAIL_PROVIDER` | não | `log` | `log` (dev/testes), `resend` ou `smtp` |
| `EMAIL_API_KEY` | sim, se `resend` | — | Chave da API do Resend |
| `EMAIL_FROM` | sim, se `resend`/`smtp` | — | Remetente dos e-mails (ex.: `Applya <no-reply@applya.dev>`) |
| `SMTP_HOST` | sim, se `smtp` | — | Host do servidor SMTP (ex.: `smtp.gmail.com`) |
| `SMTP_PORT` | sim, se `smtp` | — | Porta STARTTLS do SMTP (normalmente `587`) |
| `SMTP_USERNAME` | sim, se `smtp` | — | Usuário SMTP |
| `SMTP_PASSWORD` | sim, se `smtp` | — | Senha de app ou credencial SMTP |
| `COOKIE_SAMESITE` | não | `lax` | `lax`, `strict` ou `none` (ver [Cookies e CSRF](#cookies-e-csrf)) |
| `TRUST_PROXY_HEADERS` | não | `false` | Confia em `X-Forwarded-For` para o IP do cliente; use `true` **apenas** atrás de proxy confiável (Render) |

A validação ocorre no startup e todos os problemas são reportados de uma vez.
Nenhuma credencial é exibida nas mensagens de erro.

### Carregar variáveis no desenvolvimento

A forma mais simples no Windows é usar o script `scripts/run.ps1`, que carrega
o `.env` e sobe a API em um único comando (a partir da raiz do projeto):

```powershell
.\scripts\run.ps1
```

Ele confere as variáveis obrigatórias e não imprime valores (podem ser
segredos). Se o PowerShell bloquear scripts, use:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\run.ps1
```

Alternativa manual — exporte as variáveis no próprio terminal:

Windows — PowerShell:

```powershell
$env:APP_ENV = "development"
$env:HTTP_PORT = "8080"
$env:DATABASE_URL = "postgres://applya:applya@localhost:5432/applya?sslmode=disable"
$env:LOG_LEVEL = "debug"
$env:CORS_ALLOWED_ORIGINS = "http://localhost:5173"
$env:FRONTEND_URL = "http://localhost:5173"
$env:SESSION_DURATION = "720h"
$env:EMAIL_PROVIDER = "log"
```

Windows — Prompt de Comando (`cmd.exe`):

```bat
set APP_ENV=development
set HTTP_PORT=8080
set DATABASE_URL=postgres://applya:applya@localhost:5432/applya?sslmode=disable
set LOG_LEVEL=debug
set CORS_ALLOWED_ORIGINS=http://localhost:5173
set FRONTEND_URL=http://localhost:5173
set SESSION_DURATION=720h
set EMAIL_PROVIDER=log
```

Linux/macOS — Bash/Zsh:

```bash
export APP_ENV=development
export HTTP_PORT=8080
export DATABASE_URL="postgres://applya:applya@localhost:5432/applya?sslmode=disable"
export LOG_LEVEL=debug
export CORS_ALLOWED_ORIGINS="http://localhost:5173"
export FRONTEND_URL="http://localhost:5173"
export SESSION_DURATION="720h"
export EMAIL_PROVIDER="log"
```

Uma alternativa simples no PowerShell é ler o `.env` e exportar as variáveis:

```powershell
Get-Content .env | Where-Object { $_ -match "^\s*[^#].*=" } | ForEach-Object {
  $name, $value = $_ -split "=", 2
  Set-Item -Path "Env:$($name.Trim())" -Value $value.Trim()
}
```

---

## Execução da API

```bash
go run ./cmd/api
```

Saída esperada (formato texto em desenvolvimento):

```text
time=... level=INFO msg="iniciando applya api" service=applya-api env=development http_port=8080 log_level=INFO
time=... level=INFO msg="conexão com postgres estabelecida" service=applya-api host=localhost database=applya
time=... level=INFO msg="servidor http escutando" service=applya-api addr=:8080
```

Teste o healthcheck:

```bash
curl -i http://localhost:8080/health
```

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
X-Request-Id: 1-9f2c1a4b3d5e6f70

{"status":"ok"}
```

Para encerrar, use `Ctrl+C`: o servidor faz *graceful shutdown*, aguardando até
10 segundos pelas requisições em andamento.

Compilar um binário local:

```bash
go build -o applya-api ./cmd/api
```

Binários gerados não são versionados (ver `.gitignore`).

### Configuração do servidor HTTP

| Parâmetro | Valor |
|---|---|
| ReadHeaderTimeout | 5s |
| ReadTimeout | 10s |
| WriteTimeout | 15s |
| IdleTimeout | 60s |
| Shutdown | 10s |

---

## Estrutura de diretórios

```text
applya-backend/
├── cmd/
│   └── api/
│       └── main.go              # bootstrap: config, logger, pool, módulos, servidor
├── internal/
│   ├── auth/                    # autenticação completa
│   │   ├── auth.go              # tipos de domínio e erros
│   │   ├── service.go           # regras de negócio (cadastro, login, reset...)
│   │   ├── queries.go           # SQL/pgx (users, sessions, tokens, rate limits)
│   │   ├── password.go          # Argon2id (formato PHC)
│   │   ├── tokens.go            # tokens crypto/rand + hash SHA-256
│   │   ├── session.go           # criação/validação/revogação de sessões
│   │   ├── cookies.go           # cookies de sessão e CSRF
│   │   ├── middleware.go        # RequireAuth + contexto do usuário + CSRF
│   │   ├── ratelimit.go         # limiter de janela fixa (contador injetável)
│   │   ├── validation.go        # validação de entradas
│   │   └── *_test.go
│   ├── config/
│   │   ├── config.go            # leitura e validação das variáveis de ambiente
│   │   └── config_test.go
│   ├── database/
│   │   ├── postgres.go          # criação e encerramento do pgxpool
│   │   ├── postgres_test.go
│   │   └── postgres_integration_test.go
│   ├── email/                   # e-mails transacionais
│   │   ├── email.go             # interface Sender, LogSender e ResendSender
│   │   ├── templates.go         # Mailer e templates pt-BR
│   │   └── email_test.go
│   └── http/
│       ├── handlers/
│       │   ├── auth.go          # endpoints de autenticação
│       │   ├── auth_test.go
│       │   ├── health.go        # GET /health
│       │   └── health_test.go
│       ├── middleware/
│       │   ├── middleware.go    # request ID, logging, recoverer, CORS
│       │   └── middleware_test.go
│       ├── response/
│       │   └── response.go      # respostas JSON e erros padronizados
│       ├── router/
│       │   ├── router.go        # rotas e middlewares
│       │   ├── router_test.go
│       │   └── router_auth_test.go
│       └── server/
│           ├── server.go        # timeouts, ListenAndServe e graceful shutdown
│           └── server_test.go
├── db/
│   └── migrations/              # migrations SQL (Goose) + instruções
├── docs/
│   └── decisions.md             # decisões técnicas e justificativas
├── scripts/
│   └── run.ps1                  # carrega o .env e sobe a API (Windows)
├── .env.example
├── .gitignore
├── AGENTS.md
├── README.md
├── go.mod
└── go.sum
```

---

## Endpoint de healthcheck

### `GET /health`

Verifica **somente** se o processo HTTP está respondendo. Não consulta o banco
nem serviços externos.

Resposta `200 OK`:

```json
{
  "status": "ok"
}
```

---

## Autenticação

A autenticação é baseada em **sessões persistidas no PostgreSQL**: o token de
sessão vive em um cookie HTTP-only no navegador e o banco guarda apenas o hash
SHA-256 dele. Não há JWT. Senhas usam **Argon2id**; tokens de confirmação e
recuperação são aleatórios (`crypto/rand`, 256 bits), de uso único, armazenados
por hash e com expiração.

### Endpoints

Todas as rotas ficam sob `/api/v1/auth`:

| Método | Rota | Auth | Descrição |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | — | Cria conta (não verificada) e envia link de confirmação |
| `POST` | `/api/v1/auth/verify-email` | — | Confirma o e-mail a partir do token do link |
| `POST` | `/api/v1/auth/resend-verification` | — | Reenvia o link de confirmação |
| `POST` | `/api/v1/auth/login` | — | Valida credenciais e cria sessão (cookie) |
| `POST` | `/api/v1/auth/logout` | — | Revoga a sessão atual e remove o cookie |
| `GET` | `/api/v1/auth/me` | **sim** | Dados do usuário autenticado |
| `POST` | `/api/v1/auth/forgot-password` | — | Envia link de redefinição (resposta sempre genérica) |
| `POST` | `/api/v1/auth/reset-password` | — | Redefine a senha com o token do link |
| `GET` | `/api/v1/auth/csrf` | — | Devolve o token CSRF para o frontend |

Toda mutação (`POST`) exige, além do corpo JSON: header `Origin` permitido (ou
nenhum, para clientes não-navegador) e o par **cookie `applya_csrf` +
header `X-CSRF-Token`** (ver [Cookies e CSRF](#cookies-e-csrf)).

### Cadastro — `POST /api/v1/auth/register`

```json
{ "name": "Ana Santos", "email": "ana@example.com", "password": "SenhaForte#123" }
```

> Todos os valores acima são ilustrativos. Senha real: mínimo 8 caracteres,
> máximo 256.

Validações: nome obrigatório (máx. 100), e-mail válido e normalizado
(minúsculas) e senha de 8 a 256 caracteres.

`201 Created`:

```json
{
  "user": {
    "id": "7248a5d7-fb11-4074-847b-58076fe654fd",
    "name": "Ana Santos",
    "email": "ana@example.com",
    "email_verified": false
  },
  "verification_email_sent": true
}
```

E-mail duplicado responde `409` com `email_already_registered`. Se o envio do
e-mail de confirmação falhar, o cadastro **não** é desfeito (a conta e o token
foram criados atomicamente) e a resposta traz `verification_email_sent: false`;
o usuário pode pedir o reenvio.

### Confirmação de e-mail — `POST /api/v1/auth/verify-email`

O e-mail de confirmação contém o link
`{FRONTEND_URL}/verify-email?token=TOKEN` (válido por 24h). O frontend envia o
token à API:

```json
{ "token": "EXEMPLO-DE-TOKEN-DO-LINK" }
```

> Valor ilustrativo; o token real é uma string base64url de 43 caracteres.

`200 OK`:

```json
{ "message": "E-mail confirmado com sucesso." }
```

Tokens expirados respondem `400 expired_token`; já utilizados, `400
token_used`; desconhecidos, `400 invalid_token`. O consumo é atômico: dois
pedidos concorrentes com o mesmo token resultam em apenas uma confirmação.

### Login — `POST /api/v1/auth/login`

```json
{ "email": "ana@example.com", "password": "SenhaForte#123" }
```

`200 OK` — o usuário vem no corpo e a sessão no cookie `applya_session`:

```json
{
  "user": {
    "id": "7248a5d7-fb11-4074-847b-58076fe654fd",
    "name": "Ana Santos",
    "email": "ana@example.com",
    "email_verified": true
  }
}
```

- E-mail inexistente e senha errada produzem a **mesma** resposta `401
  invalid_credentials` (anti-enumeração); o tempo de resposta é equalizado com
  um hash artificial.
- Login antes da confirmação do e-mail responde `403 email_not_verified`.
- Mais de 10 tentativas por e-mail+IP em 15 minutos responde `429
  rate_limited` com `Retry-After`.

### Logout — `POST /api/v1/auth/logout`

Revoga a sessão no banco e remove o cookie. Responde `204 No Content`. É
idempotente (funciona mesmo sem sessão válida).

### Usuário autenticado — `GET /api/v1/auth/me`

Exige o cookie de sessão. `200 OK` com o mesmo formato `{ "user": {...} }`.
Sessões expiradas ou revogadas respondem `401 authentication_required`.

### Esqueci minha senha — `POST /api/v1/auth/forgot-password`

```json
{ "email": "ana@example.com" }
```

Sempre responde `202 Accepted` (exista ou não a conta):

```json
{ "message": "Se existir uma conta associada a este e-mail, enviaremos um link de recuperação." }
```

O link enviado é `{FRONTEND_URL}/reset-password?token=TOKEN`, válido por 30
minutos. Contas com e-mail não confirmado recebem o link de confirmação, não o
de recuperação. Limite: 5 solicitações por e-mail+IP a cada 15 minutos.

### Redefinição de senha — `POST /api/v1/auth/reset-password`

```json
{ "token": "TOKEN-DO-LINK-DE-REDEFINICAO", "new_password": "NovaSenha#456" }
```

`200 OK`:

```json
{ "message": "Senha redefinida com sucesso." }
```

Em uma única transação: o token é consumido, a senha trocada e **todas as
sessões do usuário são revogadas**. Não há login automático. O usuário recebe
um e-mail de notificação (sem a nova senha). Token reutilizado responde `400
token_used`.

### Reenvio de confirmação — `POST /api/v1/auth/resend-verification`

```json
{ "email": "ana@example.com" }
```

Resposta sempre `202 Accepted` (anti-enumeração). Só envia e-mail se a conta
existir e ainda não estiver verificada. Emitir um novo token invalida o
anterior. Limite: 3 reenvios por e-mail a cada hora.

---

## Cookies e CSRF

### Cookies

| Cookie | `HttpOnly` | Legível por JS | Conteúdo |
|---|---|---|---|
| `applya_session` | sim | não | Token de sessão (hash no banco) |
| `applya_csrf` | não | sim | Token CSRF do double-submit |

Atributos comuns: `Path=/api`, `Secure` em produção, `SameSite` conforme
`COOKIE_SAMESITE`, expiração da sessão conforme `SESSION_DURATION` (padrão 30
dias, duração fixa).

### Frontend e backend em domínios diferentes

A hospedagem planejada é **cross-site** (frontend Vue em um domínio, API no
Render em outro). Nesse cenário:

1. Defina `COOKIE_SAMESITE=none` — o atributo `Secure` é forçado
   automaticamente (navegadores ignoram `SameSite=None` sem `Secure`), e
   `APP_ENV=production` para ativar `Secure`.
2. O frontend envia credenciais com `credentials: "include"` (fetch/axios).
3. A proteção CSRF obrigatória é **dupla submissão + origem** (veja abaixo).

Limitações conhecidas de cookies cross-site:

- navegadores em modo de privacidade estrita podem bloquear cookies de
  terceiros, quebrando a sessão;
- a superfície CSRF é maior do que no mesmo site.

**Recomendação de longo prazo**: registrar um domínio próprio e servir ambos
sob o mesmo site (`app.applya.dev` + `api.applya.dev`, cookie `Domain` comum,
`COOKIE_SAMESITE=lax`), o que elimina a classificação de "terceiros" e
simplifica CSRF.

### Proteção CSRF (todas as mutações)

Duas camadas, para todos os `POST`/`PUT`/`PATCH`/`DELETE`:

1. **Origem**: se a requisição traz `Origin` (ou `Referer`), precisa casar com
   `CORS_ALLOWED_ORIGINS`. Requisições sem os dois headers (curl, SDKs) passam
   nessa camada — não são vetor de CSRF.
2. **Double-submit**: o cookie `applya_csrf` (gerado automaticamente na
   primeira resposta) deve ser devolvido no header `X-CSRF-Token`, com
   comparação em tempo constante.

Fluxo do frontend:

1. ao carregar, chame `GET /api/v1/auth/csrf` (ou leia o cookie já definido);
2. em toda mutação, envie `X-CSRF-Token` com o valor do cookie/token;
3. falhas respondem `403 csrf_failed`.

O CORS **não** é proteção contra CSRF — ele só controla a leitura de
respostas.

---

## Serviço de e-mail

E-mails transacionais (confirmação, recuperação, senha alterada) são enviados
pelo [Resend](https://resend.com), com templates em português brasileiro
centralizados em `internal/email`.

Configuração:

1. crie uma conta gratuita em [resend.com](https://resend.com) (3.000
   e-mails/mês, 100/dia);
2. verifique seu domínio de envio (ou use o domínio de teste `onboarding@resend.dev`
   para experimentar);
3. gere uma API key em *API Keys*;
4. configure no ambiente:

```text
EMAIL_PROVIDER=resend
EMAIL_API_KEY=re_sua_chave_aqui
EMAIL_FROM=Applya <no-reply@seu-dominio.dev>
```

Em desenvolvimento, `EMAIL_PROVIDER=log` não envia nada: o e-mail completo
(incluindo o link com o token, para você testar os fluxos) aparece no log da
API. **Nunca** use `log` em produção.

Nenhuma credencial vai ao código ou a arquivos versionados. O cliente HTTP do
Resend usa timeout de 10s e falhas de envio nunca expõem dados sensíveis ao
usuário (cadastro e reenvios têm caminhos de retry).

Como alternativa, SMTP com STARTTLS é suportado para provedores como Gmail.
Use uma senha de aplicativo, nunca a senha da conta:

```text
EMAIL_PROVIDER=smtp
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=seu-email@gmail.com
SMTP_PASSWORD=senha-de-app
EMAIL_FROM=Applya <seu-email@gmail.com>
```

O remetente SMTP exige TLS 1.2 ou superior e recusa servidores sem STARTTLS.

---

## Implantação no Render

O backend roda como *Web Service* no [Render](https://render.com), apontando
para o PostgreSQL do Neon.

1. **Neon**: crie o banco, copie a connection string e aplique as migrations
   (localmente, com o Goose, apontando `DATABASE_URL` para o Neon).
2. **Render → New → Web Service**, build command `go build -o app ./cmd/api`,
   start command `./app`.
3. Configure as variáveis de ambiente do serviço:

```text
APP_ENV=production
HTTP_PORT=8080
DATABASE_URL=postgres://...neon.tech/...?sslmode=require
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=https://seu-frontend.dev
FRONTEND_URL=https://seu-frontend.dev
SESSION_DURATION=720h
EMAIL_PROVIDER=resend
EMAIL_API_KEY=re_...
EMAIL_FROM=Applya <no-reply@seu-dominio.dev>
COOKIE_SAMESITE=none
TRUST_PROXY_HEADERS=true
```

- `DATABASE_URL` no Neon exige `sslmode=require`.
- `COOKIE_SAMESITE=none` é necessário porque frontend e API ficam em domínios
  diferentes (ver [Cookies e CSRF](#cookies-e-csrf)); `Secure` é ativado por
  `APP_ENV=production`.
- `TRUST_PROXY_HEADERS=true` é obrigatório no Render para que o rate limiting
  por IP enxergue o IP real do cliente; **nunca** ative em acesso direto (o
  header `X-Forwarded-For` é forjável).
- `EMAIL_PROVIDER=log` é **rejeitado em produção** pela validação de
  configuração (evita tokens de recuperação nos logs).
- A porta 8080 já é a padrão do Render.

---

## Tratamento de erros

As respostas de erro seguem sempre o mesmo formato:

```json
{
  "error": {
    "code": "not_found",
    "message": "recurso não encontrado",
    "request_id": "1-9f2c1a4b3d5e6f70"
  }
}
```

| Status | `code` | Situação |
|---|---|---|
| 400 | `validation_failed` | Entrada inválida (campos, formato, tamanhos) |
| 400 | `invalid_token` | Token de confirmação/redefinição inexistente ou malformado |
| 400 | `expired_token` | Token válido, porém expirado |
| 400 | `token_used` | Token já consumido anteriormente |
| 401 | `authentication_required` | Rota protegida sem sessão válida |
| 401 | `invalid_credentials` | E-mail ou senha incorretos (mensagem propositalmente genérica) |
| 403 | `email_not_verified` | Login com senha correta, mas e-mail não confirmado |
| 403 | `csrf_failed` | Falha na verificação CSRF (origem ou token) |
| 404 | `not_found` | Rota inexistente |
| 405 | `method_not_allowed` | Método não suportado para a rota |
| 409 | `email_already_registered` | E-mail já cadastrado |
| 429 | `rate_limited` | Limite de tentativas atingido (header `Retry-After`) |
| 500 | `internal_error` | Falha inesperada (detalhes ficam apenas no log) |

Erros internos nunca expõem stack trace, SQL, credenciais ou detalhes de
infraestrutura ao cliente. Use o `request_id` para localizar a requisição nos
logs.

---

## CORS

O CORS é configurado por `CORS_ALLOWED_ORIGINS`, com lista explícita de origens:

```text
CORS_ALLOWED_ORIGINS=http://localhost:5173,https://applya.example.com
```

- `*` é rejeitado na validação;
- credenciais são permitidas (`AllowCredentials`), usadas pelos cookies de
  sessão HTTP-only;
- headers expostos: `X-Request-Id`.

O CORS controla apenas a leitura de respostas cross-origin; a proteção contra
CSRF é separada (ver [Cookies e CSRF](#cookies-e-csrf)).

---

## Testes

```bash
# testes unitários (não exigem PostgreSQL nem serviços externos)
go test ./...

# vet e formatação
go vet ./...
gofmt -l .

# testes de integração (exigem DATABASE_URL e PostgreSQL descartável)
go test -tags=integration ./internal/auth/...
```Os testes unitários cobrem:

- `GET /health` (handler e roteador);
- carregamento, padrões e validação de configuração (incluindo as variáveis de
  autenticação e e-mail);
- respostas JSON para 404 e 405;
- CORS (origem permitida e negada);
- request ID, logging e recuperação de panic;
- timeouts do servidor;
- **autenticação**:
  - hash e verificação de senha (Argon2id), hashes adulterados e de outros
    algoritmos;
  - geração de tokens (formato, unicidade) e hash determinístico;
  - validação de nome, e-mail (normalização) e senha;
  - cookies de sessão e CSRF (atributos: `HttpOnly`, `Secure`, `SameSite`,
    `Path`, remoção);
  - middleware `RequireAuth` (401 sem sessão, usuário no contexto, remoção de
    cookie inválido);
  - CSRF (origem permitida/negada, `Referer`, `null`, double-submit);
  - rate limiter (limite, janelas, fail-open);
  - handlers com serviço fake: cadastro válido/inválido/duplicado, login
    correto/senha errada/sem confirmação, confirmação válida/expirada/
    reutilizada, reenvio, recuperação, redefinição, logout, `/me`, CSRF,
    429 com `Retry-After`, erros internos genéricos;
  - templates e envio de e-mail (links, escaping de HTML, Resend via
    `httptest`, timeout, erro HTTP).

Os testes de integração (`//go:build integration`, ignorados sem
`DATABASE_URL`) executam os fluxos completos contra um PostgreSQL **descartável**
(o esquema é criado pelo próprio teste; nunca rode contra dados reais):

- cadastro → confirmação → login → sessão;
- e-mail duplicado;
- token expirado e reutilizado;
- reenvio invalidando o token anterior;
- recuperação e redefinição (revogando sessões);
- logout e revogação;
- rate limiting de login e de redefinição;
- **confirmação concorrente**: 10 goroutines disputando o mesmo token —
  exatamente uma vence (consumo atômico);
- contador de rate limits persistido.

### Ferramentas de segurança

Além de `go vet` e `gofmt`, rode antes de publicar:

```bash
# análise estática aprofundada
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...

# vulnerabilidades conhecidas (biblioteca padrão e dependências)
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# segredos versionados (árvore atual + todo o histórico)
gitleaks git --log-opts=--all

# detector de corrida (exige CGO/compilador C)
go test -race ./...
```

O projeto fixa o toolchain mínimo seguro em `go.mod` (`toolchain go1.27.3`):
versões mais novas são baixadas automaticamente pelo Go quando necessário.

Veja também [`SECURITY.md`](SECURITY.md) para o processo de divulgação
responsável de vulnerabilidades.

---

## Migrations

As migrations ficam em `db/migrations` e são gerenciadas pelo Goose. Elas **não**
são executadas automaticamente pela API.

```bash
# criar uma migration
goose -dir db/migrations create create_profiles sql

# aplicar
goose -dir db/migrations postgres "$DATABASE_URL" up

# reverter a última
goose -dir db/migrations postgres "$DATABASE_URL" down

# status
goose -dir db/migrations postgres "$DATABASE_URL" status
```

No PowerShell:

```powershell
goose -dir db/migrations postgres $env:DATABASE_URL up
```

Ainda não existem tabelas de currículos; o esquema atual contém as tabelas de
autenticação (`users`, `sessions`, `auth_tokens`, `rate_limits`). Mais detalhes
em [`db/migrations/README.md`](db/migrations/README.md).

---

## Integração com o frontend Vue

O frontend Vue roda em outro servidor (dev: `http://localhost:5173`; produção:
domínio próprio ou Vercel/Netlify).

### Requisições com cookies

1. Configure o cliente HTTP (axios/fetch) com a base URL da API e
   `withCredentials: true` (axios) ou `credentials: "include"` (fetch) — sem
   isso o navegador não envia nem aceita o cookie de sessão.
2. Os cookies têm `Path=/api`: eles acompanham apenas as chamadas à API.
3. Nunca manipule o cookie de sessão no JavaScript — ele é HTTP-only.

```ts
// axios
const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL,
  withCredentials: true,
});
```

### CSRF

1. Obtenha o token: `GET /api/v1/auth/csrf` devolve `{ "csrf_token": "..." }`
   e define o cookie `applya_csrf`.
2. Em **todas** as mutações (`POST`/`PUT`/`PATCH`/`DELETE`), envie o header:

```ts
api.interceptors.request.use(async (config) => {
  if (["post", "put", "patch", "delete"].includes(config.method ?? "")) {
    const { data } = await api.get("/auth/csrf");
    config.headers["X-CSRF-Token"] = data.csrf_token;
  }
  return config;
});
```

3. Em produção cross-site, configure `COOKIE_SAMESITE=none` no backend (ver
   [Cookies e CSRF](#cookies-e-csrf)).

### Fluxos

- **Cadastro**: `POST /auth/register` → se `verification_email_sent` for
  `false`, ofereça "reenviar e-mail".
- **Confirmação**: a rota `/{FRONTEND_URL}/verify-email?token=...` envia o
  token para `POST /auth/verify-email`.
- **Login**: `POST /auth/login`; em seguida `GET /auth/me` confirma a sessão.
- **Esqueci a senha**: `POST /auth/forgot-password` (sempre responde 202) →
  rota `/reset-password?token=...` chama `POST /auth/reset-password`.
- **Erros**: trate por `error.code` (estável), nunca pelo texto da mensagem.

---

## Segurança

Boas práticas adotadas:

- nenhuma credencial em código ou em arquivos versionados;
- `.env` e variações locais ignorados pelo Git;
- `.env.example` contém apenas valores demonstrativos;
- `DATABASE_URL` nunca é registrada em log;
- logs não incluem cookies, tokens, senhas ou corpos de requisição (a exceção
  é o provedor de e-mail `log`, exclusivo de desenvolvimento);
- erros ao cliente são genéricos, com detalhes apenas no log do servidor;
- senhas com Argon2id (64 MiB, 3 iterações, 2 threads);
- tokens de sessão/confirmação/recuperação com `crypto/rand` (256 bits),
  armazenados apenas como hash SHA-256, de uso único e com expiração;
- sessões revogáveis (logout e redefinição de senha);
- respostas anti-enumeração de contas (login, recuperação e reenvio);
- rate limiting persistido no PostgreSQL (funciona com múltiplas instâncias);
- CSRF por double-submit + validação de origem em todas as mutações;
- cookies HTTP-only, `Secure` e `SameSite` configurado;
- queries parametrizadas (sem SQL injection);
- timeouts de leitura, escrita e header configurados;
- CORS com origens explícitas;
- panics recuperados e registrados, sem derrubar o processo;
- dependências mínimas e auditáveis.

Recomendações para o dia a dia:

- nunca coloque segredos em commits, prints, issues ou mensagens;
- use senhas diferentes entre desenvolvimento e qualquer outro ambiente;
- rode um scanner de segredos antes de publicar, por exemplo
  [Gitleaks](https://github.com/gitleaks/gitleaks):

  ```bash
  gitleaks detect --source . --verbose
  ```

- se uma credencial for commitada, **revogue/rotacione** a credencial: removê-la
  do `.gitignore` não a apaga do histórico do Git.

---

## Cuidados com `.gitignore` e credenciais

O `.gitignore` cobre:

- credenciais (`.env`, chaves privadas, certificados, arquivos de segredo);
- binários Go, executáveis, arquivos de teste, cobertura e perfis;
- configurações pessoais de IDEs;
- arquivos de sistema operacional;
- logs, caches e temporários;
- dumps, backups e dados locais do PostgreSQL.

Continuam versionados: `go.mod`, `go.sum`, `.env.example`, `README.md`,
`AGENTS.md`, código-fonte Go, testes e migrations `*.sql`.

Valide as regras com:

```bash
git check-ignore -v .env          # deve ser ignorado
git check-ignore -v .env.example  # não deve retornar nada
git check-ignore -v bin/app.exe   # deve ser ignorado
```

Nunca remova arquivos do histórico do Git sem autorização da equipe; em projeto
pessoal, avalie a necessidade antes de reescrever o histórico.

---

## Convenção de commits

O projeto segue o padrão [Conventional Commits](https://www.conventionalcommits.org/):

```text
chore: initialize Go backend
feat: add health check endpoint
test: add HTTP handler tests
docs: add setup instructions
fix: handle nil database pool
refactor: extract server timeouts
```

Mensagens curtas no imperativo, em inglês, com escopo opcional
(`feat(auth): ...`). Um commit por mudança lógica.

---

## Funcionalidades planejadas

Ainda **não implementadas** — servem como contexto para a evolução:

- **Perfil profissional**: informações pessoais, experiências, formação,
  habilidades, projetos e links.
- **Currículos**: criação, edição, duplicação, exclusão, personalização por vaga,
  seleção de informações do perfil e exportação em PDF.

Quando esses módulos existirem, cada um viverá em `internal/<módulo>` com seus
próprios handlers, consultas e testes, conforme descrito em `AGENTS.md`. A
autenticação (implementada) já fornece o usuário autenticado via
`GET /api/v1/auth/me` e o middleware `RequireAuth`.

---

## Documentação adicional

- [`AGENTS.md`](AGENTS.md) — orientações para sessões de agentes de IA.
- [`SECURITY.md`](SECURITY.md) — política de segurança e divulgação responsável.
- [`docs/decisions.md`](docs/decisions.md) — decisões técnicas e justificativas.
- [`db/migrations/README.md`](db/migrations/README.md) — comandos de migration.
