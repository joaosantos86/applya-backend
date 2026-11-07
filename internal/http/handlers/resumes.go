package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/joaosantos86/applya-backend/internal/auth"
	"github.com/joaosantos86/applya-backend/internal/http/response"
	"github.com/joaosantos86/applya-backend/internal/resume"
)

// ResumeService é a interface que os handlers de currículos consomem do módulo
// de negócio. Definida no pacote consumidor, como manda a convenção do
// projeto: resume.Service a implementa em produção e os testes usam um fake.
type ResumeService interface {
	EnsureBase(ctx context.Context, userID string) (resume.Resume, error)
	UpdateBase(ctx context.Context, userID string, patch resume.Patch) (resume.Resume, error)
	ListVersions(ctx context.Context, userID string) ([]resume.Summary, error)
	CreateVersion(ctx context.Context, userID, name string) (resume.Resume, error)
	GetVersion(ctx context.Context, userID, versionID string) (resume.Resume, error)
	UpdateVersion(ctx context.Context, userID, versionID string, patch resume.Patch) (resume.Resume, error)
	DeleteVersion(ctx context.Context, userID, versionID string) error
	Workspace(ctx context.Context, userID string) (resume.Resume, []resume.Summary, error)
}

// ResumeHandler expõe os endpoints REST de currículos. Cada handler é fino:
// decodifica o JSON, delega ao serviço e escreve a resposta. As regras de
// negócio e o SQL vivem em internal/resume.
type ResumeHandler struct {
	service ResumeService
	logger  *slog.Logger
}

// NewResumeHandler cria o handler de currículos.
func NewResumeHandler(service ResumeService, logger *slog.Logger) *ResumeHandler {
	return &ResumeHandler{service: service, logger: logger}
}

// ---------------------------------------------------------------------------
// Corpos de requisição e resposta
// ---------------------------------------------------------------------------

type createVersionRequest struct {
	Name string `json:"name"`
}

type resumeResponse struct {
	Resume resume.Resume `json:"resume"`
}

type versionsResponse struct {
	Versions []resume.Summary `json:"versions"`
}

type workspaceResponse struct {
	User     auth.PublicUser  `json:"user"`
	Base     resume.Resume    `json:"base"`
	Versions []resume.Summary `json:"versions"`
}

// errInvalidResumeBody padroniza o 400 de JSON malformado, tipo incorreto ou
// campo desconhecido.
var errInvalidResumeBody = errors.New("corpo da requisição inválido ou em formato JSON incorreto")

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// GetBase trata GET /api/v1/resumes/base. Cria o currículo-base vazio quando
// ainda não existe; chamadas repetidas devolvem o mesmo recurso.
func (h *ResumeHandler) GetBase(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	base, err := h.service.EnsureBase(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, resumeResponse{Resume: base})
}

// UpdateBase trata PATCH /api/v1/resumes/base.
func (h *ResumeHandler) UpdateBase(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	var patch resume.Patch
	if err := decodeResumeJSON(w, r, &patch); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}

	updated, err := h.service.UpdateBase(r.Context(), user.ID, patch)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, resumeResponse{Resume: updated})
}

// ListVersions trata GET /api/v1/resumes/versions.
func (h *ResumeHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	versions, err := h.service.ListVersions(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, versionsResponse{Versions: versions})
}

// CreateVersion trata POST /api/v1/resumes/versions.
func (h *ResumeHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	var req createVersionRequest
	if err := decodeResumeJSON(w, r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}

	version, err := h.service.CreateVersion(r.Context(), user.ID, req.Name)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusCreated, resumeResponse{Resume: version})
}

// GetVersion trata GET /api/v1/resumes/versions/{id}.
func (h *ResumeHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	version, err := h.service.GetVersion(r.Context(), user.ID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, resumeResponse{Resume: version})
}

// UpdateVersion trata PATCH /api/v1/resumes/versions/{id}.
func (h *ResumeHandler) UpdateVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	var patch resume.Patch
	if err := decodeResumeJSON(w, r, &patch); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}

	updated, err := h.service.UpdateVersion(r.Context(), user.ID, chi.URLParam(r, "id"), patch)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, resumeResponse{Resume: updated})
}

// DeleteVersion trata DELETE /api/v1/resumes/versions/{id}. O currículo-base
// nunca é excluído por este endpoint (a query filtra kind = 'version').
func (h *ResumeHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	if err := h.service.DeleteVersion(r.Context(), user.ID, chi.URLParam(r, "id")); err != nil {
		h.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Workspace trata GET /api/v1/workspace: em uma única resposta devolve o
// usuário autenticado, o currículo-base e a lista resumida de versões.
func (h *ResumeHandler) Workspace(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}

	base, versions, err := h.service.Workspace(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	_ = response.JSON(w, http.StatusOK, workspaceResponse{
		User:     user.Public(),
		Base:     base,
		Versions: versions,
	})
}

// ---------------------------------------------------------------------------
// Apoio
// ---------------------------------------------------------------------------

// writeUnauthenticated é uma defesa extra: as rotas de currículos sempre
// passam por RequireAuth, mas o handler não confia nisso cegamente.
func writeUnauthenticated(w http.ResponseWriter, r *http.Request) {
	_ = response.Error(w, http.StatusUnauthorized, response.CodeAuthenticationRequired,
		"autenticação necessária", middleware.GetReqID(r.Context()))
}

// decodeResumeJSON lê e valida o formato do corpo JSON, com limite de tamanho
// próprio do módulo. Campos desconhecidos são rejeitados (contrato fechado).
func decodeResumeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, resume.MaxResumeBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return maxErr
		}
		return errInvalidResumeBody
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errInvalidResumeBody
	}

	return nil
}

// writeDecodeError diferencia payload excessivo (413) de JSON inválido (400).
func (h *ResumeHandler) writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := middleware.GetReqID(r.Context())

	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		_ = response.Error(w, http.StatusRequestEntityTooLarge, response.CodePayloadTooLarge,
			"corpo da requisição excede o tamanho máximo permitido", requestID)
		return
	}

	_ = response.Error(w, http.StatusBadRequest, response.CodeValidationFail,
		err.Error(), requestID)
}

// writeError traduz erros de negócio do módulo para status e códigos estáveis.
func (h *ResumeHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := middleware.GetReqID(r.Context())

	var validationErr resume.ValidationError
	if errors.As(err, &validationErr) {
		_ = response.Error(w, http.StatusBadRequest, response.CodeValidationFail,
			validationErr.Error(), requestID)
		return
	}

	switch {
	case errors.Is(err, resume.ErrNotFound):
		_ = response.Error(w, http.StatusNotFound, response.CodeNotFound,
			"currículo não encontrado", requestID)
	case errors.Is(err, resume.ErrConflict):
		_ = response.Error(w, http.StatusConflict, response.CodeConflict,
			"o currículo foi alterado por outra requisição; recarregue e tente novamente", requestID)
	default:
		response.InternalError(w, r, h.logger, err, requestID)
	}
}
