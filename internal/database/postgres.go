// Package database gerencia a conexão com o PostgreSQL.
//
// A API usa pgxpool, o pool de conexões do pgx. Nenhuma query de negócio vive
// aqui: este pacote apenas cria, valida e encerra o pool.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// connectTimeout limita a tentativa inicial de conexão.
	connectTimeout = 5 * time.Second

	maxConns        = 10
	minConns        = 0
	maxConnLifetime = time.Hour
	maxConnIdleTime = 30 * time.Minute
	healthCheck     = time.Minute
)

// NewPool cria o pool de conexões, verifica se o PostgreSQL está acessível e
// devolve o pool pronto para uso.
//
// Se o banco estiver indisponível, a função falha: a API não inicia sem banco,
// pois todas as funcionalidades futuras dependem dele. Não há reconexão
// automática complexa; o próximo start verifica a conexão novamente.
//
// A DATABASE_URL nunca é registrada em log.
func NewPool(ctx context.Context, databaseURL string, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configuração do pool postgres inválida: %w", err)
	}

	poolConfig.MaxConns = maxConns
	poolConfig.MinConns = minConns
	poolConfig.MaxConnLifetime = maxConnLifetime
	poolConfig.MaxConnIdleTime = maxConnIdleTime
	poolConfig.HealthCheckPeriod = healthCheck

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("criar pool de conexões postgres: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectar ao postgres: %w", err)
	}

	logger.Info("conexão com postgres estabelecida",
		slog.String("host", poolConfig.ConnConfig.Host),
		slog.String("database", poolConfig.ConnConfig.Database),
	)

	return pool, nil
}
