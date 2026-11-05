//go:build integration

// Testes de integração do PostgreSQL.
//
// Execução:
//
//	go test -tags=integration ./internal/database/...
//
// Requer DATABASE_URL apontando para um PostgreSQL local disponível.
package database

import (
	"context"
	"os"
	"testing"
)

func TestNewPoolIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL não definida; teste de integração ignorado")
	}

	ctx := context.Background()

	pool, err := NewPool(ctx, databaseURL, discardLogger())
	if err != nil {
		t.Fatalf("NewPool() erro inesperado: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping() erro inesperado: %v", err)
	}
}
