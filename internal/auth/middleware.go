package auth

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// SessionFinder valida um token de sessão e devolve o usuário dono dela.
//
// É o limite que o middleware de autenticação conhece do resto do módulo:
// SessionManager o implementa, e os testes podem usar um fake sem PostgreSQL.
type SessionFinder interface {
	FindUser(ctx context.Context, sessionToken string) (User, error)
}

// RequireAuth protege rotas autenticadas.
//
// Fluxo: lê o cookie de sessão, valida a sessão no banco (existência,
// expiração e revogação) e coloca o usuário no contexto da requisição. Em
// qualquer falha responde 401 com código authentication_required e remove o
// cookie inválido do navegador.
func RequireAuth(finder SessionFinder, cookieCfg CookieConfig, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := SessionTokenFromRequest(r)
			if !ok {
				writeUnauthorized(w, r, "autenticação necessária")
				return
			}

			user, err := finder.FindUser(r.Context(), token)
			if err != nil {
				if logger != nil {
					logger.WarnContext(r.Context(), "sessão inválida rejeitada",
						slog.String("request_id", middleware.GetReqID(r.Context())),
						slog.Any("error", err),
					)
				}
				// Cookie inválido é removido para evitar repetição de erros.
				ClearSessionCookie(w, cookieCfg)
				writeUnauthorized(w, r, "sessão inválida ou expirada")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
		})
	}
}

// writeUnauthorized responde 401 com o código estável de autenticação.
func writeUnauthorized(w http.ResponseWriter, r *http.Request, message string) {
	_ = response.Error(w, http.StatusUnauthorized, response.CodeAuthenticationRequired,
		message, middleware.GetReqID(r.Context()))
}

// ---------------------------------------------------------------------------
// Contexto
// ---------------------------------------------------------------------------

type userContextKey struct{}

// WithUser coloca o usuário autenticado no contexto da requisição.
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// UserFromContext recupera o usuário autenticado colocado por RequireAuth.
// Handlers protegidos podem confiar no usuário; rotas não protegidas recebem
// ok=false.
func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

// ---------------------------------------------------------------------------
// CSRF
// ---------------------------------------------------------------------------

// csrfHeader é o header em que o frontend devolve o token CSRF.
const csrfHeader = "X-CSRF-Token"

// CSRFMiddleware protege requisições que mudam estado contra CSRF.
//
// Duas camadas (nenhuma delas é o CORS, que não protege contra CSRF):
//
//  1. Validação de origem: se a requisição traz Origin (ou Referer), a origem
//     precisa estar na lista CORS_ALLOWED_ORIGINS. Navegadores enviam Origin
//     em todas as requisições cross-origin e nas POSTs; requisições sem
//     nenhum dos headers vêm de clientes não-navegador (curl, SDKs), que não
//     estão sujeitos a CSRF e passam.
//
//  2. Double-submit: o frontend obtém o token do cookie applya_csrf (legível
//     por JS) e o devolve no header X-CSRF-Token; o middleware compara os dois
//     em tempo constante. Um site atacante não consegue ler o cookie da
//     vítima (origem diferente), portanto não consegue forjar o header.
//
// O cookie applya_csrf é criado automaticamente na primeira resposta (em
// qualquer método), então basta o frontend chamar GET /api/v1/auth/csrf antes
// da primeira mutação, ou ler o cookie já presente.
type CSRFMiddleware struct {
	allowedOrigins map[string]struct{}
	cookieCfg      CookieConfig
}

// NewCSRFMiddleware cria o middleware a partir das origens permitidas.
func NewCSRFMiddleware(allowedOrigins []string, cookieCfg CookieConfig) *CSRFMiddleware {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[origin] = struct{}{}
	}

	return &CSRFMiddleware{allowedOrigins: origins, cookieCfg: cookieCfg}
}

// Handler é o middleware em si.
func (m *CSRFMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Garante que o navegador sempre tenha o cookie do token CSRF.
		m.ensureCookie(w, r)

		if isUnsafeMethod(r.Method) {
			if !m.originAllowed(r) || !m.tokenMatches(r) {
				_ = response.Error(w, http.StatusForbidden, response.CodeCSRFFailed,
					"falha na verificação CSRF", middleware.GetReqID(r.Context()))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isUnsafeMethod identifica métodos que mudam estado (protegidos por CSRF).
func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// ensureCookie define o cookie CSRF quando ausente, garantindo que toda
// resposta forneça o token para o frontend.
func (m *CSRFMiddleware) ensureCookie(w http.ResponseWriter, r *http.Request) {
	if _, ok := CSRFTokenFromRequest(r); ok {
		return
	}

	token, _, err := NewToken()
	if err != nil {
		return
	}

	SetCSRFCookie(w, token, m.cookieCfg)
}

// originAllowed valida a origem da requisição.
func (m *CSRFMiddleware) originAllowed(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		_, ok := m.allowedOrigins[origin]
		return ok
	}

	if referer := r.Header.Get("Referer"); referer != "" {
		return m.refererAllowed(referer)
	}

	// Sem Origin e sem Referer: cliente não-navegador. Cookies não são
	// enviados automaticamente por esses clientes, logo não há CSRF.
	return true
}

// refererAllowed verifica se a origem do Referer está na lista permitida.
func (m *CSRFMiddleware) refererAllowed(referer string) bool {
	parsed, err := url.Parse(referer)
	if err != nil {
		return false
	}

	origin := parsed.Scheme + "://" + parsed.Host
	_, ok := m.allowedOrigins[origin]
	return ok
}

// tokenMatches compara o header X-CSRF-Token com o cookie applya_csrf em
// tempo constante (após normalizar ambos com SHA-256 para comprimento fixo).
func (m *CSRFMiddleware) tokenMatches(r *http.Request) bool {
	cookieToken, ok := CSRFTokenFromRequest(r)
	if !ok {
		return false
	}

	headerToken := r.Header.Get(csrfHeader)
	if headerToken == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(HashToken(cookieToken)), []byte(HashToken(headerToken))) == 1
}
