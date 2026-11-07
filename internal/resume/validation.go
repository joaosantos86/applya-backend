package resume

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	monthPattern  = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	uuidPattern   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	itemIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// ValidID informa se o identificador recebido na rota é um UUID canônico.
// A validação evita enviar valores arbitrários ao PostgreSQL.
func ValidID(id string) bool {
	return uuidPattern.MatchString(id)
}

// ValidatePatch valida e normaliza um patch em memória. O patch é mutado:
// strings são aparadas, identificadores ausentes são gerados e display_order é
// recalculado.
func (p *Patch) Validate() error {
	if p.Revision != nil && *p.Revision < 1 {
		return NewValidationError("revision deve ser maior ou igual a 1")
	}

	if p.Name == nil && p.Data == nil {
		return NewValidationError("informe ao menos um campo para atualizar")
	}

	if p.Name != nil {
		name, err := normalizeName(*p.Name)
		if err != nil {
			return err
		}
		p.Name = &name
	}

	if p.Data != nil {
		if err := p.Data.validate(); err != nil {
			return err
		}
	}

	return nil
}

// validate valida as seções presentes no DataPatch.
func (p *DataPatch) validate() error {
	if p.Personal == nil && p.Summary == nil && p.Experiences == nil &&
		p.Education == nil && p.Skills == nil && p.Projects == nil && p.Languages == nil {
		return NewValidationError("data não contém nenhuma seção para atualizar")
	}

	if p.Personal != nil {
		if err := validatePersonal(p.Personal); err != nil {
			return err
		}
	}

	if p.Summary != nil {
		summary := strings.TrimSpace(*p.Summary)
		if utf8.RuneCountInString(summary) > MaxSummaryLength {
			return NewValidationError("resumo profissional deve ter no máximo %d caracteres", MaxSummaryLength)
		}
		p.Summary = &summary
	}

	if p.Experiences != nil {
		if err := validateExperiences(p.Experiences); err != nil {
			return err
		}
	}
	if p.Education != nil {
		if err := validateEducation(p.Education); err != nil {
			return err
		}
	}
	if p.Skills != nil {
		if err := validateSkills(p.Skills); err != nil {
			return err
		}
	}
	if p.Projects != nil {
		if err := validateProjects(p.Projects); err != nil {
			return err
		}
	}
	if p.Languages != nil {
		if err := validateLanguages(p.Languages); err != nil {
			return err
		}
	}

	return nil
}

func normalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", NewValidationError("nome do currículo é obrigatório")
	}
	if utf8.RuneCountInString(name) > MaxNameLength {
		return "", NewValidationError("nome do currículo deve ter no máximo %d caracteres", MaxNameLength)
	}
	return name, nil
}

func validatePersonal(p *PersonalInfo) error {
	p.FullName = strings.TrimSpace(p.FullName)
	p.Title = strings.TrimSpace(p.Title)
	p.Email = strings.TrimSpace(p.Email)
	p.Phone = strings.TrimSpace(p.Phone)
	p.City = strings.TrimSpace(p.City)
	p.State = strings.TrimSpace(p.State)
	p.LinkedIn = strings.TrimSpace(p.LinkedIn)
	p.GitHub = strings.TrimSpace(p.GitHub)
	p.Website = strings.TrimSpace(p.Website)

	if err := checkMax("nome completo", p.FullName, MaxFullNameLength); err != nil {
		return err
	}
	if err := checkMax("título profissional", p.Title, MaxTitleLength); err != nil {
		return err
	}
	if err := checkMax("cidade", p.City, MaxLocationLength); err != nil {
		return err
	}
	if err := checkMax("estado", p.State, MaxLocationLength); err != nil {
		return err
	}

	if p.Email != "" {
		if utf8.RuneCountInString(p.Email) > MaxEmailLength {
			return NewValidationError("e-mail de contato deve ter no máximo %d caracteres", MaxEmailLength)
		}
		addr, err := mail.ParseAddress(p.Email)
		if err != nil || addr.Address != p.Email {
			return NewValidationError("e-mail de contato inválido")
		}
	}

	if p.Phone != "" {
		if err := checkMax("telefone", p.Phone, MaxPhoneLength); err != nil {
			return err
		}
		if !validPhone(p.Phone) {
			return NewValidationError("telefone inválido; use apenas dígitos, espaços e os caracteres + ( ) - .")
		}
	}

	if err := validateURL("LinkedIn", p.LinkedIn); err != nil {
		return err
	}
	if err := validateURL("GitHub", p.GitHub); err != nil {
		return err
	}
	if err := validateURL("site", p.Website); err != nil {
		return err
	}

	return nil
}

func validPhone(phone string) bool {
	if utf8.RuneCountInString(phone) < 3 {
		return false
	}

	digits := 0
	for _, r := range phone {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == ' ', r == '+', r == '(', r == ')', r == '-', r == '.':
		default:
			return false
		}
	}
	return digits >= 3
}

