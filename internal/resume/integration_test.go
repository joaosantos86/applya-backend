//go:build integration

// Testes de integração do módulo de currículos.
//
// Execução:
//
//	go test -tags=integration ./internal/resume
//
// Requer DATABASE_URL apontando para um PostgreSQL descartável. Os testes
// criam usuários únicos e removem cada um ao final (o ON DELETE CASCADE remove
// seus currículos). NUNCA rode contra um banco com dados reais.
package resume

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/joaosantos86/applya-backend/internal/database"
)

var testPool *pgxpool.Pool

var schemaCriado sync.Once

func TestMain(m *testing.M) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Println("DATABASE_URL não definida; testes de integração ignorados")
		os.Exit(0)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	pool, err := database.NewPool(context.Background(), databaseURL, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conectar ao postgres: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	schemaCriado.Do(func() {
		if err := createSchema(context.Background(), pool); err != nil {
			fmt.Fprintf(os.Stderr, "criar esquema: %v\n", err)
			pool.Close()
			os.Exit(1)
		}
	})

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// createSchema espelha db/migrations/20261106000000_create_auth_tables.sql
// (apenas users, dependência de currículos) e
// db/migrations/20261107000000_create_resumes.sql.
func createSchema(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
			email             TEXT NOT NULL UNIQUE CHECK (char_length(email) <= 254),
			password_hash     TEXT NOT NULL,
			email_verified_at TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS resumes (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			base_resume_id UUID,
			kind           TEXT NOT NULL CHECK (kind IN ('base', 'version')),
			name           TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
			data           JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
			revision       BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
			base_kind      TEXT,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT resumes_base_consistency CHECK (
				(kind = 'base' AND base_resume_id IS NULL AND base_kind IS NULL) OR
				(kind = 'version' AND base_resume_id IS NOT NULL AND base_kind = 'base')
			),
			CONSTRAINT resumes_no_self_reference CHECK (base_resume_id IS NULL OR base_resume_id <> id),
			CONSTRAINT resumes_id_user_kind_key UNIQUE (id, user_id, kind),
			CONSTRAINT resumes_base_same_user_kind
				FOREIGN KEY (base_resume_id, user_id, base_kind)
				REFERENCES resumes (id, user_id, kind)
				ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS resumes_one_base_per_user ON resumes (user_id) WHERE kind = 'base'`,
		`CREATE INDEX IF NOT EXISTS resumes_user_kind_updated_idx ON resumes (user_id, kind, updated_at DESC)`,
	}

	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("executar DDL: %w", err)
		}
	}

	return nil
}

func newTestUser(t *testing.T) string {
	t.Helper()

	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("gerar e-mail: %v", err)
	}
	email := fmt.Sprintf("resume-%s@example.com", hex.EncodeToString(raw[:]))

	var id string
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO users (name, email, password_hash) VALUES ('Usuário de teste', $1, 'hash') RETURNING id`,
		email).Scan(&id); err != nil {
		t.Fatalf("criar usuário: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id); err != nil {
			t.Errorf("remover usuário: %v", err)
		}
	})

	return id
}

func newTestService() *Service { return NewService(NewStore(testPool)) }

func TestEnsureBaseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	userID := newTestUser(t)
	service := newTestService()

	first, err := service.EnsureBase(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureBase: %v", err)
	}
	second, err := service.EnsureBase(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureBase (segunda chamada): %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("ids diferentes: %q e %q", first.ID, second.ID)
	}
	if first.Type != KindBase || first.BaseResumeID != nil {
		t.Errorf("currículo-base inválido: %+v", first)
	}

	var count int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM resumes WHERE user_id = $1 AND kind = 'base'`, userID).Scan(&count); err != nil {
		t.Fatalf("contar currículos-base: %v", err)
	}
	if count != 1 {
		t.Errorf("currículos-base = %d, esperado 1", count)
	}
}

func TestEnsureBaseConcurrent(t *testing.T) {
	ctx := context.Background()
	userID := newTestUser(t)
	service := newTestService()

	const goroutines = 16
	ids := make(chan string, goroutines)
	errs := make(chan error, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			base, err := service.EnsureBase(ctx, userID)
			if err != nil {
				errs <- err
				return
			}
			ids <- base.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Fatalf("EnsureBase concorrente: %v", err)
	}

	var first string
	for id := range ids {
		if first == "" {
			first = id
			continue
		}
		if id != first {
			t.Errorf("ids divergentes: %q e %q", first, id)
		}
	}

	var count int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM resumes WHERE user_id = $1 AND kind = 'base'`, userID).Scan(&count); err != nil {
		t.Fatalf("contar currículos-base: %v", err)
	}
	if count != 1 {
		t.Errorf("currículos-base = %d, esperado 1", count)
	}
}

