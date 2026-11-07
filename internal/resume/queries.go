package resume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store concentra a persistência do módulo de currículos (pgx). Toda query da
// tabela resumes vive aqui; nenhum SQL é escrito nos handlers.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore cria o Store sobre o pool informado.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// resumeColumns é a projeção completa de um currículo. A ordem corresponde a
// scanResume.
const resumeColumns = "id, user_id, base_resume_id, kind, name, data, revision, created_at, updated_at"

// summaryColumns é a projeção usada na sidebar (sem o documento JSONB).
const summaryColumns = "id, base_resume_id, kind, name, revision, created_at, updated_at"

// rowScanner descreve o contrato mínimo de pgx.Row necessário para ler uma
// linha.
type rowScanner interface {
	Scan(dest ...any) error
}

// uuidParam converte o ID textual em parâmetro pgtype.UUID. IDs chegam de
// fontes confiáveis (sessão autenticada ou rota validada), mas a conversão
// ainda pode falhar e é tratada como erro interno.
func uuidParam(id string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(id); err != nil {
		return u, fmt.Errorf("resume: id %q não é um uuid válido: %w", id, err)
	}
	return u, nil
}

// scanResume lê um currículo completo. pgx.ErrNoRows é devolvido sem wrapping
// para que o chamador o traduza em ErrNotFound.
func scanResume(row rowScanner) (Resume, error) {
	var (
		r       Resume
		id      pgtype.UUID
		userID  pgtype.UUID
		baseID  pgtype.UUID
		kind    string
		rawData []byte
	)

	if err := row.Scan(&id, &userID, &baseID, &kind, &r.Name, &rawData, &r.Revision, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return Resume{}, err
	}

	r.ID = id.String()
	r.Type = Kind(kind)
	if baseID.Valid {
		value := baseID.String()
		r.BaseResumeID = &value
	}

	r.Data = NewData()
	if len(rawData) > 0 {
		if err := json.Unmarshal(rawData, &r.Data); err != nil {
			return Resume{}, fmt.Errorf("resume: documento JSONB inválido para o currículo %s: %w", r.ID, err)
		}
	}
	r.Data.Normalize()

	return r, nil
}

// scanSummary lê a projeção resumida usada na listagem de versões.
func scanSummary(row rowScanner) (Summary, error) {
	var (
		s      Summary
		id     pgtype.UUID
		baseID pgtype.UUID
		kind   string
	)

	if err := row.Scan(&id, &baseID, &kind, &s.Name, &s.Revision, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return Summary{}, err
	}

	s.ID = id.String()
	s.Type = Kind(kind)
	if baseID.Valid {
		value := baseID.String()
		s.BaseResumeID = &value
	}

	return s, nil
}

