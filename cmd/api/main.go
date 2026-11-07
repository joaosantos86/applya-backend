// Comando api é o ponto de entrada do backend do Applya.
//
// Responsabilidades: carregar configuração, montar logger, abrir o pool do
// PostgreSQL, construir os módulos (autenticação, e-mail), montar o roteador e
// executar o servidor HTTP até o encerramento gracioso. Lógica de negócio não
// pertence a este pacote.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/config"
	"github.com/joaosantos86/applya-backend/internal/database"
	"github.com/joaosantos86/applya-backend/internal/email"
	"github.com/joaosantos86/applya-backend/internal/http/handlers"
	"github.com/joaosantos86/applya-backend/internal/http/router"
	"github.com/joaosantos86/applya-backend/internal/http/server"
	"github.com/joaosantos86/applya-backend/internal/resume"
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
		slog.String("email_provider", cfg.EmailProvider),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	sender, err := newEmailSender(cfg, logger)
	if err != nil {
		return err
	}
	mailer := email.NewMailer(sender, cfg.EmailFrom, cfg.FrontendURL)

	store := auth.NewStore(pool)
	sessionManager := auth.NewSessionManager(store, cfg.SessionDuration, time.Now)

	service := auth.NewService(auth.ServiceOptions{
		Store:    store,
		Sessions: sessionManager,
		Mailer:   mailer,
		Hasher:   auth.DefaultPasswordHasher(),
		Logger:   logger,
	})

	cookieCfg := auth.CookieConfig{
		Secure:   cfg.IsProduction(),
		SameSite: cfg.CookieSameSite,
	}

	authHandler := handlers.NewAuthHandler(service, cookieCfg, cfg.SessionDuration, logger)
	csrfMiddleware := auth.NewCSRFMiddleware(cfg.CORSAllowedOrigins, cookieCfg)

	resumeService := resume.NewService(resume.NewStore(pool))
	resumeHandler := handlers.NewResumeHandler(resumeService, logger)

	handler := router.New(router.Options{
		Logger:            logger,
		AllowedOrigins:    cfg.CORSAllowedOrigins,
		TrustProxyHeaders: cfg.TrustProxyHeaders,
		Auth: router.AuthOptions{
			Handler:     authHandler,
			RequireAuth: auth.RequireAuth(sessionManager, cookieCfg, logger),
			CSRF:        csrfMiddleware.Handler,
		},
		Resumes: router.ResumeOptions{
			Handler: resumeHandler,
		},
	})

	srv := server.New(server.Options{
		Handler: handler,
		Port:    cfg.HTTPPort,
		Logger:  logger,
	})

	return server.Run(ctx, srv, logger)
}

// newEmailSender cria o Sender conforme EMAIL_PROVIDER.
func newEmailSender(cfg config.Config, logger *slog.Logger) (email.Sender, error) {
	switch cfg.EmailProvider {
	case config.EmailProviderResend:
		return email.NewResendSender(cfg.EmailAPIKey, nil), nil
	case config.EmailProviderSMTP:
		return email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword), nil
	case config.EmailProviderLog:
		return email.NewLogSender(logger), nil
	default:
		return nil, fmt.Errorf("provedor de e-mail desconhecido: %q", cfg.EmailProvider)
	}
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