func TestUpdateBasePersistsSectionsAndPreservesOmitted(t *testing.T) {
	ctx := context.Background()
	userID := newTestUser(t)
	service := newTestService()

	base, err := service.EnsureBase(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureBase: %v", err)
	}

	summary := "Resumo inicial"
	patch := Patch{
		Data: &DataPatch{
			Personal: &PersonalInfo{FullName: "João Silva", Email: "joao@example.com"},
			Summary:  &summary,
			Experiences: []Experience{{
				Company:   "ACME",
				Role:      "Backend",
				StartDate: "2024-01",
				Current:   true,
			}},
			Skills: []Skill{{Name: "Go"}, {Name: "PostgreSQL"}},
			Languages: []Language{{
				Language:    "Inglês",
				Proficiency: ProficiencyAdvanced,
			}},
		},
		Revision: &base.Revision,
	}

	updated, err := service.UpdateBase(ctx, userID, patch)
	if err != nil {
		t.Fatalf("UpdateBase: %v", err)
	}

	if updated.Data.Personal.FullName != "João Silva" {
		t.Errorf("full_name = %q", updated.Data.Personal.FullName)
	}
	if len(updated.Data.Experiences) != 1 || updated.Data.Experiences[0].ID == "" {
		t.Errorf("experiência não persistida corretamente: %+v", updated.Data.Experiences)
	}
	if len(updated.Data.Skills) != 2 || updated.Data.Skills[1].DisplayOrder != 1 {
		t.Errorf("habilidades/ordem não persistidas: %+v", updated.Data.Skills)
	}
	if updated.Revision != base.Revision+1 {
		t.Errorf("revision = %d, esperado %d", updated.Revision, base.Revision+1)
	}

	// Patch de uma seção preserva as demais.
	newSummary := "Resumo atualizado"
	patched, err := service.UpdateBase(ctx, userID, Patch{Data: &DataPatch{Summary: &newSummary}})
	if err != nil {
		t.Fatalf("UpdateBase (resumo): %v", err)
	}
	if patched.Data.Summary != newSummary {
		t.Errorf("summary = %q", patched.Data.Summary)
	}
	if len(patched.Data.Experiences) != 1 || len(patched.Data.Skills) != 2 {
		t.Error("seções omitidas deveriam ser preservadas")
	}

	// Coleção vazia esvazia a seção.
	cleared, err := service.UpdateBase(ctx, userID, Patch{Data: &DataPatch{Skills: []Skill{}}})
	if err != nil {
		t.Fatalf("UpdateBase (limpar habilidades): %v", err)
	}
	if len(cleared.Data.Skills) != 0 {
		t.Errorf("skills = %d, esperado 0", len(cleared.Data.Skills))
	}
	if len(cleared.Data.Experiences) != 1 {
		t.Error("experiências deveriam continuar intactas")
	}

	// Revisão antiga é rejeitada.
	stale := base.Revision
	if _, err := service.UpdateBase(ctx, userID, Patch{Revision: &stale, Data: &DataPatch{Summary: &newSummary}}); !errors.Is(err, ErrConflict) {
		t.Errorf("erro = %v, esperado ErrConflict", err)
	}
}

