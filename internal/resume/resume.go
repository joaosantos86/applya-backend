// Package resume implementa o domínio de currículos do Applya: currículo-base
// único por usuário, versões independentes criadas a partir dele, validação dos
// dados profissionais e persistência em PostgreSQL (pgx).
//
// O módulo é dono da tabela resumes. Handlers HTTP ficam em
// internal/http/handlers; este pacote concentra regras de negócio, validação e
// SQL.
package resume

import (
	"errors"
	"fmt"
	"time"
)

// Kind identifica se um currículo é o currículo-base do usuário ou uma versão
// derivada dele.
type Kind string

const (
	// KindBase é o único currículo-base lógico de cada usuário.
	KindBase Kind = "base"
	// KindVersion é uma versão independente criada a partir do currículo-base.
	KindVersion Kind = "version"
)

// Níveis de proficiência aceitos em idiomas.
const (
	ProficiencyBeginner     = "beginner"
	ProficiencyIntermediate = "intermediate"
	ProficiencyAdvanced     = "advanced"
	ProficiencyFluent       = "fluent"
	ProficiencyNative       = "native"
)

// Limites de validação e tamanho. Eles são aplicados no backend de forma
// independente da validação feita pelo frontend.
const (
	// DefaultBaseName é o nome inicial do currículo-base.
	DefaultBaseName = "Currículo-base"

	// MaxResumeBodyBytes limita o corpo JSON de uma requisição de currículo
	// (512 KiB). O limite também mantém o documento JSONB em tamanho saudável.
	MaxResumeBodyBytes = 512 << 10

	// MaxNameLength limita o nome do currículo (base ou versão).
	MaxNameLength = 120

	// Limites de campos pessoais.
	MaxFullNameLength = 120
	MaxTitleLength    = 120
	MaxEmailLength    = 254
	MaxPhoneLength    = 40
	MaxLocationLength = 100
	MaxURLLength      = 2048

	// Limites de texto livre.
	MaxSummaryLength     = 5000
	MaxDescriptionLength = 4000

	// Limites de coleções.
	MaxExperiences  = 50
	MaxEducation    = 30
	MaxSkills       = 100
	MaxProjects     = 50
	MaxLanguages    = 20
	MaxTechnologies = 50

	// Limites de campos de item.
	MaxCompanyLength     = 200
	MaxRoleLength        = 200
	MaxInstitutionLength = 200
	MaxCourseLength      = 200
	MaxDegreeLength      = 120
	MaxSkillNameLength   = 100
	MaxProjectNameLength = 200
	MaxLanguageLength    = 100
	MaxTechnologyLength  = 100

	// MaxItemIDLength limita o identificador de itens de coleção.
	MaxItemIDLength = 64
)

// Erros de negócio devolvidos pelo serviço. Os handlers os traduzem para
// status HTTP e códigos estáveis.
var (
	// ErrNotFound indica currículo inexistente, de outro usuário, do tipo
	// errado para a operação ou já excluído. O handler responde 404.
	ErrNotFound = errors.New("currículo não encontrado")

	// ErrConflict indica que a revisão enviada pelo cliente não corresponde à
	// revisão atual: outra requisição alterou o currículo no intervalo.
	ErrConflict = errors.New("currículo foi alterado por outra requisição")
)

// ValidationError é um erro de validação de entrada com mensagem própria para
// o usuário final. O handler responde 400 com o código validation_failed.
type ValidationError struct {
	msg string
}

// NewValidationError cria um ValidationError com a mensagem informada.
func NewValidationError(format string, args ...any) ValidationError {
	return ValidationError{msg: fmt.Sprintf(format, args...)}
}

// Error implementa a interface error.
func (e ValidationError) Error() string { return e.msg }

// PersonalInfo reúne os dados pessoais de contato do currículo.
type PersonalInfo struct {
	FullName string `json:"full_name"`
	Title    string `json:"title"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	City     string `json:"city"`
	State    string `json:"state"`
	LinkedIn string `json:"linkedin"`
	GitHub   string `json:"github"`
	Website  string `json:"website"`
}

// Experience é uma experiência profissional do currículo.
type Experience struct {
	ID           string `json:"id"`
	Company      string `json:"company"`
	Role         string `json:"role"`
	Location     string `json:"location"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Current      bool   `json:"current"`
	Description  string `json:"description"`
	DisplayOrder int    `json:"display_order"`
}

