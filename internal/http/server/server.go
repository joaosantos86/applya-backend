// Package server cria e executa o servidor HTTP com timeouts explícitos e
// encerramento gracioso.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Timeouts do servidor. São constantes com propósito: o projeto usa valores
// conservadores e não precisa (ainda) configurá-los por ambiente.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second

	// shutdownTimeout é o tempo máximo de espera para que as requisições em
	// andamento terminem durante o encerramento.
	shutdownTimeout = 10 * time.Second
)

// Options reúne as dependências necessárias para criar o servidor.
type Options struct {
	Handler http.Handler
	Port    int
	Logger  *slog.Logger
}

// New cria o servidor HTTP com timeouts e logger configurados.
func New(opts Options) *http.Server {
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           opts.Handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	if opts.Logger != nil {
		// Redireciona os logs internos do net/http para o slog estruturado.
		srv.ErrorLog = slog.NewLogLogger(opts.Logger.Handler(), slog.LevelError)
	}

	return srv
}

// Run inicia o servidor e bloqueia até que ctx seja cancelado (por exemplo, por
// SIGINT/SIGTERM) ou até que o servidor falhe.
//
// No encerramento, novas conexões são recusadas e as requisições em andamento
// têm até shutdownTimeout para terminar.
func Run(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	errCh := make(chan error, 1)

	go func() {
		logger.Info("servidor http escutando", slog.String("addr", srv.Addr))

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("servidor http: %w", err)
			return
		}

		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("sinal de encerramento recebido, iniciando graceful shutdown")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("encerrar servidor http: %w", err)
	}

	logger.Info("servidor http encerrado")
	return nil
}
