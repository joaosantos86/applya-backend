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

Este repositório contém a **fundação técnica** da API:

- servidor HTTP com timeouts e encerramento gracioso;
- roteamento com Chi;
- configuração e validação por variáveis de ambiente;
- logging estruturado com `log/slog`;
- pool de conexões PostgreSQL com `pgxpool`;
- estrutura de migrations com Goose;
- CORS configurável;
- tratamento padronizado de erros em JSON;
- healthcheck em `GET /health`;
- testes automatizados.

Autenticação, CRUDs, currículos e exportação em PDF **ainda não foram
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
| `log/slog` | Logging estruturado |
| `net/http` | Servidor HTTP |
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

A validação ocorre no startup e todos os problemas são reportados de uma vez.
Nenhuma credencial é exibida nas mensagens de erro.

### Carregar variáveis no desenvolvimento

Windows — PowerShell:

```powershell
$env:APP_ENV = "development"
$env:HTTP_PORT = "8080"
$env:DATABASE_URL = "postgres://applya:applya@localhost:5432/applya?sslmode=disable"
$env:LOG_LEVEL = "debug"
$env:CORS_ALLOWED_ORIGINS = "http://localhost:5173"
```

Windows — Prompt de Comando (`cmd.exe`):

```bat
set APP_ENV=development
set HTTP_PORT=8080
set DATABASE_URL=postgres://applya:applya@localhost:5432/applya?sslmode=disable
set LOG_LEVEL=debug
set CORS_ALLOWED_ORIGINS=http://localhost:5173
```

Linux/macOS — Bash/Zsh:

```bash
export APP_ENV=development
export HTTP_PORT=8080
export DATABASE_URL="postgres://applya:applya@localhost:5432/applya?sslmode=disable"
export LOG_LEVEL=debug
export CORS_ALLOWED_ORIGINS="http://localhost:5173"
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
│       └── main.go              # bootstrap: config, logger, pool, router, servidor
├── internal/
│   ├── config/
│   │   ├── config.go            # leitura e validação das variáveis de ambiente
│   │   └── config_test.go
│   ├── database/
│   │   ├── postgres.go          # criação e encerramento do pgxpool
│   │   ├── postgres_test.go
│   │   └── postgres_integration_test.go
│   └── http/
│       ├── handlers/
│       │   ├── health.go        # GET /health
│       │   └── health_test.go
│       ├── middleware/
│       │   ├── middleware.go    # request ID, logging, recoverer, CORS
│       │   └── middleware_test.go
│       ├── response/
│       │   └── response.go      # respostas JSON e erros padronizados
│       ├── router/
│       │   ├── router.go        # rotas e middlewares
│       │   └── router_test.go
│       └── server/
│           ├── server.go        # timeouts, ListenAndServe e graceful shutdown
│           └── server_test.go
├── db/
│   └── migrations/              # migrations SQL (Goose) + instruções
├── docs/
│   └── decisions.md             # decisões técnicas e justificativas
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
| 404 | `not_found` | Rota inexistente |
| 405 | `method_not_allowed` | Método não suportado para a rota |
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
- credenciais são permitidas (`AllowCredentials`), preparando a futura
  autenticação com cookies HTTP-only;
- headers expostos: `X-Request-Id`.

---

## Testes

```bash
# testes unitários (não exigem PostgreSQL)
go test ./...

# vet e formatação
go vet ./...
gofmt -l .

# testes de integração (exigem DATABASE_URL e PostgreSQL em execução)
go test -tags=integration ./...
```

Os testes cobrem:

- `GET /health` (handler e roteador);
- carregamento, padrões e validação de configuração;
- respostas JSON para 404 e 405;
- CORS (origem permitida e negada);
- request ID;
- logging de requisições;
- recuperação de panic sem exposição de detalhes;
- timeouts do servidor.

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

Ainda não existem tabelas ou migrations de negócio. Mais detalhes em
[`db/migrations/README.md`](db/migrations/README.md).

---

## Integração futura com o frontend

O frontend Vue rodará em outro servidor de desenvolvimento, normalmente em
`http://localhost:5173`. A configuração padrão de CORS já permite essa origem.

Fluxo esperado:

1. frontend envia requisições para `http://localhost:8080` (base URL
   configurável no frontend);
2. respostas são JSON;
3. futuramente, autenticação por cookie HTTP-only com `credentials: "include"`
   nas requisições do frontend;
4. erros seguem o formato padronizado e podem ser tratados por `code`.

Nenhuma funcionalidade de autenticação foi implementada nesta etapa.

---

## Segurança

Boas práticas adotadas:

- nenhuma credencial em código ou em arquivos versionados;
- `.env` e variações locais ignorados pelo Git;
- `.env.example` contém apenas valores demonstrativos;
- `DATABASE_URL` nunca é registrada em log;
- logs não incluem cookies, tokens, senhas ou corpos de requisição;
- erros ao cliente são genéricos, com detalhes apenas no log do servidor;
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

- **Autenticação**: cadastro, login, logout, gerenciamento de sessões e proteção
  de endpoints.
- **Perfil profissional**: informações pessoais, experiências, formação,
  habilidades, projetos e links.
- **Currículos**: criação, edição, duplicação, exclusão, personalização por vaga,
  seleção de informações do perfil e exportação em PDF.

Quando esses módulos existirem, cada um viverá em `internal/<módulo>` com seus
próprios handlers, consultas e testes, conforme descrito em `AGENTS.md`.

---

## Documentação adicional

- [`AGENTS.md`](AGENTS.md) — orientações para sessões de agentes de IA.
- [`docs/decisions.md`](docs/decisions.md) — decisões técnicas e justificativas.
- [`db/migrations/README.md`](db/migrations/README.md) — comandos de migration.
