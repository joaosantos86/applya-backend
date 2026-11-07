package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/joaosantos86/applya-backend/internal/email"
)

// Tempos de vida dos tokens de uso único.
//
// Fixos em código de propósito: são propriedades de segurança do produto, não
// de ambiente. Confirmação de e-mail vale 24h; recuperação de senha, 30min.
const (
	VerificationTokenTTL = 24 * time.Hour
	ResetTokenTTL        = 30 * time.Minute
)

// Janelas e limites de rate limiting por endpoint. Também fixos: documentados
// no README; ajustes futuros devem ser deliberados, não acidentais.
const (
	loginRateWindow    = 15 * time.Minute
	loginRateLimit     = 10 // tentativas por e-mail+IP
	forgotRateWindow   = 15 * time.Minute
	forgotRateLimit    = 5 // solicitações por e-mail+IP
	resendRateWindow   = time.Hour
	resendRateLimit    = 3 // reenvios por e-mail
	registerRateWindow = time.Hour
	registerRateLimit  = 10 // cadastros por IP
	verifyRateWindow   = 15 * time.Minute
	verifyRateLimit    = 30 // confirmações por IP
	resetRateWindow    = 15 * time.Minute
	resetRateLimit     = 10 // redefinições por IP
)

// ServiceOptions reúne as dependências do serviço de autenticação.
type ServiceOptions struct {
	Store    *Store
	Sessions *SessionManager
	Mailer   *email.Mailer
	Hasher   PasswordHasher

	// Now fornece o relógio; injetável para testes.
	Now func() time.Time

	// Logger registra eventos de negócio (nunca senhas, tokens ou links).
	Logger *slog.Logger

	// Rate limiters por endpoint. Se nil, são criados com os padrões acima,
	// usando o contador do PostgreSQL (Store).
	LoginLimiter              *RateLimiter
	ForgotPasswordLimiter     *RateLimiter
	ResendVerificationLimiter *RateLimiter
	RegisterLimiter           *RateLimiter
	VerifyEmailLimiter        *RateLimiter
	ResetPasswordLimiter      *RateLimiter
}

// Service implementa as regras de negócio da autenticação.
//
// Ele orquestra validação, persistência, hash de senha, sessões e e-mails. Os
// handlers HTTP apenas traduzem as entradas/saídas HTTP para este serviço.
type Service struct {
	store    *Store
	sessions *SessionManager
	mailer   *email.Mailer
	hasher   PasswordHasher
	now      func() time.Time
	logger   *slog.Logger

	loginLimiter              *RateLimiter
	forgotPasswordLimiter     *RateLimiter
	resendVerificationLimiter *RateLimiter
	registerLimiter           *RateLimiter
	verifyEmailLimiter        *RateLimiter
	resetPasswordLimiter      *RateLimiter
}

// NewService cria o serviço preenchendo os padrões dos campos opcionais.
func NewService(opts ServiceOptions) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	hasher := opts.Hasher
	if hasher == (PasswordHasher{}) {
		hasher = DefaultPasswordHasher()
	}

	return &Service{
		store:    opts.Store,
		sessions: opts.Sessions,
		mailer:   opts.Mailer,
		hasher:   hasher,
		now:      now,
		logger:   logger,

		loginLimiter:              defaultLimiter(opts.LoginLimiter, opts.Store, loginRateWindow, loginRateLimit, now, logger),
		forgotPasswordLimiter:     defaultLimiter(opts.ForgotPasswordLimiter, opts.Store, forgotRateWindow, forgotRateLimit, now, logger),
		resendVerificationLimiter: defaultLimiter(opts.ResendVerificationLimiter, opts.Store, resendRateWindow, resendRateLimit, now, logger),
		registerLimiter:           defaultLimiter(opts.RegisterLimiter, opts.Store, registerRateWindow, registerRateLimit, now, logger),
		verifyEmailLimiter:        defaultLimiter(opts.VerifyEmailLimiter, opts.Store, verifyRateWindow, verifyRateLimit, now, logger),
		resetPasswordLimiter:      defaultLimiter(opts.ResetPasswordLimiter, opts.Store, resetRateWindow, resetRateLimit, now, logger),
	}
}

// defaultLimiter usa o limitador informado ou cria um sobre o contador do
// PostgreSQL (Store implementa Counter).
func defaultLimiter(limiter *RateLimiter, store *Store, window time.Duration, limit int64, now func() time.Time, logger *slog.Logger) *RateLimiter {
	if limiter != nil {
		return limiter
	}
	return NewRateLimiter(store, window, limit, now, logger)
}

// ---------------------------------------------------------------------------
// Cadastro
// ---------------------------------------------------------------------------

