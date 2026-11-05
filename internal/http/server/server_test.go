package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestNewConfiguresServer(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Options{Handler: http.NotFoundHandler(), Port: 8080, Logger: logger})
	defer srv.Close()

	if srv.Addr != ":8080" {
		t.Errorf("Addr = %q, esperado %q", srv.Addr, ":8080")
	}
	if srv.ReadHeaderTimeout != readHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, esperado %v", srv.ReadHeaderTimeout, readHeaderTimeout)
	}
	if srv.ReadTimeout != readTimeout {
		t.Errorf("ReadTimeout = %v, esperado %v", srv.ReadTimeout, readTimeout)
	}
	if srv.WriteTimeout != writeTimeout {
		t.Errorf("WriteTimeout = %v, esperado %v", srv.WriteTimeout, writeTimeout)
	}
	if srv.IdleTimeout != idleTimeout {
		t.Errorf("IdleTimeout = %v, esperado %v", srv.IdleTimeout, idleTimeout)
	}
	if srv.ErrorLog == nil {
		t.Error("ErrorLog deveria ser configurado com slog")
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Options{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Port:   0, // porta efêmera: o teste não depende de uma porta fixa
		Logger: logger,
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, srv, logger) }()

	// Dá tempo para o ListenAndServe começar a aceitar conexões.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() erro inesperado no shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() não encerrou dentro do timeout do graceful shutdown")
	}
}

func TestTimeoutsAreBounded(t *testing.T) {
	t.Parallel()

	values := map[string]time.Duration{
		"readHeaderTimeout": readHeaderTimeout,
		"readTimeout":       readTimeout,
		"writeTimeout":      writeTimeout,
		"idleTimeout":       idleTimeout,
		"shutdownTimeout":   shutdownTimeout,
	}

	for name, value := range values {
		if value <= 0 {
			t.Errorf("%s deve ser positivo, recebido %v", name, value)
		}
		if value > time.Minute {
			t.Errorf("%s excessivamente longo para uma API: %v", name, value)
		}
	}
}
