// Package config carrega e valida as configurações da aplicação a partir de
// variáveis de ambiente.
//
// A configuração é lida uma única vez na inicialização. Qualquer valor
// obrigatório ausente ou inválido resulta em erro, para que a API falhe cedo
// (fail fast) em vez de subir com uma configuração incorreta.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Ambientes suportados pela aplicação.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// Provedores de e-mail suportados.
const (
	EmailProviderLog    = "log"
	EmailProviderResend = "resend"
	EmailProviderSMTP   = "smtp"
)

// Valores padrão utilizados quando a variável de ambiente não é definida.
const (
	defaultHTTPPort       = 8080
	defaultLogLevel       = "info"
	defaultSessionDur     = 720 * time.Hour // 30 dias
	defaultEmailProvider  = EmailProviderLog
	defaultCookieSameSite = http.SameSiteLaxMode
)

// Config agrupa todas as configurações da aplicação.
type Config struct {
	// AppEnv identifica o ambiente de execução (development, test, production).
	AppEnv string
	// HTTPPort é a porta TCP em que o servidor HTTP escuta.
	HTTPPort int
	// DatabaseURL é a string de conexão do PostgreSQL.
	DatabaseURL string
	// LogLevel é o nível mínimo de log.
	LogLevel slog.Level
	// CORSAllowedOrigins lista as origens autorizadas a chamar a API.
	CORSAllowedOrigins []string

	// FrontendURL é a origem do frontend usada nos links de confirmação e
	// recuperação enviados por e-mail (ex.: https://applya.dev).
	FrontendURL string
	// SessionDuration é o tempo de vida das sessões de login.
	SessionDuration time.Duration
	// EmailProvider seleciona o provedor de e-mail (log, resend ou smtp).
	EmailProvider string
	// EmailAPIKey é a chave da API do Resend (apenas quando EmailProvider=resend).
	EmailAPIKey string
	// SMTPHost, SMTPPort, SMTPUsername e SMTPPassword configuram o provedor SMTP.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	// EmailFrom é o remetente dos e-mails transacionais.
	EmailFrom string
	// CookieSameSite define o atributo SameSite dos cookies de autenticação.
	CookieSameSite http.SameSite
	// TrustProxyHeaders indica que a API está atrás de um proxy confiável
	// (Render) e pode usar X-Forwarded-For para identificar o cliente.
	// Quando false, headers de encaminhamento são ignorados para evitar
	// spoofing de IP (GHSA-3fxj-6jh8-hvhx).
	TrustProxyHeaders bool
}

// IsProduction informa se a aplicação está rodando em produção.
func (c Config) IsProduction() bool {
	return c.AppEnv == EnvProduction
}