// Register cria a conta, o token de confirmação e dispara o e-mail de
// verificação.
//
// Devolve a projeção pública do usuário e se o e-mail de confirmação foi
// enviado. Falhas no envio NÃO são erro: a conta já existe (criada
// atomicamente com o token) e o usuário pode pedir o reenvio.
func (s *Service) Register(ctx context.Context, name, emailInput, password, clientIP string) (PublicUser, bool, error) {
	normalized, err := ValidateRegisterInput(name, emailInput, password)
	if err != nil {
		return PublicUser{}, false, err
	}

	if err := s.registerLimiter.Allow(ctx, "register:"+clientIP); err != nil {
		return PublicUser{}, false, err
	}

	_, err = s.store.FindUserByEmail(ctx, normalized)
	if err == nil {
		// O e-mail já existe. O hash abaixo só iguala o tempo de resposta
		// entre "e-mail novo" e "e-mail duplicado".
		s.hasher.VerifyAgainstDummy(password)
		return PublicUser{}, false, ErrEmailTaken
	}
	if err != ErrUserNotFound {
		return PublicUser{}, false, fmt.Errorf("verificar e-mail existente: %w", err)
	}

	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return PublicUser{}, false, err
	}

	token, tokenHash, err := NewToken()
	if err != nil {
		return PublicUser{}, false, err
	}

	user, err := s.store.CreateUserWithVerificationToken(ctx,
		name, normalized, passwordHash, tokenHash, s.now().Add(VerificationTokenTTL))
	if err != nil {
		return PublicUser{}, false, err
	}

	if err := s.mailer.SendVerification(ctx, user.Email, user.Name, token); err != nil {
		// O cadastro foi concluído; só o e-mail falhou. O log tem os detalhes
		// e o usuário pode reenviar. Nenhum dado sensível vai ao cliente.
		s.logger.WarnContext(ctx, "falha ao enviar e-mail de confirmação",
			slog.String("user_id", user.ID),
			slog.Any("error", err),
		)
		return user.Public(), false, nil
	}

	s.logger.InfoContext(ctx, "usuário cadastrado e e-mail de confirmação enviado",
		slog.String("user_id", user.ID),
	)

	return user.Public(), true, nil
}

// ---------------------------------------------------------------------------
// Confirmação de e-mail
// ---------------------------------------------------------------------------

// VerifyEmail consome o token e marca o e-mail do usuário como verificado.
func (s *Service) VerifyEmail(ctx context.Context, token, clientIP string) error {
	if err := ValidateTokenFormat(token); err != nil {
		return ErrInvalidToken
	}

	if err := s.verifyEmailLimiter.Allow(ctx, "verify:"+clientIP); err != nil {
		return err
	}

	err := s.store.ConsumeEmailVerificationToken(ctx, HashToken(token), s.now())
	if err != nil {
		return err
	}

	return nil
}

// ResendVerification reenvia o link de confirmação.
//
// A resposta é sempre genérica (não retorna erro para e-mail desconhecido ou
// já verificado) para não permitir enumeração de contas. O rate limit por
// e-mail impede abuso de caixas de entrada alheias.
func (s *Service) ResendVerification(ctx context.Context, emailInput string) error {
	normalized, err := ValidateEmailInput(emailInput)
	if err != nil {
		return err
	}

	if err := s.resendVerificationLimiter.Allow(ctx, "resend:"+normalized); err != nil {
		return err
	}

	user, err := s.store.FindUserByEmail(ctx, normalized)
	if err == ErrUserNotFound || user.EmailVerified() {
		// Conta inexistente ou já verificada: nada a fazer, mesma resposta.
		return nil
	}
	if err != nil {
		return fmt.Errorf("buscar usuário para reenvio: %w", err)
	}

	token, tokenHash, err := NewToken()
	if err != nil {
		return err
	}

	if err := s.store.CreateEmailVerificationToken(ctx, user.ID, tokenHash, s.now().Add(VerificationTokenTTL)); err != nil {
		return fmt.Errorf("criar token de confirmação: %w", err)
	}

	if err := s.mailer.SendVerification(ctx, user.Email, user.Name, token); err != nil {
		s.logger.WarnContext(ctx, "falha ao reenviar e-mail de confirmação",
			slog.String("user_id", user.ID),
			slog.Any("error", err),
		)
		// Mesma resposta genérica: o usuário pode tentar de novo.
		return nil
	}

	s.logger.InfoContext(ctx, "e-mail de confirmação reenviado", slog.String("user_id", user.ID))

	return nil
}

// ---------------------------------------------------------------------------
// Login e logout
// ---------------------------------------------------------------------------