// Education é uma formação acadêmica do currículo.
type Education struct {
	ID           string `json:"id"`
	Institution  string `json:"institution"`
	Course       string `json:"course"`
	Degree       string `json:"degree"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Description  string `json:"description"`
	DisplayOrder int    `json:"display_order"`
}

// Skill é uma habilidade do currículo.
type Skill struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayOrder int    `json:"display_order"`
}

// Project é um projeto do currículo.
type Project struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Technologies []string `json:"technologies"`
	URL          string   `json:"url"`
	Repository   string   `json:"repository"`
	DisplayOrder int      `json:"display_order"`
}

// Language é um idioma do currículo.
type Language struct {
	ID           string `json:"id"`
	Language     string `json:"language"`
	Proficiency  string `json:"proficiency"`
	DisplayOrder int    `json:"display_order"`
}

// Data é o documento profissional do currículo. É persistido como JSONB: o
// documento é sempre lido e gravado por inteiro, e as coleções são listas
// ordenadas — não há consultas relacionais dentro das seções neste MVP.
type Data struct {
	Personal    PersonalInfo `json:"personal"`
	Summary     string       `json:"summary"`
	Experiences []Experience `json:"experiences"`
	Education   []Education  `json:"education"`
	Skills      []Skill      `json:"skills"`
	Projects    []Project    `json:"projects"`
	Languages   []Language   `json:"languages"`
}

// NewData cria um documento vazio com coleções não nulas, para que o JSON
// serializado use [] em vez de null.
func NewData() Data {
	return Data{
		Experiences: []Experience{},
		Education:   []Education{},
		Skills:      []Skill{},
		Projects:    []Project{},
		Languages:   []Language{},
	}
}

// Normalize garante coleções não nulas e recalcula display_order a partir da
// posição no array. A ordem enviada pelo cliente é a ordem preservada; o
// servidor apenas materializa essa ordem no campo display_order.
func (d *Data) Normalize() {
	if d.Experiences == nil {
		d.Experiences = []Experience{}
	}
	for i := range d.Experiences {
		d.Experiences[i].DisplayOrder = i
	}

	if d.Education == nil {
		d.Education = []Education{}
	}
	for i := range d.Education {
		d.Education[i].DisplayOrder = i
	}

	if d.Skills == nil {
		d.Skills = []Skill{}
	}
	for i := range d.Skills {
		d.Skills[i].DisplayOrder = i
	}

	if d.Projects == nil {
		d.Projects = []Project{}
	}
	for i := range d.Projects {
		if d.Projects[i].Technologies == nil {
			d.Projects[i].Technologies = []string{}
		}
		d.Projects[i].DisplayOrder = i
	}

	if d.Languages == nil {
		d.Languages = []Language{}
	}
	for i := range d.Languages {
		d.Languages[i].DisplayOrder = i
	}
}

// Resume é um currículo persistido (base ou versão), como devolvido pela API.
// user_id nunca é exposto: o proprietário é sempre o usuário da sessão.
type Resume struct {
	ID           string    `json:"id"`
	Type         Kind      `json:"type"`
	Name         string    `json:"name"`
	BaseResumeID *string   `json:"base_resume_id"`
	Revision     int64     `json:"revision"`
	Data         Data      `json:"data"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Summary é a projeção resumida usada na sidebar (lista de versões).
type Summary struct {
	ID           string    `json:"id"`
	Type         Kind      `json:"type"`
	Name         string    `json:"name"`
	BaseResumeID *string   `json:"base_resume_id"`
	Revision     int64     `json:"revision"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Summary devolve a projeção resumida do currículo.
func (r Resume) Summary() Summary {
	return Summary{
		ID:           r.ID,
		Type:         r.Type,
		Name:         r.Name,
		BaseResumeID: r.BaseResumeID,
		Revision:     r.Revision,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

// Patch descreve uma atualização parcial de currículo.
//
// Semântica (documentada em docs/api-resumes.md):
//   - name omitido preserva o nome atual;
//   - data omitido preserva o documento;
//   - dentro de data, cada seção omitida é preservada e cada seção enviada
//     substitui integralmente a seção atual (inclusive coleções vazias);
//   - revision, quando enviada, exige que o currículo ainda esteja naquela
//     revisão, evitando sobrescrever alterações concorrentes.
type Patch struct {
	Name     *string    `json:"name"`
	Data     *DataPatch `json:"data"`
	Revision *int64     `json:"revision"`
}

// DataPatch descreve a substituição de seções do documento. Slices nil
// significam "seção omitida"; slices vazias (não nil) esvaziam a seção.
type DataPatch struct {
	Personal    *PersonalInfo `json:"personal"`
	Summary     *string       `json:"summary"`
	Experiences []Experience  `json:"experiences"`
	Education   []Education   `json:"education"`
	Skills      []Skill       `json:"skills"`
	Projects    []Project     `json:"projects"`
	Languages   []Language    `json:"languages"`
}