// EnsureBase devolve o currículo-base do usuário, criando-o vazio quando ainda
// não existe.
//
// A criação é segura sob concorrência: o índice único parcial
// resumes_one_base_per_user, combinado com INSERT ... ON CONFLICT DO NOTHING,
// garante que duas requisições simultâneas nunca criem dois currículos-base.
// Quando o INSERT não devolve linha, a transação lê o registro já commitado
// pela requisição concorrente.
func (s *Store) EnsureBase(ctx context.Context, userID string) (Resume, error) {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return Resume{}, err
	}

	emptyData, err := json.Marshal(NewData())
	if err != nil {
		return Resume{}, fmt.Errorf("serializar currículo vazio: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Resume{}, fmt.Errorf("iniciar transação de currículo-base: %w", err)
	}
	defer tx.Rollback(ctx)

	resume, err := scanResume(tx.QueryRow(ctx,
		`INSERT INTO resumes (user_id, kind, name, data)
		 VALUES ($1, 'base', $2, $3)
		 ON CONFLICT (user_id) WHERE kind = 'base' DO NOTHING
		 RETURNING `+resumeColumns,
		userUUID, DefaultBaseName, emptyData))
	if errors.Is(err, pgx.ErrNoRows) {
		resume, err = scanResume(tx.QueryRow(ctx,
			`SELECT `+resumeColumns+` FROM resumes WHERE user_id = $1 AND kind = 'base'`,
			userUUID))
		if errors.Is(err, pgx.ErrNoRows) {
			return Resume{}, fmt.Errorf("currículo-base não encontrado após criação concorrente")
		}
	}
	if err != nil {
		return Resume{}, fmt.Errorf("garantir currículo-base: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Resume{}, fmt.Errorf("confirmar transação de currículo-base: %w", err)
	}

	return resume, nil
}

// Update aplica um patch ao currículo-base (versionID vazio) ou a uma versão.
//
// A linha é bloqueada com SELECT ... FOR UPDATE dentro da transação: patches
// de seções diferentes não se perdem, pois cada um lê o documento já
// atualizado pelo anterior. Quando o cliente envia revision, a divergência
// devolve ErrConflict antes de qualquer escrita.
func (s *Store) Update(ctx context.Context, userID string, kind Kind, versionID string, patch Patch) (Resume, error) {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return Resume{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Resume{}, fmt.Errorf("iniciar transação de atualização: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `SELECT ` + resumeColumns + ` FROM resumes WHERE user_id = $1 AND kind = $2`
	args := []any{userUUID, string(kind)}

	if kind == KindVersion {
		versionUUID, err := uuidParam(versionID)
		if err != nil {
			return Resume{}, ErrNotFound
		}
		query += ` AND id = $3`
		args = append(args, versionUUID)
	}
	query += ` FOR UPDATE`

	current, err := scanResume(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	if err != nil {
		return Resume{}, fmt.Errorf("bloquear currículo para atualização: %w", err)
	}

	if patch.Revision != nil && *patch.Revision != current.Revision {
		return Resume{}, ErrConflict
	}

	name := current.Name
	if patch.Name != nil {
		name = *patch.Name
	}

	data := current.Data
	if patch.Data != nil {
		data = applyDataPatch(data, *patch.Data)
	}
	data.Normalize()

	payload, err := json.Marshal(data)
	if err != nil {
		return Resume{}, fmt.Errorf("serializar documento do currículo: %w", err)
	}

	resumeID, err := uuidParam(current.ID)
	if err != nil {
		return Resume{}, err
	}

	updated, err := scanResume(tx.QueryRow(ctx,
		`UPDATE resumes
		 SET name = $1, data = $2, revision = revision + 1, updated_at = now()
		 WHERE id = $3 AND user_id = $4 AND kind = $5
		 RETURNING `+resumeColumns,
		name, payload, resumeID, userUUID, string(kind)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	if err != nil {
		return Resume{}, fmt.Errorf("atualizar currículo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Resume{}, fmt.Errorf("confirmar transação de atualização: %w", err)
	}

	return updated, nil
}

// applyDataPatch substitui apenas as seções presentes no patch. Slices nil
// representam seções omitidas; slices vazias esvaziam a seção.
func applyDataPatch(data Data, patch DataPatch) Data {
	if patch.Personal != nil {
		data.Personal = *patch.Personal
	}
	if patch.Summary != nil {
		data.Summary = *patch.Summary
	}
	if patch.Experiences != nil {
		data.Experiences = patch.Experiences
	}
	if patch.Education != nil {
		data.Education = patch.Education
	}
	if patch.Skills != nil {
		data.Skills = patch.Skills
	}
	if patch.Projects != nil {
		data.Projects = patch.Projects
	}
	if patch.Languages != nil {
		data.Languages = patch.Languages
	}
	return data
}

// CreateVersion cria uma versão copiando o documento do currículo-base dentro
// do próprio PostgreSQL (INSERT ... SELECT). A cópia é integral: a versão passa
// a ter seu próprio registro JSONB, sem compartilhar estrutura com o base.
func (s *Store) CreateVersion(ctx context.Context, userID, name string) (Resume, error) {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return Resume{}, err
	}

	version, err := scanResume(s.pool.QueryRow(ctx,
		`INSERT INTO resumes (user_id, base_resume_id, base_kind, kind, name, data)
		 SELECT user_id, id, 'base', 'version', $2, data
		 FROM resumes
		 WHERE user_id = $1 AND kind = 'base'
		 RETURNING `+resumeColumns,
		userUUID, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	if err != nil {
		return Resume{}, fmt.Errorf("criar versão: %w", err)
	}

	return version, nil
}

// ListVersions lista as versões do usuário, da mais recentemente alterada para
// a mais antiga.
func (s *Store) ListVersions(ctx context.Context, userID string) ([]Summary, error) {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+summaryColumns+`
		 FROM resumes
		 WHERE user_id = $1 AND kind = 'version'
		 ORDER BY updated_at DESC, created_at DESC, id`,
		userUUID)
	if err != nil {
		return nil, fmt.Errorf("listar versões: %w", err)
	}
	defer rows.Close()

	versions := make([]Summary, 0)
	for rows.Next() {
		version, err := scanSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("ler versão: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("percorrer versões: %w", err)
	}

	return versions, nil
}

// GetVersion devolve uma versão completa do usuário. Currículos-base nunca são
// devolvidos por este método.
func (s *Store) GetVersion(ctx context.Context, userID, versionID string) (Resume, error) {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return Resume{}, err
	}

	versionUUID, err := uuidParam(versionID)
	if err != nil {
		return Resume{}, ErrNotFound
	}

	version, err := scanResume(s.pool.QueryRow(ctx,
		`SELECT `+resumeColumns+`
		 FROM resumes
		 WHERE id = $1 AND user_id = $2 AND kind = 'version'`,
		versionUUID, userUUID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	if err != nil {
		return Resume{}, fmt.Errorf("buscar versão: %w", err)
	}

	return version, nil
}

// DeleteVersion exclui uma versão do usuário. O filtro por kind = 'version'
// impede que o currículo-base seja excluído por este caminho.
func (s *Store) DeleteVersion(ctx context.Context, userID, versionID string) error {
	userUUID, err := uuidParam(userID)
	if err != nil {
		return err
	}

	versionUUID, err := uuidParam(versionID)
	if err != nil {
		return ErrNotFound
	}

	ct, err := s.pool.Exec(ctx,
		`DELETE FROM resumes WHERE id = $1 AND user_id = $2 AND kind = 'version'`,
		versionUUID, userUUID)
	if err != nil {
		return fmt.Errorf("excluir versão: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
