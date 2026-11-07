package resume

import (
	"errors"
	"strings"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }
func stringPtr(v string) *string {
	return &v
}

func TestPatchValidateFullDocument(t *testing.T) {
	t.Parallel()

	patch := Patch{
		Name: stringPtr("  Versão Backend  "),
		Data: &DataPatch{
			Personal: &PersonalInfo{
				FullName: "  João Silva ",
				Email:    "joao@example.com",
				LinkedIn: "https://linkedin.com/in/joao",
				GitHub:   "https://github.com/joao",
				Website:  "https://joao.dev",
				Phone:    "+55 (11) 99999-0000",
			},
			Summary: stringPtr("  Desenvolvedor Go  "),
			Experiences: []Experience{{
				Company:   " Applya ",
				Role:      " Backend ",
				StartDate: "2024-01",
				EndDate:   "2025-06",
			}},
			Skills: []Skill{{Name: " Go "}},
			Projects: []Project{{
				Name:         "API",
				Technologies: []string{" Go ", "PostgreSQL"},
				URL:          "https://example.com",
			}},
			Languages: []Language{{Language: "Inglês", Proficiency: ProficiencyAdvanced}},
		},
	}

	if err := patch.Validate(); err != nil {
		t.Fatalf("Validate() = %v, esperado nil", err)
	}

	if *patch.Name != "Versão Backend" {
		t.Errorf("nome = %q, esperado aparado", *patch.Name)
	}
	if patch.Data.Personal.FullName != "João Silva" {
		t.Errorf("full_name = %q, esperado aparado", patch.Data.Personal.FullName)
	}
	if *patch.Data.Summary != "Desenvolvedor Go" {
		t.Errorf("summary = %q, esperado aparado", *patch.Data.Summary)
	}
	if patch.Data.Experiences[0].Company != "Applya" {
		t.Errorf("empresa = %q, esperado aparado", patch.Data.Experiences[0].Company)
	}
	if patch.Data.Experiences[0].ID == "" {
		t.Error("id da experiência deveria ser gerado")
	}
	if patch.Data.Experiences[0].DisplayOrder != 0 {
		t.Errorf("display_order = %d, esperado 0", patch.Data.Experiences[0].DisplayOrder)
	}
	if patch.Data.Projects[0].Technologies[0] != "Go" {
		t.Errorf("tecnologia = %q, esperado aparada", patch.Data.Projects[0].Technologies[0])
	}
}

func TestPatchValidateErrors(t *testing.T) {
	t.Parallel()

	longText := strings.Repeat("a", MaxSummaryLength+1)
	tooManySkills := make([]Skill, MaxSkills+1)
	for i := range tooManySkills {
		tooManySkills[i] = Skill{Name: "skill"}
	}

	cases := []struct {
		name  string
		patch Patch
	}{
		{"vazio", Patch{}},
		{"revision inválida", Patch{Revision: int64Ptr(0), Name: stringPtr("x")}},
		{"nome vazio", Patch{Name: stringPtr("   ")}},
		{"nome longo", Patch{Name: stringPtr(strings.Repeat("a", MaxNameLength+1))}},
		{"data sem seções", Patch{Data: &DataPatch{}}},
		{"resumo longo", Patch{Data: &DataPatch{Summary: stringPtr(longText)}}},
		{"e-mail inválido", Patch{Data: &DataPatch{Personal: &PersonalInfo{Email: "invalido"}}}},
		{"telefone inválido", Patch{Data: &DataPatch{Personal: &PersonalInfo{Phone: "abc123"}}}},
		{"telefone longo", Patch{Data: &DataPatch{Personal: &PersonalInfo{Phone: strings.Repeat("1", MaxPhoneLength+1)}}}},
		{"linkedin inválido", Patch{Data: &DataPatch{Personal: &PersonalInfo{LinkedIn: "linkedin.com/in/x"}}}},
		{"website inválido", Patch{Data: &DataPatch{Personal: &PersonalInfo{Website: "ftp://example.com"}}}},
		{"mês inválido", Patch{Data: &DataPatch{Experiences: []Experience{{Company: "A", Role: "B", StartDate: "2024-13"}}}}},
		{"fim antes do início", Patch{Data: &DataPatch{Experiences: []Experience{{Company: "A", Role: "B", StartDate: "2025-01", EndDate: "2024-01"}}}}},
		{"término sem início", Patch{Data: &DataPatch{Experiences: []Experience{{Company: "A", Role: "B", EndDate: "2024-01"}}}}},
		{"atual com término", Patch{Data: &DataPatch{Experiences: []Experience{{Company: "A", Role: "B", StartDate: "2024-01", EndDate: "2024-02", Current: true}}}}},
		{"empresa ausente", Patch{Data: &DataPatch{Experiences: []Experience{{Role: "B"}}}}},
		{"id duplicado", Patch{Data: &DataPatch{Skills: []Skill{{ID: "x", Name: "A"}, {ID: "x", Name: "B"}}}}},
		{"id inválido", Patch{Data: &DataPatch{Skills: []Skill{{ID: "a b", Name: "A"}}}}},
		{"proficiência inválida", Patch{Data: &DataPatch{Languages: []Language{{Language: "Inglês", Proficiency: "expert"}}}}},
		{"tecnologia vazia", Patch{Data: &DataPatch{Projects: []Project{{Name: "P", Technologies: []string{"Go", " "}}}}}},
		{"projeto sem nome", Patch{Data: &DataPatch{Projects: []Project{{URL: "https://example.com"}}}}},
		{"formaçao sem curso", Patch{Data: &DataPatch{Education: []Education{{Institution: "X"}}}}},
		{"muitas habilidades", Patch{Data: &DataPatch{Skills: tooManySkills}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.patch.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, esperado erro")
			}

			var validationErr ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("erro = %T, esperado ValidationError", err)
			}
		})
	}
}

