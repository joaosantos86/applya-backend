// Comando api é o ponto de entrada do backend do Applya.
//
// Responsabilidades: carregar configuração, montar logger, abrir o pool do
// PostgreSQL, montar o roteador e executar o servidor HTTP até o encerramento
// gracioso. Lógica de negócio não pertence a este pacote.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joaosantos86/applya-backend/internal/config"
	"github.com/joaosantos86/applya-backend/internal/database"
	"github.com/joaosantos86/applya-backend/internal/http/router"
	"github.com/joaosantos86/applya-backend/internal/http/server"
)

func main() {
	if err := run(); err != nil {
		// O logger ainda não existe quando a configuração falha.
		fmt.Fprintf(os.Stderr, "falha ao iniciar a api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregar configuração: %w", err)
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	logger.Info("iniciando applya api",
		slog.String("env", cfg.AppEnv),
		slog.Int("http_port", cfg.HTTPPort),
		slog.String("log_level", cfg.LogLevel.String()),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	handler := router.New(router.Options{
		Logger:         logger,
		AllowedOrigins: cfg.CORSAllowedOrigins,
	})

	srv := server.New(server.Options{
		Handler: handler,
		Port:    cfg.HTTPPort,
		Logger:  logger,
	})

	return server.Run(ctx, srv, logger)
}

// newLogger cria o logger estruturado (log/slog) usado pela aplicação.
//
// Em desenvolvimento o formato é texto legível; em produção, JSON para
// facilitar a coleta por agregadores de log.
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}

	var handler slog.Handler
	if cfg.IsProduction() {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler).With(slog.String("service", "applya-api"))
}
