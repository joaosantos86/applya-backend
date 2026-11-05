package config

import (
	"log/slog"
	"strings"
	"testing"
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
}

func TestLoadFromDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(env(map[string]string{
		"DATABASE_URL":         "postgresql://user:pass@localhost:5432/applya",
		"CORS_ALLOWED_ORIGINS": "http://localhost:5173",
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
