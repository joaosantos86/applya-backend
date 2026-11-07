package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memoryCounter é um Counter em memória para testes do rate limiter.
type memoryCounter struct {
	mu   sync.Mutex
	vals map[string]int64
}

func newMemoryCounter() *memoryCounter {
	return &memoryCounter{vals: make(map[string]int64)}
}

func (c *memoryCounter) Increment(_ context.Context, key string, windowStart time.Time) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := key + "|" + windowStart.UTC().Format(time.RFC3339Nano)
	c.vals[k]++
	return c.vals[k], nil
}

func TestRateLimiterAllowsUntilLimit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := NewRateLimiter(newMemoryCounter(), time.Minute, 3, func() time.Time { return now }, discardLogger())
	ctx := context.Background()

	for range 3 {
		if err := limiter.Allow(ctx, "chave"); err != nil {
			t.Fatalf("tentativa dentro do limite rejeitada: %v", err)
		}
	}

	err := limiter.Allow(ctx, "chave")
	if err == nil {
		t.Fatal("quarta tentativa deveria ser rejeitada")
	}

	rateErr, ok := err.(RateLimitError)
	if !ok {
		t.Fatalf("erro = %T, esperado RateLimitError", err)
	}
	if rateErr.RetryAfter <= 0 || rateErr.RetryAfter > time.Minute {
		t.Errorf("RetryAfter = %v, esperado entre 0 e 1m", rateErr.RetryAfter)
	}
}

func TestRateLimiterWindowResets(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := NewRateLimiter(newMemoryCounter(), time.Minute, 1, func() time.Time { return now }, discardLogger())
	ctx := context.Background()

	if err := limiter.Allow(ctx, "chave"); err != nil {
		t.Fatalf("primeira tentativa rejeitada: %v", err)
	}
	if err := limiter.Allow(ctx, "chave"); err == nil {
		t.Fatal("segunda tentativa na mesma janela deveria ser rejeitada")
	}

	// Nova janela: o contador zera.
	now = now.Add(time.Minute)
	if err := limiter.Allow(ctx, "chave"); err != nil {
		t.Fatalf("tentativa na janela seguinte rejeitada: %v", err)
	}
}

func TestRateLimiterFailOpen(t *testing.T) {
	t.Parallel()

	failing := failingCounter{}
	limiter := NewRateLimiter(failing, time.Minute, 1, time.Now, discardLogger())

	// Falha do contador libera a requisição (disponibilidade em primeiro
	// lugar); o problema é registrado, não propagado.
	if err := limiter.Allow(context.Background(), "chave"); err != nil {
		t.Fatalf("fail-open esperado, recebeu erro: %v", err)
	}
}

type failingCounter struct{}

func (failingCounter) Increment(context.Context, string, time.Time) (int64, error) {
	return 0, errors.New("contador indisponível")
}