func TestPatchValidateAcceptsEmptyCollectionsToClear(t *testing.T) {
	t.Parallel()

	patch := Patch{Data: &DataPatch{
		Experiences: []Experience{},
		Skills:      []Skill{},
		Education:   []Education{},
		Projects:    []Project{},
		Languages:   []Language{},
	}}

	if err := patch.Validate(); err != nil {
		t.Fatalf("Validate() = %v, esperado nil", err)
	}
}

func TestApplyDataPatchPreservesOmittedSections(t *testing.T) {
	t.Parallel()

	current := Data{
		Personal:    PersonalInfo{FullName: "João"},
		Summary:     "Resumo antigo",
		Experiences: []Experience{{ID: "e1", Company: "ACME"}},
		Skills:      []Skill{{ID: "s1", Name: "Go"}},
	}
	current.Normalize()

	newSummary := "Resumo novo"
	updated := applyDataPatch(current, DataPatch{
		Summary:     &newSummary,
		Experiences: []Experience{},
	})

	if updated.Summary != newSummary {
		t.Errorf("summary = %q, esperado %q", updated.Summary, newSummary)
	}
	if updated.Personal.FullName != "João" {
		t.Error("personal omitido deveria ser preservado")
	}
	if len(updated.Experiences) != 0 {
		t.Errorf("experiences = %d itens, esperado 0", len(updated.Experiences))
	}
	if len(updated.Skills) != 1 {
		t.Errorf("skills = %d itens, esperado 1 (preservada)", len(updated.Skills))
	}
}

func TestApplyDataPatchDoesNotMutateOriginalCollections(t *testing.T) {
	t.Parallel()

	original := []Skill{{ID: "s1", Name: "Go"}}
	current := NewData()
	current.Skills = original

	replacement := []Skill{{ID: "s2", Name: "Vue"}}
	updated := applyDataPatch(current, DataPatch{Skills: replacement})
	updated.Skills[0].Name = "Alterado"

	if original[0].Name != "Go" {
		t.Errorf("coleção original foi mutada: %q", original[0].Name)
	}
}

func TestValidID(t *testing.T) {
	t.Parallel()

	valid := "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	if !ValidID(valid) {
		t.Errorf("ValidID(%q) = false, esperado true", valid)
	}

	for _, invalid := range []string{"", "1", "not-a-uuid", "3f2504e0-4f89-11d3-9a0c", "3f2504e0-4f89-11d3-9a0c-0305e82c330G"} {
		if ValidID(invalid) {
			t.Errorf("ValidID(%q) = true, esperado false", invalid)
		}
	}
}

func TestNewDataSerializesEmptyCollections(t *testing.T) {
	t.Parallel()

	data := NewData()
	data.Normalize()

	if data.Experiences == nil || data.Education == nil || data.Skills == nil || data.Projects == nil || data.Languages == nil {
		t.Fatal("coleções de NewData não deveriam ser nil")
	}
}