func validateURL(field, raw string) error {
	if raw == "" {
		return nil
	}
	if utf8.RuneCountInString(raw) > MaxURLLength {
		return NewValidationError("%s deve ter no máximo %d caracteres", field, MaxURLLength)
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return NewValidationError("%s deve ser uma URL http:// ou https:// válida", field)
	}
	return nil
}

func validateExperiences(items []Experience) error {
	if len(items) > MaxExperiences {
		return NewValidationError("no máximo %d experiências são permitidas", MaxExperiences)
	}

	seen := make(map[string]struct{}, len(items))
	for i := range items {
		item := &items[i]

		item.Company = strings.TrimSpace(item.Company)
		item.Role = strings.TrimSpace(item.Role)
		item.Location = strings.TrimSpace(item.Location)
		item.StartDate = strings.TrimSpace(item.StartDate)
		item.EndDate = strings.TrimSpace(item.EndDate)
		item.Description = strings.TrimSpace(item.Description)

		if item.Company == "" {
			return NewValidationError("experiência %d: empresa é obrigatória", i+1)
		}
		if err := checkMax(fmt.Sprintf("experiência %d: empresa", i+1), item.Company, MaxCompanyLength); err != nil {
			return err
		}
		if item.Role == "" {
			return NewValidationError("experiência %d: cargo é obrigatório", i+1)
		}
		if err := checkMax(fmt.Sprintf("experiência %d: cargo", i+1), item.Role, MaxRoleLength); err != nil {
			return err
		}
		if err := checkMax(fmt.Sprintf("experiência %d: localização", i+1), item.Location, MaxLocationLength); err != nil {
			return err
		}
		if err := checkMax(fmt.Sprintf("experiência %d: descrição", i+1), item.Description, MaxDescriptionLength); err != nil {
			return err
		}
		if err := validatePeriod(item.StartDate, item.EndDate, item.Current); err != nil {
			return NewValidationError("experiência %d: %s", i+1, err)
		}

		if err := normalizeItemID(&item.ID, seen); err != nil {
			return NewValidationError("experiência %d: %s", i+1, err)
		}
		item.DisplayOrder = i
	}

	return nil
}

func validateEducation(items []Education) error {
	if len(items) > MaxEducation {
		return NewValidationError("no máximo %d formações são permitidas", MaxEducation)
	}

	seen := make(map[string]struct{}, len(items))
	for i := range items {
		item := &items[i]

		item.Institution = strings.TrimSpace(item.Institution)
		item.Course = strings.TrimSpace(item.Course)
		item.Degree = strings.TrimSpace(item.Degree)
		item.StartDate = strings.TrimSpace(item.StartDate)
		item.EndDate = strings.TrimSpace(item.EndDate)
		item.Description = strings.TrimSpace(item.Description)

		if item.Institution == "" {
			return NewValidationError("formação %d: instituição é obrigatória", i+1)
		}
		if err := checkMax(fmt.Sprintf("formação %d: instituição", i+1), item.Institution, MaxInstitutionLength); err != nil {
			return err
		}
		if item.Course == "" {
			return NewValidationError("formação %d: curso é obrigatório", i+1)
		}
		if err := checkMax(fmt.Sprintf("formação %d: curso", i+1), item.Course, MaxCourseLength); err != nil {
			return err
		}
		if err := checkMax(fmt.Sprintf("formação %d: grau", i+1), item.Degree, MaxDegreeLength); err != nil {
			return err
		}
		if err := checkMax(fmt.Sprintf("formação %d: descrição", i+1), item.Description, MaxDescriptionLength); err != nil {
			return err
		}
		if err := validatePeriod(item.StartDate, item.EndDate, false); err != nil {
			return NewValidationError("formação %d: %s", i+1, err)
		}

		if err := normalizeItemID(&item.ID, seen); err != nil {
			return NewValidationError("formação %d: %s", i+1, err)
		}
		item.DisplayOrder = i
	}

	return nil
}

func validateSkills(items []Skill) error {
	if len(items) > MaxSkills {
		return NewValidationError("no máximo %d habilidades são permitidas", MaxSkills)
	}

	seen := make(map[string]struct{}, len(items))
	for i := range items {
		item := &items[i]

		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			return NewValidationError("habilidade %d: nome é obrigatório", i+1)
		}
		if err := checkMax(fmt.Sprintf("habilidade %d: nome", i+1), item.Name, MaxSkillNameLength); err != nil {
			return err
		}
		if err := normalizeItemID(&item.ID, seen); err != nil {
			return NewValidationError("habilidade %d: %s", i+1, err)
		}
		item.DisplayOrder = i
	}

	return nil
}

