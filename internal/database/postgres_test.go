package database

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewPoolRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := NewPool(context.Background(), "não é uma url de conexão", discardLogger())
	if err == nil {
		t.Fatal("NewPool() esperava erro, recebeu nil")
	}
	if !strings.Contains(err.Error(), "configuração do pool postgres inválida") {
		t.Errorf("erro = %q, esperado mencionar configuração inválida", err.Error())
	}
}
