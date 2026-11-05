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
}

// New cria o roteador com todas as rotas e middlewares da API.
func New(opts Options) http.Handler {
	r := chi.NewRouter()

	// A ordem importa: o request ID precisa existir antes do logger e do
	// recoverer, e o CORS deve responder ao preflight antes das rotas.
	r.Use(middleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger(opts.Logger))
	r.Use(middleware.Recoverer(opts.Logger))
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

	return r
}
