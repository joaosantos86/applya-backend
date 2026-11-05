// Package handlers contém os handlers HTTP da API.
//
// Nesta etapa existe apenas o healthcheck. Handlers de negócio serão criados
// junto com seus módulos, mantendo cada handler pequeno: ler a requisição,
// chamar a camada responsável e escrever a resposta.
package handlers

import (
	"net/http"

	"github.com/joaosantos86/applya-backend/internal/http/response"
)

type healthResponse struct {
	Status string `json:"status"`
}

// Health responde se o processo HTTP está no ar.
//
// O healthcheck não consulta o banco de dados nem qualquer dependência externa:
// ele verifica somente a disponibilidade do servidor HTTP.
func Health(w http.ResponseWriter, _ *http.Request) {
	_ = response.JSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
