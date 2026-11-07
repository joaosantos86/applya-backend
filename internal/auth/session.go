package auth

import (
	"context"
	"fmt"
	"time"
)

// SessionManager cria e valida sessões de usuário.
//
// O token cru gerado em Create é entregue apenas ao navegador (cookie); o
// banco guarda somente o hash SHA-256. FindUser reconstrói o hash a partir do
// token vindo do cookie.
type SessionManager struct {
	store    *Store
	duration time.Duration
	now      func() time.Time
}

// NewSessionManager cria o gerenciador de sessões.
//
// duration é o tempo de vida de uma sessão a partir do login (duração fixa;
// sem extensão automática por atividade).
func NewSessionManager(store *Store, duration time.Duration, now func() time.Time) *SessionManager {
	return &SessionManager{store: store, duration: duration, now: now}
}

// Duration devolve a duração configurada das sessões (para o cookie Max-Age).
func (m *SessionManager) Duration() time.Duration { return m.duration }

// Create gera um token de sessão, persiste seu hash e devolve o token cru.
func (m *SessionManager) Create(ctx context.Context, userID string) (string, error) {
	token, hash, err := NewToken()
	if err != nil {
		return "", err
	}

	if err := m.store.CreateSession(ctx, userID, hash, m.now().Add(m.duration)); err != nil {
		return "", fmt.Errorf("criar sessão: %w", err)
	}

	return token, nil
}

// FindUser valida o token de sessão e devolve o usuário dono dela. Sessões
// inexistentes, expiradas ou revogadas devolvem ErrSessionNotFound.
func (m *SessionManager) FindUser(ctx context.Context, token string) (User, error) {
	user, err := m.store.FindUserBySessionToken(ctx, HashToken(token))
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// Revoke revoga a sessão identificada pelo token, se ainda estiver ativa.
func (m *SessionManager) Revoke(ctx context.Context, token string) error {
	return m.store.RevokeSession(ctx, HashToken(token))
}

// RevokeAll revoga todas as sessões ativas do usuário.
func (m *SessionManager) RevokeAll(ctx context.Context, userID string) error {
	return m.store.RevokeAllUserSessions(ctx, userID)
}
