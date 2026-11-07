package auth

import (
	"context"
	"log/slog"
	"time"
)

// Counter incrementa contadores de janela fixa e devolve o total acumulado na
// janela identificada por windowStart.
//
// A janela fixa (janelas alinhadas ao relógio, não deslizantes) permite que
// todas as instâncias da aplicação compartilhem os mesmos limites quando o
// contador é persistido — no caso do Applya, na tabela rate_limits do
// PostgreSQL. Ver docs/decisions.md.
type Counter interface {
	Increment(ctx context.Context, key string, windowStart time.Time) (int64, error)
}

// RateLimiter aplica um limite de requisições por janela fixa a uma chave.
type RateLimiter struct {
	counter Counter
	window  time.Duration
	limit   int64
	now     func() time.Time
	logger  *slog.Logger
}

// NewRateLimiter cria um limitador de janela fixa.
func NewRateLimiter(counter Counter, window time.Duration, limit int64, now func() time.Time, logger *slog.Logger) *RateLimiter {
	return &RateLimiter{counter: counter, window: window, limit: limit, now: now, logger: logger}
}

// Allow registra uma tentativa para a chave e devolve nil quando permitida, ou
// RateLimitError com o tempo de espera restante.
//
// Falhas do contador (ex.: banco indisponível) liberam a requisição
// (fail-open) para não derrubar a API inteira; o problema é registrado no log.
// A consequência é a perda temporária da proteção contra abuso — aceitável
// para este projeto, documentada em docs/decisions.md.
func (l *RateLimiter) Allow(ctx context.Context, key string) error {
	now := l.now()
	windowStart := now.Truncate(l.window)

	count, err := l.counter.Increment(ctx, key, windowStart)
	if err != nil {
		if l.logger != nil {
			l.logger.WarnContext(ctx, "rate limiter indisponível, liberando requisição",
				slog.String("key", key),
				slog.Any("error", err),
			)
		}
		return nil
	}

	if count <= l.limit {
		return nil
	}

	return RateLimitError{RetryAfter: windowStart.Add(l.window).Sub(now)}
}
