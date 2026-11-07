package config

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func validEnv() map[string]string {
	return map[string]string{
		"APP_ENV":              "development",
		"HTTP_PORT":            "9000",
		"DATABASE_URL":         "postgres://applya:applya@localhost:5432/applya?sslmode=disable",
		"LOG_LEVEL":            "debug",
		"CORS_ALLOWED_ORIGINS": "http://localhost:5173, https://applya.example.com",
		"FRONTEND_URL":         "https://applya.example.com",
		"SESSION_DURATION":     "48h",
		"EMAIL_PROVIDER":       "resend",
		"EMAIL_API_KEY":        "re_chave_demonstrativa",
		"EMAIL_FROM":           "Applya <no-reply@applya.example.com>",
		"COOKIE_SAMESITE":      "none",
	}
}

func TestLoadFromValid(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(env(validEnv()))
	if err != nil {
		t.Fatalf("LoadFrom() erro inesperado: %v", err)
	}

	if cfg.AppEnv != EnvDevelopment {
		t.Errorf("AppEnv = %q, esperado %q", cfg.AppEnv, EnvDevelopment)
	}
	if cfg.HTTPPort != 9000 {
		t.Errorf("HTTPPort = %d, esperado 9000", cfg.HTTPPort)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, esperado %v", cfg.LogLevel, slog.LevelDebug)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORSAllowedOrigins = %v, esperado 2 origens", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[1] != "https://applya.example.com" {
		t.Errorf("origem[1] = %q, esperado %q", cfg.CORSAllowedOrigins[1], "https://applya.example.com")
	}
	if cfg.FrontendURL != "https://applya.example.com" {
		t.Errorf("FrontendURL = %q", cfg.FrontendURL)
	}
	if cfg.SessionDuration != 48*time.Hour {
		t.Errorf("SessionDuration = %v, esperado 48h", cfg.SessionDuration)
	}
	if cfg.EmailProvider != EmailProviderResend {
		t.Errorf("EmailProvider = %q, esperado %q", cfg.EmailProvider, EmailProviderResend)
	}
	if cfg.EmailAPIKey != "re_chave_demonstrativa" {
		t.Errorf("EmailAPIKey = %q", cfg.EmailAPIKey)
	}
	if cfg.EmailFrom != "Applya <no-reply@applya.example.com>" {
		t.Errorf("EmailFrom = %q", cfg.EmailFrom)
	}
	if cfg.CookieSameSite != http.SameSiteNoneMode {
		t.Errorf("CookieSameSite = %v, esperado SameSite=None", cfg.CookieSameSite)
	}
}

func TestLoadFromDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(env(map[string]string{
		"DATABASE_URL":         "postgresql://user:pass@localhost:5432/applya",
		"CORS_ALLOWED_ORIGINS": "http://localhost:5173",
		"FRONTEND_URL":         "http://localhost:5173",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() erro inesperado: %v", err)
	}

	if cfg.AppEnv != EnvDevelopment {
		t.Errorf("AppEnv padrão = %q, esperado %q", cfg.AppEnv, EnvDevelopment)
	}
	if cfg.HTTPPort != defaultHTTPPort {
		t.Errorf("HTTPPort padrão = %d, esperado %d", cfg.HTTPPort, defaultHTTPPort)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel padrão = %v, esperado %v", cfg.LogLevel, slog.LevelInfo)
	}
	if cfg.SessionDuration != defaultSessionDur {
		t.Errorf("SessionDuration padrão = %v, esperado %v", cfg.SessionDuration, defaultSessionDur)
	}
	if cfg.EmailProvider != EmailProviderLog {
		t.Errorf("EmailProvider padrão = %q, esperado %q", cfg.EmailProvider, EmailProviderLog)
	}
	if cfg.CookieSameSite != http.SameSiteLaxMode {
		t.Errorf("CookieSameSite padrão = %v, esperado lax", cfg.CookieSameSite)
	}
}

func TestLoadFromSMTP(t *testing.T) {
	t.Parallel()

	values := validEnv()
	values["EMAIL_PROVIDER"] = "smtp"
	values["SMTP_HOST"] = "smtp.gmail.com"
	values["SMTP_PORT"] = "587"
	values["SMTP_USERNAME"] = "applya@example.com"
	values["SMTP_PASSWORD"] = "senha-de-app"

	cfg, err := LoadFrom(env(values))
	if err != nil {
		t.Fatalf("LoadFrom() erro inesperado: %v", err)
	}
	if cfg.EmailProvider != EmailProviderSMTP || cfg.SMTPHost != "smtp.gmail.com" || cfg.SMTPPort != 587 {
		t.Errorf("configuração SMTP inesperada: provider=%q host=%q port=%d", cfg.EmailProvider, cfg.SMTPHost, cfg.SMTPPort)
	}
}

func TestLoadFromInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		override  map[string]string
		wantInMsg []string
	}{
		{
			name:      "DATABASE_URL obrigatória",
			override:  map[string]string{"DATABASE_URL": ""},
			wantInMsg: []string{"DATABASE_URL é obrigatória"},
		},
		{
			name:      "DATABASE_URL com esquema inválido",
			override:  map[string]string{"DATABASE_URL": "mysql://user:pass@localhost:3306/applya"},
			wantInMsg: []string{"DATABASE_URL deve usar o esquema"},
		},
		{
			name:      "DATABASE_URL sem banco",
			override:  map[string]string{"DATABASE_URL": "postgres://user:pass@localhost:5432"},
			wantInMsg: []string{"nome do banco"},
		},
		{
			name:      "porta não numérica",
			override:  map[string]string{"HTTP_PORT": "abc"},
			wantInMsg: []string{"HTTP_PORT deve ser um número inteiro"},
		},
		{
			name:      "porta fora do intervalo",
			override:  map[string]string{"HTTP_PORT": "70000"},
			wantInMsg: []string{"HTTP_PORT deve estar entre"},
		},
		{
			name:      "ambiente desconhecido",
			override:  map[string]string{"APP_ENV": "staging"},
			wantInMsg: []string{"APP_ENV deve ser um de"},
		},
		{
			name:      "nível de log desconhecido",
			override:  map[string]string{"LOG_LEVEL": "trace"},
			wantInMsg: []string{"LOG_LEVEL deve ser debug, info, warn ou error"},
		},
		{
			name:      "CORS ausente",
			override:  map[string]string{"CORS_ALLOWED_ORIGINS": ""},
			wantInMsg: []string{"CORS_ALLOWED_ORIGINS é obrigatória"},
		},
		{
			name:      "CORS com curinga",
			override:  map[string]string{"CORS_ALLOWED_ORIGINS": "*"},
			wantInMsg: []string{"não pode liberar todas as origens"},
		},
		{
			name:      "CORS com curinga na lista",
			override:  map[string]string{"CORS_ALLOWED_ORIGINS": "http://localhost:5173,*"},
			wantInMsg: []string{"não pode conter"},
		},
		{
			name:      "CORS com path",
			override:  map[string]string{"CORS_ALLOWED_ORIGINS": "http://localhost:5173/api"},
			wantInMsg: []string{"remova o path"},
		},
		{
			name:      "CORS sem esquema",
			override:  map[string]string{"CORS_ALLOWED_ORIGINS": "localhost:5173"},
			wantInMsg: []string{"use http:// ou https://"},
		},
		{
			name:      "FRONTEND_URL obrigatória",
			override:  map[string]string{"FRONTEND_URL": ""},
			wantInMsg: []string{"FRONTEND_URL é obrigatória"},
		},
		{
			name:      "FRONTEND_URL com esquema inválido",
			override:  map[string]string{"FRONTEND_URL": "ftp://applya.example.com"},
			wantInMsg: []string{"FRONTEND_URL deve usar http:// ou https://"},
		},
		{
			name:      "FRONTEND_URL com query",
			override:  map[string]string{"FRONTEND_URL": "https://applya.example.com?x=1"},
			wantInMsg: []string{"FRONTEND_URL não deve conter query"},
		},
		{
			name:      "SESSION_DURATION inválida",
			override:  map[string]string{"SESSION_DURATION": "amanhã"},
			wantInMsg: []string{"SESSION_DURATION deve ser uma duração válida"},
		},
		{
			name:      "SESSION_DURATION zero",
			override:  map[string]string{"SESSION_DURATION": "0h"},
			wantInMsg: []string{"SESSION_DURATION deve ser maior que zero"},
		},
		{
			name:      "EMAIL_PROVIDER desconhecido",
			override:  map[string]string{"EMAIL_PROVIDER": "gmail"},
			wantInMsg: []string{"EMAIL_PROVIDER deve ser"},
		},
		{
			name:      "EMAIL_API_KEY obrigatória no resend",
			override:  map[string]string{"EMAIL_API_KEY": ""},
			wantInMsg: []string{"EMAIL_API_KEY é obrigatória"},
		},
		{
			name:      "EMAIL_FROM obrigatório no resend",
			override:  map[string]string{"EMAIL_FROM": ""},
			wantInMsg: []string{"EMAIL_FROM é obrigatório"},
		},
		{
			name: "SMTP exige configuração completa",
			override: map[string]string{
				"EMAIL_PROVIDER": "smtp", "SMTP_HOST": "", "SMTP_PORT": "", "SMTP_USERNAME": "", "SMTP_PASSWORD": "", "EMAIL_FROM": "",
			},
			wantInMsg: []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "EMAIL_FROM"},
		},
		{
			name:      "COOKIE_SAMESITE desconhecido",
			override:  map[string]string{"COOKIE_SAMESITE": "everything"},
			wantInMsg: []string{"COOKIE_SAMESITE deve ser lax, strict ou none"},
		},
		{
			name:      "EMAIL_PROVIDER log proibido em produção",
			override:  map[string]string{"APP_ENV": "production", "EMAIL_PROVIDER": "log"},
			wantInMsg: []string{"EMAIL_PROVIDER=log não é permitido em produção"},
		},
		{
			name:      "TRUST_PROXY_HEADERS inválido",
			override:  map[string]string{"TRUST_PROXY_HEADERS": "talvez"},
			wantInMsg: []string{"TRUST_PROXY_HEADERS deve ser true ou false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			values := validEnv()
			for key, value := range tt.override {
				values[key] = value
			}

			_, err := LoadFrom(env(values))
			if err == nil {
				t.Fatal("LoadFrom() esperava erro, recebeu nil")
			}

			for _, want := range tt.wantInMsg {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("erro = %q, esperado conter %q", err.Error(), want)
				}
			}
		})
	}
}