// Load lê as configurações a partir do ambiente do processo.
func Load() (Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom lê as configurações usando a função getenv informada.
//
// A injeção de getenv permite testar o carregamento sem alterar o ambiente
// global do processo.
func LoadFrom(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("config: getenv não pode ser nil")
	}

	var errs []error

	appEnv, err := parseAppEnv(getenv("APP_ENV"))
	if err != nil {
		errs = append(errs, err)
	}

	httpPort, err := parseHTTPPort(getenv("HTTP_PORT"))
	if err != nil {
		errs = append(errs, err)
	}

	databaseURL, err := parseDatabaseURL(getenv("DATABASE_URL"))
	if err != nil {
		errs = append(errs, err)
	}

	logLevel, err := parseLogLevel(getenv("LOG_LEVEL"))
	if err != nil {
		errs = append(errs, err)
	}

	origins, err := parseAllowedOrigins(getenv("CORS_ALLOWED_ORIGINS"))
	if err != nil {
		errs = append(errs, err)
	}

	frontendURL, err := parseFrontendURL(getenv("FRONTEND_URL"))
	if err != nil {
		errs = append(errs, err)
	}

	sessionDuration, err := parseSessionDuration(getenv("SESSION_DURATION"))
	if err != nil {
		errs = append(errs, err)
	}

	emailProvider, err := parseEmailProvider(getenv("EMAIL_PROVIDER"))
	if err != nil {
		errs = append(errs, err)
	}

	emailAPIKey := strings.TrimSpace(getenv("EMAIL_API_KEY"))
	emailFrom := strings.TrimSpace(getenv("EMAIL_FROM"))
	if emailProvider == EmailProviderResend {
		if emailAPIKey == "" {
			errs = append(errs, errors.New("EMAIL_API_KEY é obrigatória quando EMAIL_PROVIDER=resend"))
		}
		if emailFrom == "" {
			errs = append(errs, errors.New("EMAIL_FROM é obrigatório quando EMAIL_PROVIDER=resend"))
		}
	}
	smtpHost := strings.TrimSpace(getenv("SMTP_HOST"))
	smtpUsername := strings.TrimSpace(getenv("SMTP_USERNAME"))
	smtpPassword := strings.TrimSpace(getenv("SMTP_PASSWORD"))
	smtpPort, smtpPortErr := parseSMTPPort(getenv("SMTP_PORT"))
	if emailProvider == EmailProviderSMTP {
		if smtpHost == "" {
			errs = append(errs, errors.New("SMTP_HOST é obrigatório quando EMAIL_PROVIDER=smtp"))
		}
		if smtpPortErr != nil {
			errs = append(errs, smtpPortErr)
		} else if smtpPort == 0 {
			errs = append(errs, errors.New("SMTP_PORT é obrigatório quando EMAIL_PROVIDER=smtp"))
		}
		if smtpUsername == "" {
			errs = append(errs, errors.New("SMTP_USERNAME é obrigatório quando EMAIL_PROVIDER=smtp"))
		}
		if smtpPassword == "" {
			errs = append(errs, errors.New("SMTP_PASSWORD é obrigatório quando EMAIL_PROVIDER=smtp"))
		}
		if emailFrom == "" {
			errs = append(errs, errors.New("EMAIL_FROM é obrigatório quando EMAIL_PROVIDER=smtp"))
		}
	}
	if appEnv == EnvProduction && emailProvider == EmailProviderLog {
		// O provedor log registra o e-mail completo, incluindo links de
		// confirmação/recuperação com tokens. Em produção isso colocaria
		// tokens de tomada de conta nos logs.
		errs = append(errs, errors.New("EMAIL_PROVIDER=log não é permitido em produção; use resend"))
	}

	cookieSameSite, err := parseCookieSameSite(getenv("COOKIE_SAMESITE"))
	if err != nil {
		errs = append(errs, err)
	}

	trustProxyHeaders, err := parseTrustProxyHeaders(getenv("TRUST_PROXY_HEADERS"))
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("configuração inválida:\n%w", errors.Join(errs...))
	}

	return Config{
		AppEnv:             appEnv,
		HTTPPort:           httpPort,
		DatabaseURL:        databaseURL,
		LogLevel:           logLevel,
		CORSAllowedOrigins: origins,
		FrontendURL:        frontendURL,
		SessionDuration:    sessionDuration,
		EmailProvider:      emailProvider,
		EmailAPIKey:        emailAPIKey,
		SMTPHost:           smtpHost,
		SMTPPort:           smtpPort,
		SMTPUsername:       smtpUsername,
		SMTPPassword:       smtpPassword,
		EmailFrom:          emailFrom,
		CookieSameSite:     cookieSameSite,
		TrustProxyHeaders:  trustProxyHeaders,
	}, nil
}

func parseAppEnv(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return EnvDevelopment, nil
	}

	switch value {
	case EnvDevelopment, EnvTest, EnvProduction:
		return value, nil
	default:
		return "", fmt.Errorf("APP_ENV deve ser um de %q, %q ou %q (recebido %q)", EnvDevelopment, EnvTest, EnvProduction, raw)
	}
}

func parseHTTPPort(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultHTTPPort, nil
	}

	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("HTTP_PORT deve ser um número inteiro (recebido %q)", raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("HTTP_PORT deve estar entre 1 e 65535 (recebido %d)", port)
	}

	return port, nil
}

func parseDatabaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("DATABASE_URL é obrigatória (ex.: postgres://user:senha@localhost:5432/applya?sslmode=disable)")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("DATABASE_URL deve usar o esquema postgres:// ou postgresql:// (recebido %q)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("DATABASE_URL deve informar host e porta do PostgreSQL")
	}
	if strings.TrimPrefix(parsed.Path, "/") == "" {
		return "", errors.New("DATABASE_URL deve informar o nome do banco de dados")
	}

	return value, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		value = defaultLogLevel
	}

	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL deve ser debug, info, warn ou error (recebido %q)", raw)
	}
}

func parseAllowedOrigins(raw string) ([]string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, errors.New("CORS_ALLOWED_ORIGINS é obrigatória (ex.: http://localhost:5173)")
	}
	if value == "*" {
		return nil, errors.New("CORS_ALLOWED_ORIGINS não pode liberar todas as origens (\"*\"); informe as origens permitidas separadas por vírgula")
	}

	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))

	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return nil, errors.New("CORS_ALLOWED_ORIGINS não pode conter \"*\"; informe as origens permitidas explicitamente")
		}
		if err := validateOrigin(origin); err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}

	if len(origins) == 0 {
		return nil, errors.New("CORS_ALLOWED_ORIGINS deve conter ao menos uma origem válida")
	}

	return origins, nil
}

// validateOrigin garante que a origem tenha apenas esquema, host e porta
// opcional, sem path, query ou fragmento, conforme esperado pelo navegador.
func validateOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS contém uma origem inválida %q: %w", origin, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS contém uma origem inválida %q: use http:// ou https://", origin)
	}
	if parsed.Host == "" {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS contém uma origem inválida %q: host ausente", origin)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS contém uma origem inválida %q: remova o path", origin)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS contém uma origem inválida %q: remova query e fragmento", origin)
	}

	return nil
}

// parseFrontendURL valida a origem do frontend usada nos links de e-mail.
func parseFrontendURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("FRONTEND_URL é obrigatória (ex.: http://localhost:5173)")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("FRONTEND_URL inválida: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("FRONTEND_URL deve usar http:// ou https:// (recebido %q)", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("FRONTEND_URL deve informar o host do frontend (recebido %q)", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("FRONTEND_URL não deve conter query ou fragmento (recebido %q)", raw)
	}

	// Armazenada sem barra final: os links são montados com path próprio.
	return strings.TrimSuffix(value, "/"), nil
}

// parseSessionDuration valida a duração das sessões.
func parseSessionDuration(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultSessionDur, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("SESSION_DURATION deve ser uma duração válida (ex.: 720h, 24h) (recebido %q)", raw)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("SESSION_DURATION deve ser maior que zero (recebido %q)", raw)
	}

	return duration, nil
}

// parseEmailProvider valida o provedor de e-mail.
func parseEmailProvider(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return defaultEmailProvider, nil
	}

	switch value {
	case EmailProviderLog, EmailProviderResend, EmailProviderSMTP:
		return value, nil
	default:
		return "", fmt.Errorf("EMAIL_PROVIDER deve ser %q, %q ou %q (recebido %q)", EmailProviderLog, EmailProviderResend, EmailProviderSMTP, raw)
	}
}

func parseSMTPPort(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("SMTP_PORT deve estar entre 1 e 65535 (recebido %q)", raw)
	}
	return port, nil
}

// parseCookieSameSite valida o atributo SameSite dos cookies.
func parseCookieSameSite(raw string) (http.SameSite, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return defaultCookieSameSite, nil
	}

	switch value {
	case "lax":
		return http.SameSiteLaxMode, nil
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return 0, fmt.Errorf("COOKIE_SAMESITE deve ser lax, strict ou none (recebido %q)", raw)
	}
}

// parseTrustProxyHeaders valida a confiança em headers de proxy.
func parseTrustProxyHeaders(raw string) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return false, nil
	}

	trust, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("TRUST_PROXY_HEADERS deve ser true ou false (recebido %q)", raw)
	}

	return trust, nil
}
