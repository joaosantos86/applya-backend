// Package router monta o roteador HTTP e registra as rotas da API.
//
// Toda nova rota deve ser registrada aqui ou em um submódulo que exponha uma
// função de registro. O roteador não deve conter lógica de negócio.
package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/joaosantos86/applya-backend/internal/http/handlers"
	"github.com/joaosantos86/applya-backend/internal/http/middleware"
	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// Options reúne as dependências necessárias para montar o roteador.
type Options struct {
	Logger         *slog.Logger
	AllowedOrigins []string

	// TrustProxyHeaders indica que a API está atrás de um proxy confiável
	// (Render) e pode usar X-Forwarded-For para identificar o cliente.
	TrustProxyHeaders bool

	// Auth reúne as rotas e middlewares de autenticação. Quando Handler é
	// nil, as rotas /api/v1/auth não são registradas.
	Auth AuthOptions

	// Resumes reúne o handler de currículos. As rotas só são registradas
	// quando o handler e os middlewares de autenticação/CSRF estão presentes:
	// nenhuma rota de currículo existe sem proteção de sessão.
	Resumes ResumeOptions
}

// ResumeOptions descreve o que o roteador precisa do módulo de currículos.
type ResumeOptions struct {
	Handler *handlers.ResumeHandler
}

// AuthOptions descreve o que o roteador precisa do módulo de autenticação.
type AuthOptions struct {
	Handler     *handlers.AuthHandler
	RequireAuth func(http.Handler) http.Handler
	CSRF        func(http.Handler) http.Handler
}

// New cria o roteador com todas as rotas e middlewares da API.
func New(opts Options) http.Handler {
	r := chi.NewRouter()

	// A ordem importa: o request ID precisa existir antes do logger e do
	// recoverer, e o CORS deve responder ao preflight antes das rotas.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP(opts.TrustProxyHeaders))
	r.Use(middleware.Logger(opts.Logger))
	r.Use(middleware.Recoverer(opts.Logger))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(opts.AllowedOrigins))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		_ = response.Error(w, http.StatusNotFound, response.CodeNotFound,
			"recurso não encontrado", chimiddleware.GetReqID(r.Context()))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		_ = response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed,
			"método não permitido para este recurso", chimiddleware.GetReqID(r.Context()))
	})

	r.Get("/health", handlers.Health)

	if opts.Auth.Handler != nil {
		registerAuthRoutes(r, opts.Auth)
	}

	if opts.Resumes.Handler != nil && opts.Auth.RequireAuth != nil && opts.Auth.CSRF != nil {
		registerResumeRoutes(r, opts)
	}

	return r
}

// registerAuthRoutes monta as rotas do módulo de autenticação.
func registerAuthRoutes(r chi.Router, authOpts AuthOptions) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		if authOpts.CSRF != nil {
			r.Use(authOpts.CSRF)
		}

		h := authOpts.Handler
		r.Get("/csrf", h.CSRF)
		r.Post("/register", h.Register)
		r.Post("/verify-email", h.VerifyEmail)
		r.Post("/resend-verification", h.ResendVerification)
		r.Post("/login", h.Login)
		r.Post("/logout", h.Logout)
		r.Post("/forgot-password", h.ForgotPassword)
		r.Post("/reset-password", h.ResetPassword)

		if authOpts.RequireAuth != nil {
			r.Group(func(r chi.Router) {
				r.Use(authOpts.RequireAuth)
				r.Get("/me", h.Me)
			})
		}
	})
}

// registerResumeRoutes monta as rotas de currículos. Todas exigem sessão
// autenticada; os métodos de escrita também passam pelo middleware CSRF.
func registerResumeRoutes(r chi.Router, opts Options) {
	h := opts.Resumes.Handler
	guard := func(r chi.Router) {
		r.Use(opts.Auth.RequireAuth)
		r.Use(opts.Auth.CSRF)

		r.Get("/base", h.GetBase)
		r.Patch("/base", h.UpdateBase)

		r.Get("/versions", h.ListVersions)
		r.Post("/versions", h.CreateVersion)
		r.Get("/versions/{id}", h.GetVersion)
		r.Patch("/versions/{id}", h.UpdateVersion)
		r.Delete("/versions/{id}", h.DeleteVersion)
	}

	r.Route("/api/v1/resumes", guard)

	r.Group(func(r chi.Router) {
		r.Use(opts.Auth.RequireAuth)
		r.Use(opts.Auth.CSRF)
		r.Get("/api/v1/workspace", h.Workspace)
	})
}