func TestLoadFromReportsAllErrors(t *testing.T) {
	t.Parallel()

	_, err := LoadFrom(env(map[string]string{
		"APP_ENV":              "invalid",
		"HTTP_PORT":            "-1",
		"DATABASE_URL":         "",
		"LOG_LEVEL":            "verbose",
		"CORS_ALLOWED_ORIGINS": "",
	}))
	if err == nil {
		t.Fatal("LoadFrom() esperava erro, recebeu nil")
	}

	for _, want := range []string{"APP_ENV", "HTTP_PORT", "DATABASE_URL", "LOG_LEVEL", "CORS_ALLOWED_ORIGINS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("erro agregado deveria citar %q; recebido: %v", want, err)
		}
	}
}

func TestLoadFromNilGetenv(t *testing.T) {
	t.Parallel()

	if _, err := LoadFrom(nil); err == nil {
		t.Fatal("LoadFrom(nil) esperava erro, recebeu nil")
	}
}

func TestIsProduction(t *testing.T) {
	t.Parallel()

	if (Config{AppEnv: EnvProduction}).IsProduction() != true {
		t.Error("Config{AppEnv: production}.IsProduction() = false, esperado true")
	}
	if (Config{AppEnv: EnvDevelopment}).IsProduction() != false {
		t.Error("Config{AppEnv: development}.IsProduction() = true, esperado false")
	}
}

func TestLoadFromDoesNotLeakCredentials(t *testing.T) {
	t.Parallel()

	values := validEnv()
	values["DATABASE_URL"] = "mysql://applya:super-secret@localhost:3306/applya"

	_, err := LoadFrom(env(values))
	if err == nil {
		t.Fatal("LoadFrom() esperava erro, recebeu nil")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Errorf("mensagem de erro não deve conter a senha do banco: %v", err)
	}
}

func TestLoadFromTrustProxyHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
	}{
		{"", false}, // padrão: não confiar (evita spoofing de IP)
		{"false", false},
		{"true", true},
		{"TRUE", true},
	}

	for _, tt := range tests {
		values := validEnv()
		values["TRUST_PROXY_HEADERS"] = tt.raw

		cfg, err := LoadFrom(env(values))
		if err != nil {
			t.Errorf("LoadFrom() com TRUST_PROXY_HEADERS=%q erro inesperado: %v", tt.raw, err)
			continue
		}
		if cfg.TrustProxyHeaders != tt.want {
			t.Errorf("TrustProxyHeaders = %v, esperado %v (raw %q)", cfg.TrustProxyHeaders, tt.want, tt.raw)
		}
	}
}
