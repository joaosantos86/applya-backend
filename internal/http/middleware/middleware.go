// Package middleware reúne os middlewares HTTP compartilhados pela API.
//
// Cada middleware deve ter uma responsabilidade única e depender apenas de
// net/http, log/slog e do pacote response quando precisar escrever JSON.
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

// corsMaxAge é por quanto tempo o navegador pode reaproveitar o resultado do
// preflight.
const corsMaxAge = 300

// requestIDHeader é o header usado para correlacionar uma requisição entre o
// cliente, os logs e as respostas de erro.
const requestIDHeader = "X-Request-Id"

// RequestID garante que toda requisição tenha um ID (reaproveitando o header
// enviado pelo cliente ou gerando um novo) e devolve esse ID na resposta.
func RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(requestIDHeader, middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	}))
}

// Logger registra cada requisição HTTP em formato estruturado.
//
// São registrados apenas metadados da requisição. Headers sensíveis (cookies,
// Authorization) e corpos de requisição nunca são registrados.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			logger.LogAttrs(r.Context(), levelForStatus(status), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

func levelForStatus(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// Recoverer recupera panics de handlers, registra o panic com stack trace no
// log do servidor e responde JSON 500 sem expor detalhes internos ao cliente.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				// ErrAbortHandler é usado pelo net/http para abortar uma
				// resposta de forma intencional; deve continuar propagando.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				requestID := middleware.GetReqID(r.Context())
				logger.ErrorContext(r.Context(), "panic recuperado em handler HTTP",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("request_id", requestID),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)

				// Se a resposta já começou a ser enviada, não é possível
				// enviar outro status.
				if written, ok := w.(interface{ Written() bool }); ok && written.Written() {
					return
				}

				_ = response.Error(w, http.StatusInternalServerError, response.CodeInternal,
					"erro interno do servidor", requestID)
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// CORS permite que o frontend (Vue) chame a API a partir das origens
// configuradas.
//
// AllowCredentials é habilitado desde já porque a autenticação futura usará
// cookies HTTP-only. Por isso as origens precisam ser explícitas: a combinação
// de credenciais com origem curinga é rejeitada pelo navegador.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           corsMaxAge,
	})
}