// Login valida credenciais, exige e-mail confirmado e cria uma sessão.
//
// Devolve a projeção pública do usuário e o token de sessão cru, que o handler
// coloca no cookie HTTP-only. E-mails inexistentes e senhas erradas produzem a
// MESMA resposta (ErrInvalidCredentials), com trabalho de hash artificial no
// caso de e-mail inexistente para não vazar tempo de resposta.
func (s *Service) Login(ctx context.Context, emailInput, password, clientIP string) (PublicUser, string, error) {
	normalized, err := ValidateLoginInput(emailInput, password)
	if err != nil {
		return PublicUser{}, "", err
	}

	if err := s.loginLimiter.Allow(ctx, "login:"+normalized+":"+clientIP); err != nil {
		return PublicUser{}, "", err
	}

	user, err := s.store.FindUserByEmail(ctx, normalized)
	if err == ErrUserNotFound {
		s.hasher.VerifyAgainstDummy(password)
		return PublicUser{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return PublicUser{}, "", fmt.Errorf("buscar usuário para login: %w", err)
	}

	ok, err := s.hasher.Verify(password, user.PasswordHash)
	if err != nil {
		return PublicUser{}, "", fmt.Errorf("verificar senha: %w", err)
	}
	if !ok {
		return PublicUser{}, "", ErrInvalidCredentials
	}

	if !user.EmailVerified() {
		return PublicUser{}, "", ErrEmailNotVerified
	}

	token, err := s.sessions.Create(ctx, user.ID)
	if err != nil {
		return PublicUser{}, "", fmt.Errorf("criar sessão: %w", err)
	}

	s.logger.InfoContext(ctx, "login realizado", slog.String("user_id", user.ID))

	return user.Public(), token, nil
}

// Logout revoga a sessão atual. Revogar uma sessão inexistente (ou já
// revogada) não é erro: o logout é idempotente.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}

	if err := s.sessions.Revoke(ctx, sessionToken); err != nil {
		return fmt.Errorf("revogar sessão: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Recuperação e redefinição de senha
// ---------------------------------------------------------------------------

// ForgotPassword inicia a recuperação de senha.
//
// A resposta é sempre a mesma, existindo ou não a conta (anti-enumeração).
// Contas com e-mail não confirmado recebem o link de confirmação, não o de
// recuperação. Um hash artificial iguala o tempo de resposta quando a conta
// não existe.
func (s *Service) ForgotPassword(ctx context.Context, emailInput, clientIP string) error {
	normalized, err := ValidateEmailInput(emailInput)
	if err != nil {
		return err
	}

	if err := s.forgotPasswordLimiter.Allow(ctx, "forgot:"+normalized+":"+clientIP); err != nil {
		return err
	}

	user, err := s.store.FindUserByEmail(ctx, normalized)
	if err == ErrUserNotFound {
		s.hasher.VerifyAgainstDummy("applya-dummy-password")
		return nil
	}
	if err != nil {
		return fmt.Errorf("buscar usuário para recuperação: %w", err)
	}

	if !user.EmailVerified() {
		// Conta não verificada: manda o link de confirmação em vez do reset.
		return s.resendVerificationEmail(ctx, user)
	}

	token, tokenHash, err := NewToken()
	if err != nil {
		return err
	}

	if err := s.store.CreatePasswordResetToken(ctx, user.ID, tokenHash, s.now().Add(ResetTokenTTL)); err != nil {
		return fmt.Errorf("criar token de recuperação: %w", err)
	}

	if err := s.mailer.SendPasswordReset(ctx, user.Email, user.Name, token); err != nil {
		s.logger.WarnContext(ctx, "falha ao enviar e-mail de recuperação de senha",
			slog.String("user_id", user.ID),
			slog.Any("error", err),
		)
		// Resposta genérica; o usuário pode solicitar novamente.
		return nil
	}

	s.logger.InfoContext(ctx, "e-mail de recuperação de senha enviado", slog.String("user_id", user.ID))

	return nil
}

// resendVerificationEmail centraliza o reenvio usado por ResendVerification e
// ForgotPassword (para contas não confirmadas).
func (s *Service) resendVerificationEmail(ctx context.Context, user User) error {
	token, tokenHash, err := NewToken()
	if err != nil {
		return err
	}

	if err := s.store.CreateEmailVerificationToken(ctx, user.ID, tokenHash, s.now().Add(VerificationTokenTTL)); err != nil {
		return fmt.Errorf("criar token de confirmação: %w", err)
	}

	if err := s.mailer.SendVerification(ctx, user.Email, user.Name, token); err != nil {
		s.logger.WarnContext(ctx, "falha ao enviar e-mail de confirmação",
			slog.String("user_id", user.ID),
			slog.Any("error", err),
		)
		return nil
	}

	return nil
}

// ResetPassword redefine a senha a partir de um token de recuperação.
//
// Em uma única transação o token é consumido, a senha trocada e todas as
// sessões do usuário são revogadas. Não há login automático. Após o sucesso,
// um e-mail de notificação é enviado (sem a nova senha).
func (s *Service) ResetPassword(ctx context.Context, token, newPassword, clientIP string) error {
	if err := ValidateTokenFormat(token); err != nil {
		return ErrInvalidToken
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	// O limite é verificado antes do hash Argon2 (operação cara) para
	// conter tentativas abusivas.
	if err := s.resetPasswordLimiter.Allow(ctx, "reset:"+clientIP); err != nil {
		return err
	}

	passwordHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}

	user, err := s.store.ConsumePasswordResetToken(ctx, HashToken(token), passwordHash, s.now())
	if err != nil {
		return err
	}

	// Notificação de senha alterada: melhor esforço, sem a senha e sem links.
	if err := s.mailer.SendPasswordChanged(ctx, user.Email, user.Name); err != nil {
		s.logger.WarnContext(ctx, "falha ao enviar notificação de senha alterada",
			slog.String("user_id", user.ID),
			slog.Any("error", err),
		)
		return nil
	}

	s.logger.InfoContext(ctx, "senha redefinida", slog.String("user_id", user.ID))

	return nil
}
