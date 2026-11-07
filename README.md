# Applya API

Backend for Applya, a personal project for creating and tailoring resumes. The frontend is a separate Vue application. This repository currently provides the API foundation and authentication; resumes, profiles, and exports are not implemented yet.

## Stack

- Go 1.27
- `net/http` and Chi
- PostgreSQL with pgx
- Goose migrations
- Argon2id password hashing
- Cookie-based sessions

## Features

- health check;
- registration and email verification;
- login, logout, and current-user lookup;
- password reset;
- PostgreSQL-backed sessions and rate limiting;
- CSRF protection, CORS, and request logging;
- SMTP, Resend, or local log email delivery.

## Requirements

- Go 1.27 or newer;
- PostgreSQL 16 or newer;
- [Goose](https://github.com/pressly/goose) for migrations.

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

On Windows, add `%USERPROFILE%\go\bin` to `PATH` to call `goose` directly.

## Local setup

Copy the environment template:

```powershell
Copy-Item .env.example .env
```

Update `DATABASE_URL` if needed. The default local URL is:

```text
postgres://applya:applya@localhost:5432/applya?sslmode=disable
```

Load the local environment and apply migrations:

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

The default address is `http://localhost:8080`.

## Environment

The service reads environment variables at startup. `scripts/run.ps1` loads `.env` locally; hosting platforms should provide the same values through their environment-variable UI.

| Variable | Required | Description |
| --- | --- | --- |
| `APP_ENV` | No | `development`, `test`, or `production` |
| `HTTP_PORT` | No | HTTP port; defaults to `8080` |
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `LOG_LEVEL` | No | `debug`, `info`, `warn`, or `error` |
| `CORS_ALLOWED_ORIGINS` | Yes | Explicit, comma-separated frontend origins |
| `FRONTEND_URL` | Yes | Base URL used in email links |
| `SESSION_DURATION` | No | Session lifetime; defaults to `720h` |
| `EMAIL_PROVIDER` | No | `log`, `resend`, or `smtp` |
| `EMAIL_FROM` | SMTP/Resend | Sender address |
| `EMAIL_API_KEY` | Resend | Resend API key |
| `SMTP_HOST` | SMTP | SMTP hostname |
| `SMTP_PORT` | SMTP | STARTTLS port, usually `587` |
| `SMTP_USERNAME` | SMTP | SMTP username |
| `SMTP_PASSWORD` | SMTP | App password or provider credential |
| `COOKIE_SAMESITE` | No | `lax`, `strict`, or `none` |
| `TRUST_PROXY_HEADERS` | No | Enable only behind a trusted proxy |

Origins must be explicit. `CORS_ALLOWED_ORIGINS=*` is rejected.

### Email

`EMAIL_PROVIDER=log` is intended for local development. It writes the full email and verification link to the API log instead of sending a message.

SMTP requires STARTTLS and TLS 1.2 or newer. Example for Gmail:

```text
EMAIL_PROVIDER=smtp
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-address@gmail.com
SMTP_PASSWORD=your-16-character-app-password
EMAIL_FROM=Applya <your-address@gmail.com>
```

Use a Google app password, not the password used to sign in to Gmail.

For Resend:

```text
EMAIL_PROVIDER=resend
EMAIL_API_KEY=re_...
EMAIL_FROM=Applya <no-reply@your-domain.com>
```

## API

```text
GET /health
```

```json
{"status":"ok"}
```

### Authentication endpoints

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/v1/auth/register` | Creates an unverified account |
| `POST` | `/api/v1/auth/verify-email` | Verifies a one-time token |
| `POST` | `/api/v1/auth/resend-verification` | Sends a new verification link |
| `POST` | `/api/v1/auth/login` | Creates a session |
| `POST` | `/api/v1/auth/logout` | Invalidates the session |
| `GET` | `/api/v1/auth/me` | Returns the authenticated user |
| `GET` | `/api/v1/auth/csrf` | Returns and sets a CSRF token |
| `POST` | `/api/v1/auth/forgot-password` | Requests a reset link |
| `POST` | `/api/v1/auth/reset-password` | Resets a password with a one-time token |

`GET /api/v1/auth/me` returns `401 authentication_required` without a valid session. That is the normal logged-out state.

### Cookies and CSRF

The API uses an HTTP-only `applya_session` cookie. Every state-changing request needs CSRF protection:

1. Call `GET /api/v1/auth/csrf` with `credentials: 'include'`.
2. Keep the `applya_csrf` cookie.
3. Send its value as `X-CSRF-Token` on `POST`, `PUT`, `PATCH`, and `DELETE` requests.

The API also validates `Origin` or `Referer` when present. Invalid CSRF requests return `403 csrf_failed`.

```ts
await fetch("http://localhost:8080/api/v1/auth/register", {
  method: "POST",
  credentials: "include",
  headers: {
    "Content-Type": "application/json",
    "X-CSRF-Token": csrfToken,
  },
  body: JSON.stringify(payload),
})
```

### Error format

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Please check the submitted fields.",
    "request_id": "..."
  }
}
```

Common codes are `validation_failed`, `authentication_required`, `invalid_credentials`, `email_not_verified`, `csrf_failed`, `rate_limited`, and `internal_error`.

## CORS

The local frontend configuration is:

```text
CORS_ALLOWED_ORIGINS=http://localhost:5173
```

The CORS middleware allows credentials, API methods, `Content-Type`, `X-CSRF-Token`, and preflight `OPTIONS` requests. It runs before route-level authentication and CSRF checks.

## Migrations

Migrations live in `db/migrations` and are never run automatically by the API.

```bash
goose -dir db/migrations create create_profiles sql
goose -dir db/migrations postgres "$DATABASE_URL" up
goose -dir db/migrations postgres "$DATABASE_URL" status
goose -dir db/migrations postgres "$DATABASE_URL" down
```

The current authentication migration creates `users`, `sessions`, `auth_tokens`, and `rate_limits`.

## Checks

```bash
gofmt -l .
go vet ./...
staticcheck ./...
govulncheck ./...
go test ./...
go build ./cmd/api
gitleaks git --log-opts=--all
```

Integration tests require a disposable PostgreSQL database:

```bash
go test -tags=integration ./internal/auth
```

## Deployment

The API can run on Render with Neon PostgreSQL. Keep `.env` local and configure the deployment through the platform dashboard:

```text
APP_ENV=production
HTTP_PORT=10000
DATABASE_URL=postgres://...?sslmode=require
CORS_ALLOWED_ORIGINS=https://your-frontend.vercel.app
FRONTEND_URL=https://your-frontend.vercel.app
COOKIE_SAMESITE=none
TRUST_PROXY_HEADERS=true
```

For a frontend and API on different sites, `COOKIE_SAMESITE=none` is required. Production cookies are marked `Secure` automatically.

## Project structure

```text
cmd/api/                 application bootstrap
internal/auth/           sessions, tokens, CSRF, and rate limiting
internal/config/         environment validation
internal/database/       PostgreSQL pool lifecycle
internal/email/          templates and SMTP/Resend delivery
internal/http/           handlers, middleware, responses, and routes
db/migrations/           Goose migrations
docs/                    technical decisions
```

## Security

- Passwords use Argon2id.
- Session, verification, and reset tokens come from `crypto/rand` and are stored as hashes.
- Verification tokens expire after 24 hours; reset tokens expire after 30 minutes.
- Never commit `.env`, database URLs, SMTP credentials, or provider keys.

## License

Personal portfolio and learning project.