func TestVersionLifecycleAndIsolation(t *testing.T) {
	ctx := context.Background()
	userA := newTestUser(t)
	userB := newTestUser(t)
	service := newTestService()

	baseA, err := service.EnsureBase(ctx, userA)
	if err != nil {
		t.Fatalf("EnsureBase A: %v", err)
	}

	summary := "Resumo do base"
	if _, err := service.UpdateBase(ctx, userA, Patch{Data: &DataPatch{
		Personal: &PersonalInfo{FullName: "Pessoa A"},
		Summary:  &summary,
		Skills:   []Skill{{Name: "Go"}},
	}}); err != nil {
		t.Fatalf("UpdateBase A: %v", err)
	}

	version, err := service.CreateVersion(ctx, userA, "Backend Go")
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if version.Type != KindVersion || version.BaseResumeID == nil || *version.BaseResumeID != baseA.ID {
		t.Errorf("versão inválida: %+v", version)
	}
	if version.Data.Summary != summary || len(version.Data.Skills) != 1 {
		t.Error("versão não copiou os dados do currículo-base")
	}

	// Alterar o base não altera a versão.
	newSummary := "Resumo alterado no base"
	if _, err := service.UpdateBase(ctx, userA, Patch{Data: &DataPatch{Summary: &newSummary}}); err != nil {
		t.Fatalf("UpdateBase A (2): %v", err)
	}
	reloaded, err := service.GetVersion(ctx, userA, version.ID)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if reloaded.Data.Summary != summary {
		t.Errorf("summary da versão = %q, esperado %q (independente)", reloaded.Data.Summary, summary)
	}

	// Alterar a versão não altera o base.
	versionSummary := "Resumo da versão"
	if _, err := service.UpdateVersion(ctx, userA, version.ID, Patch{
		Name: stringPtr("Backend Go Sênior"),
		Data: &DataPatch{Summary: &versionSummary},
	}); err != nil {
		t.Fatalf("UpdateVersion: %v", err)
	}
	baseReloaded, err := service.EnsureBase(ctx, userA)
	if err != nil {
		t.Fatalf("EnsureBase (2): %v", err)
	}
	if baseReloaded.Data.Summary != newSummary {
		t.Errorf("summary do base = %q, esperado %q", baseReloaded.Data.Summary, newSummary)
	}
	if baseReloaded.Name == "Backend Go Sênior" {
		t.Error("nome do base foi alterado pela versão")
	}

	// Revisão antiga da versão também é rejeitada.
	staleRevision := version.Revision
	otherSummary := "Não deve ser aplicado"
	if _, err := service.UpdateVersion(ctx, userA, version.ID, Patch{
		Revision: &staleRevision,
		Data:     &DataPatch{Summary: &otherSummary},
	}); !errors.Is(err, ErrConflict) {
		t.Errorf("erro = %v, esperado ErrConflict com revisão antiga da versão", err)
	}

	// Outro usuário não acessa, edita nem exclui a versão.
	if _, err := service.GetVersion(ctx, userB, version.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVersion por outro usuário: erro = %v, esperado ErrNotFound", err)
	}
	if _, err := service.UpdateVersion(ctx, userB, version.ID, Patch{Name: stringPtr("Hack")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateVersion por outro usuário: erro = %v, esperado ErrNotFound", err)
	}
	if err := service.DeleteVersion(ctx, userB, version.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteVersion por outro usuário: erro = %v, esperado ErrNotFound", err)
	}
	if _, err := service.GetVersion(ctx, userA, version.ID); err != nil {
		t.Errorf("versão deveria continuar existindo: %v", err)
	}

	// O dono exclui a versão.
	if err := service.DeleteVersion(ctx, userA, version.ID); err != nil {
		t.Fatalf("DeleteVersion: %v", err)
	}
	if _, err := service.GetVersion(ctx, userA, version.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("erro = %v, esperado ErrNotFound após exclusão", err)
	}
}

func TestDeleteBaseIsNotAllowedByVersionPath(t *testing.T) {
	ctx := context.Background()
	userID := newTestUser(t)
	service := newTestService()

	base, err := service.EnsureBase(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureBase: %v", err)
	}

	if err := service.DeleteVersion(ctx, userID, base.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteVersion no base: erro = %v, esperado ErrNotFound", err)
	}
	if _, err := service.UpdateVersion(ctx, userID, base.ID, Patch{Name: stringPtr("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateVersion no base: erro = %v, esperado ErrNotFound", err)
	}

	reloaded, err := service.EnsureBase(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureBase (2): %v", err)
	}
	if reloaded.ID != base.ID {
		t.Error("currículo-base foi removido")
	}
}

func TestVersionsAreListedOnlyForOwner(t *testing.T) {
	ctx := context.Background()
	userA := newTestUser(t)
	userB := newTestUser(t)
	service := newTestService()

	if _, err := service.CreateVersion(ctx, userA, "A1"); err != nil {
		t.Fatalf("CreateVersion A1: %v", err)
	}
	if _, err := service.CreateVersion(ctx, userA, "A2"); err != nil {
		t.Fatalf("CreateVersion A2: %v", err)
	}
	if _, err := service.CreateVersion(ctx, userB, "B1"); err != nil {
		t.Fatalf("CreateVersion B1: %v", err)
	}

	versionsA, err := service.ListVersions(ctx, userA)
	if err != nil {
		t.Fatalf("ListVersions A: %v", err)
	}
	if len(versionsA) != 2 {
		t.Errorf("versões de A = %d, esperado 2", len(versionsA))
	}

	versionsB, err := service.ListVersions(ctx, userB)
	if err != nil {
		t.Fatalf("ListVersions B: %v", err)
	}
	if len(versionsB) != 1 || versionsB[0].Name != "B1" {
		t.Errorf("versões de B = %+v, esperado apenas B1", versionsB)
	}
}

func TestConcurrentSectionUpdatesDoNotLoseData(t *testing.T) {
	ctx := context.Background()
	userID := newTestUser(t)
	service := newTestService()

	if _, err := service.EnsureBase(ctx, userID); err != nil {
		t.Fatalf("EnsureBase: %v", err)
	}

	// Cada rodada atualiza resumo e habilidades ao mesmo tempo, sem revision.
	// O SELECT ... FOR UPDATE deve serializar as transações e preservar as duas
	// seções; uma perda de atualização apareceria como resumo ou habilidade
	// ausente no recurso final.
	for round := 0; round < 5; round++ {
		summary := fmt.Sprintf("Resumo concorrente %d", round)
		skillName := fmt.Sprintf("Habilidade %d", round)

		start := make(chan struct{})
		errs := make(chan error, 2)

		go func() {
			<-start
			_, err := service.UpdateBase(ctx, userID, Patch{Data: &DataPatch{Summary: &summary}})
			errs <- err
		}()
		go func() {
			<-start
			_, err := service.UpdateBase(ctx, userID, Patch{Data: &DataPatch{Skills: []Skill{{Name: skillName}}}})
			errs <- err
		}()

		close(start)
		for i := 0; i < 2; i++ {
			if err := <-errs; err != nil {
				t.Fatalf("rodada %d: atualização concorrente: %v", round, err)
			}
		}

		base, err := service.EnsureBase(ctx, userID)
		if err != nil {
			t.Fatalf("rodada %d: recarregar base: %v", round, err)
		}
		if base.Data.Summary != summary {
			t.Errorf("rodada %d: summary = %q, esperado %q", round, base.Data.Summary, summary)
		}
		if len(base.Data.Skills) != 1 || base.Data.Skills[0].Name != skillName {
			t.Errorf("rodada %d: skills = %+v, esperado [%s]", round, base.Data.Skills, skillName)
		}
	}
}

func TestWorkspaceEnsuresBaseAndListsVersions(t *testing.T) {
	ctx := context.Background()
	userID := newTestUser(t)
	service := newTestService()

	base, versions, err := service.Workspace(ctx, userID)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	if base.Type != KindBase || len(versions) != 0 {
		t.Fatalf("workspace inicial inesperado: base=%+v versions=%+v", base, versions)
	}
	baseID := base.ID

	if _, err := service.CreateVersion(ctx, userID, "Backend Go"); err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}

	base, versions, err = service.Workspace(ctx, userID)
	if err != nil {
		t.Fatalf("Workspace (2): %v", err)
	}
	if base.ID != baseID {
		t.Fatal("base não deveria mudar")
	}
	if len(versions) != 1 || versions[0].Name != "Backend Go" {
		t.Errorf("versions = %+v, esperado Backend Go", versions)
	}
}

func TestDatabaseRejectsInvalidResumeRelationships(t *testing.T) {
	ctx := context.Background()
	userA := newTestUser(t)
	userB := newTestUser(t)
	service := newTestService()

	baseA, err := service.EnsureBase(ctx, userA)
	if err != nil {
		t.Fatalf("EnsureBase A: %v", err)
	}
	baseB, err := service.EnsureBase(ctx, userB)
	if err != nil {
		t.Fatalf("EnsureBase B: %v", err)
	}

	versionA, err := service.CreateVersion(ctx, userA, "V1")
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}

	cases := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "segundo currículo-base do mesmo usuário",
			sql:  `INSERT INTO resumes (user_id, kind, name) VALUES ($1, 'base', 'Outro')`,
			args: []any{userA},
		},
		{
			name: "versão apontando para base de outro usuário",
			sql:  `INSERT INTO resumes (user_id, base_resume_id, base_kind, kind, name) VALUES ($1, $2, 'base', 'version', 'X')`,
			args: []any{userB, baseA.ID},
		},
		{
			name: "versão apontando para outra versão",
			sql:  `INSERT INTO resumes (user_id, base_resume_id, base_kind, kind, name) VALUES ($1, $2, 'base', 'version', 'X')`,
			args: []any{userA, versionA.ID},
		},
		{
			name: "versão sem relação de base",
			sql:  `INSERT INTO resumes (user_id, kind, name) VALUES ($1, 'version', 'X')`,
			args: []any{userA},
		},
		{
			name: "currículo-base com pai",
			sql:  `INSERT INTO resumes (user_id, base_resume_id, base_kind, kind, name) VALUES ($1, $2, 'base', 'base', 'X')`,
			args: []any{userA, baseA.ID},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := testPool.Exec(ctx, tc.sql, tc.args...); err == nil {
				t.Fatal("INSERT deveria ter sido rejeitado pelo banco")
			}
		})
	}

	// O base do usuário B continua válido e não foi afetado.
	if _, err := service.EnsureBase(ctx, userB); err != nil {
		t.Fatalf("EnsureBase B após tentativas inválidas: %v", err)
	}
	if baseB.ID == baseA.ID {
		t.Error("currículos-base de usuários diferentes não deveriam coincidir")
	}
}