func validateProjects(items []Project) error {
	if len(items) > MaxProjects {
		return NewValidationError("no máximo %d projetos são permitidos", MaxProjects)
	}

	seen := make(map[string]struct{}, len(items))
	for i := range items {
		item := &items[i]

		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		item.URL = strings.TrimSpace(item.URL)
		item.Repository = strings.TrimSpace(item.Repository)

		if item.Name == "" {
			return NewValidationError("projeto %d: nome é obrigatório", i+1)
		}
		if err := checkMax(fmt.Sprintf("projeto %d: nome", i+1), item.Name, MaxProjectNameLength); err != nil {
			return err
		}
		if err := checkMax(fmt.Sprintf("projeto %d: descrição", i+1), item.Description, MaxDescriptionLength); err != nil {
			return err
		}
		if err := validateURL(fmt.Sprintf("projeto %d: URL", i+1), item.URL); err != nil {
			return err
		}
		if err := validateURL(fmt.Sprintf("projeto %d: repositório", i+1), item.Repository); err != nil {
			return err
		}

		if len(item.Technologies) > MaxTechnologies {
			return NewValidationError("projeto %d: no máximo %d tecnologias são permitidas", i+1, MaxTechnologies)
		}
		technologies := make([]string, len(item.Technologies))
		for j, tech := range item.Technologies {
			tech = strings.TrimSpace(tech)
			if tech == "" {
				return NewValidationError("projeto %d: tecnologia %d não pode ser vazia", i+1, j+1)
			}
			if utf8.RuneCountInString(tech) > MaxTechnologyLength {
				return NewValidationError("projeto %d: tecnologia %d deve ter no máximo %d caracteres", i+1, j+1, MaxTechnologyLength)
			}
			technologies[j] = tech
		}
		item.Technologies = technologies

		if err := normalizeItemID(&item.ID, seen); err != nil {
			return NewValidationError("projeto %d: %s", i+1, err)
		}
		item.DisplayOrder = i
	}

	return nil
}

func validateLanguages(items []Language) error {
	if len(items) > MaxLanguages {
		return NewValidationError("no máximo %d idiomas são permitidos", MaxLanguages)
	}

	seen := make(map[string]struct{}, len(items))
	for i := range items {
		item := &items[i]

		item.Language = strings.TrimSpace(item.Language)
		item.Proficiency = strings.TrimSpace(item.Proficiency)

		if item.Language == "" {
			return NewValidationError("idioma %d: idioma é obrigatório", i+1)
		}
		if err := checkMax(fmt.Sprintf("idioma %d: idioma", i+1), item.Language, MaxLanguageLength); err != nil {
			return err
		}
		if !validProficiency(item.Proficiency) {
			return NewValidationError("idioma %d: proficiência deve ser beginner, intermediate, advanced, fluent ou native", i+1)
		}
		if err := normalizeItemID(&item.ID, seen); err != nil {
			return NewValidationError("idioma %d: %s", i+1, err)
		}
		item.DisplayOrder = i
	}

	return nil
}

func validProficiency(value string) bool {
	switch value {
	case ProficiencyBeginner, ProficiencyIntermediate, ProficiencyAdvanced, ProficiencyFluent, ProficiencyNative:
		return true
	default:
		return false
	}
}

// validatePeriod valida datas no formato YYYY-MM. Datas são opcionais, mas o
// término exige início e não pode ser anterior a ele. Um trabalho marcado como
// atual não pode ter data de término.
func validatePeriod(start, end string, current bool) error {
	if start != "" && !monthPattern.MatchString(start) {
		return NewValidationError("data de início deve estar no formato YYYY-MM")
	}
	if end != "" && !monthPattern.MatchString(end) {
		return NewValidationError("data de término deve estar no formato YYYY-MM")
	}
	if current && end != "" {
		return NewValidationError("trabalho atual não pode ter data de término")
	}
	if end != "" && start == "" {
		return NewValidationError("data de término exige data de início")
	}
	if start != "" && end != "" && end < start {
		return NewValidationError("data de término não pode ser anterior à data de início")
	}
	return nil
}

// normalizeItemID valida um identificador recebido ou gera um novo. IDs devem
// ser únicos dentro da coleção.
func normalizeItemID(id *string, seen map[string]struct{}) error {
	value := strings.TrimSpace(*id)
	if value == "" {
		generated, err := newItemID()
		if err != nil {
			return fmt.Errorf("gerar identificador do item: %w", err)
		}
		value = generated
	}

	if !itemIDPattern.MatchString(value) {
		return NewValidationError("identificador inválido; use até %d caracteres entre letras, números, _ e -", MaxItemIDLength)
	}
	if _, exists := seen[value]; exists {
		return NewValidationError("identificador duplicado em %q", value)
	}
	seen[value] = struct{}{}

	*id = value
	return nil
}

// newItemID gera um UUID v4 sem dependência externa.
func newItemID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])

	return string(buf[:]), nil
}

func checkMax(field, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return NewValidationError("%s deve ter no máximo %d caracteres", field, max)
	}
	return nil
}
