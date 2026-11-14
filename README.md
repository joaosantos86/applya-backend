# Applya backend

Applya is a personal project for building resumes tailored to job applications.
This repository contains the Go REST API; the frontend lives in
`applya-frontend`.

The API currently covers registration, email verification, sign-in, password
recovery, session management, and a persistent resume workspace. The workspace
has one base resume per user, independent resume versions, optimistic
concurrency, and A4 PDF export. Profiles, templates, public sharing, AI, and
payments are not implemented.

## Stack

Go 1.27, `net/http`, Chi, PostgreSQL with pgx, Goose migrations, Argon2id,
HTTP-only session cookies, structured `log/slog` logging, and
`go-pdf/fpdf` with embedded fonts.

## Run locally

Use Go 1.27+ and PostgreSQL 16+. Install Goose if it is not already available:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Copy the environment template and adjust `DATABASE_URL` if needed:

```powershell
Copy-Item .env.example .env
```

The default local database URL is:

```text
postgres://applya:applya@localhost:5432/applya?sslmode=disable
```

Load the environment and apply migrations:

```powershell
Get-Content .env | Where-Object { $_ -match "^\s*[^#].*=" } | ForEach-Object {
  $name, $value = $_ -split "=", 2
  Set-Item -Path "Env:$($name.Trim())" -Value $value.Trim()
}

goose -dir db/migrations postgres $env:DATABASE_URL up
```

Start the API:

```powershell
.\scripts\run.ps1
```

The API listens at `http://localhost:8080` by default. To check that it is
running:

```bash
curl http://localhost:8080/health
```

The expected response is `{"status":"ok"}`.

## Environment

The API reads configuration once during startup. `.env` is loaded by
`scripts/run.ps1`; hosted environments must provide these values through their
environment configuration.

| Variable | Required | Purpose |
| --- | --- | --- |
| `APP_ENV` | No | `development`, `test`, or `production` |
| `HTTP_PORT` | No | HTTP port; defaults to `8080` |
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `LOG_LEVEL` | No | `debug`, `info`, `warn`, or `error` |
| `CORS_ALLOWED_ORIGINS` | Yes | Explicit, comma-separated frontend origins |
| `FRONTEND_URL` | Yes | Base URL used in email links |
| `SESSION_DURATION` | No | Session lifetime; defaults to `720h` |
| `EMAIL_PROVIDER` | No | `log`, `resend`, or `smtp` |
| `EMAIL_FROM` | Resend/SMTP | Transactional email sender |
| `EMAIL_API_KEY` | Resend | Resend API key |
| `SMTP_HOST` | SMTP | SMTP host |
| `SMTP_PORT` | SMTP | STARTTLS port, usually `587` |
| `SMTP_USERNAME` | SMTP | SMTP username |
| `SMTP_PASSWORD` | SMTP | SMTP password or app password |
| `COOKIE_SAMESITE` | No | `lax`, `strict`, or `none` |
| `TRUST_PROXY_HEADERS` | No | Enable only behind a trusted proxy |

Origins must be explicit. `CORS_ALLOWED_ORIGINS=*` is rejected.

`EMAIL_PROVIDER=log` is intended for local development. It writes the complete
email and link to the API log instead of sending it. It is rejected in
production. Resend and SMTP require the corresponding credentials; SMTP uses
STARTTLS and TLS 1.2 or newer.

## API

### Authentication

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/v1/auth/register` | Create an unverified account |
| `POST` | `/api/v1/auth/verify-email` | Verify an email with a one-time token |
| `POST` | `/api/v1/auth/resend-verification` | Send another verification link |
| `POST` | `/api/v1/auth/login` | Create a session |
| `POST` | `/api/v1/auth/logout` | Invalidate the current session |
| `GET` | `/api/v1/auth/me` | Return the authenticated user |
| `GET` | `/api/v1/auth/csrf` | Return and set a CSRF token |
| `POST` | `/api/v1/auth/forgot-password` | Request a password reset link |
| `POST` | `/api/v1/auth/reset-password` | Set a new password with a one-time token |

`GET /api/v1/auth/me` returns `401 authentication_required` when no valid
session exists. This is the normal signed-out state.

### Resumes

All resume routes require an authenticated session. Ownership comes from the
session; the API never accepts `user_id` from the client.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/resumes/base` | Get or create the base resume |
| `PATCH` | `/api/v1/resumes/base` | Update the base resume |
| `GET` | `/api/v1/resumes/versions` | List the user's versions |
| `POST` | `/api/v1/resumes/versions` | Create a version from the base |
| `GET` | `/api/v1/resumes/versions/{id}` | Get a complete version |
| `PATCH` | `/api/v1/resumes/versions/{id}` | Update a version |
| `DELETE` | `/api/v1/resumes/versions/{id}` | Delete a version |
| `POST` | `/api/v1/resumes/export/pdf` | Generate and return an A4 PDF |
| `GET` | `/api/v1/workspace` | Load the user, base, and versions |

