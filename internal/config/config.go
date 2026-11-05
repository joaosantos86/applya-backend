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
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Ambientes suportados pela aplicação.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// Valores padrão utilizados quando a variável de ambiente não é definida.
const (
	defaultHTTPPort = 8080
	defaultLogLevel = "info"
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

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("configuração inválida:\n%w", errors.Join(errs...))
	}

	return Config{
		AppEnv:             appEnv,
		HTTPPort:           httpPort,
		DatabaseURL:        databaseURL,
		LogLevel:           logLevel,
		CORSAllowedOrigins: origins,
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