PATCH requests are partial by section: omitted sections are preserved, sent
sections replace the entire section, and `[]` clears a collection. A `revision`
value provides optimistic concurrency and stale updates return `409 conflict`.
Versions are independent copies: changing the base does not change existing
versions, and changing a version does not change the base.

### PDF export

`POST /api/v1/resumes/export/pdf` accepts the complete resume in the request
body and returns the PDF directly. The document is not persisted. The accepted
sections are `personal`, `summary`, `experiences`, `education`, `skills`,
`projects`, and `languages`.

```bash
curl -X POST http://localhost:8080/api/v1/resumes/export/pdf \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  --cookie "applya_session=$SESSION; applya_csrf=$CSRF_TOKEN" \
  --data-binary @resume.json \
  --output resume.pdf
```

The export validates field and collection limits, URLs, dates, and language
proficiency. URLs must use HTTP or HTTPS; dates use `YYYY-MM`. Text is rendered
as plain text, never as client-provided HTML or CSS. The API limits exports to
10 per minute per user and returns `429 rate_limited` with `Retry-After` when
the limit is exceeded. The full contract is in
[`docs/openapi.yaml`](docs/openapi.yaml), with frontend integration notes in
[`docs/frontend-integration-pdf.md`](docs/frontend-integration-pdf.md).

## Sessions and CSRF

Sessions are stored in PostgreSQL and sent in the HTTP-only `applya_session`
cookie. The frontend must use `credentials: 'include'` for API requests.

Every state-changing request (`POST`, `PUT`, `PATCH`, and `DELETE`) requires a
double-submit CSRF token:

1. Call `GET /api/v1/auth/csrf` with credentials included.
2. Keep the `applya_csrf` cookie.
3. Send its value in the `X-CSRF-Token` header.

The API also validates `Origin` or `Referer` when present. Invalid requests
return `403 csrf_failed`. CORS allows credentials, the API methods,
`Content-Type`, `X-CSRF-Token`, and preflight `OPTIONS` requests.

## Errors

Errors use one JSON shape:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Check the submitted fields.",
    "request_id": "..."
  }
}
```

Common codes include `validation_failed`, `authentication_required`,
`invalid_credentials`, `email_not_verified`, `csrf_failed`, `rate_limited`,
`not_found`, `conflict`, `payload_too_large`, `invalid_resume`,
`pdf_generation_failed`, and `internal_error`.

## Migrations

Migrations live in `db/migrations` and are never run automatically by the API.

```bash
goose -dir db/migrations postgres "$DATABASE_URL" up
goose -dir db/migrations postgres "$DATABASE_URL" status
goose -dir db/migrations postgres "$DATABASE_URL" down
```

The current migrations create the authentication tables (`users`, `sessions`,
`auth_tokens`, and `rate_limits`) and the resume table with the base/version
constraints.

## Commands and checks

```bash
gofmt -l .
go vet ./...
staticcheck ./...
govulncheck ./...
go test ./...
go build ./cmd/api
gitleaks git --log-opts=--all
```

Unit tests run without PostgreSQL. Integration tests require a disposable
database and are marked with the `integration` build tag:

```bash
go test -tags=integration ./internal/auth
go test -tags=integration ./internal/resume
```

## Source layout

```text
cmd/api/                 Application bootstrap
internal/auth/           Sessions, tokens, CSRF, and rate limiting
internal/config/         Environment validation
internal/database/       PostgreSQL pool lifecycle
internal/email/          Email templates and providers
internal/http/           Handlers, middleware, responses, and routes
internal/metrics/         In-memory PDF export counters
internal/resume/         Resume domain, validation, persistence, and PDF
docs/                    Technical decisions and OpenAPI contract
db/migrations/            Goose migrations
```

## Deployment

The API can run on a Go host with managed PostgreSQL. Configure the production
environment through the hosting provider rather than committing `.env`:

```text
APP_ENV=production
HTTP_PORT=10000
DATABASE_URL=postgres://...?sslmode=require
CORS_ALLOWED_ORIGINS=https://your-frontend.example.com
FRONTEND_URL=https://your-frontend.example.com
EMAIL_PROVIDER=resend
COOKIE_SAMESITE=none
TRUST_PROXY_HEADERS=true
```

For frontend and API on different sites, `COOKIE_SAMESITE=none` is required and
production cookies receive `Secure` automatically. Validate browser flows,
CORS, HTTPS, cookies, and response headers before publishing.

## Security

Passwords use Argon2id. Session, confirmation, and recovery tokens are generated
with `crypto/rand` and only their hashes are stored. Confirmation tokens expire
after 24 hours and recovery tokens after 30 minutes. Resume queries are always
filtered by the authenticated user. Never commit `.env`, database URLs, email
credentials, provider keys, or dumps.

## License

Personal portfolio and learning project.
