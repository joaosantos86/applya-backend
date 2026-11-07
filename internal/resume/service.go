package resume

import (
	"context"
	"fmt"
)

// Service concentra as regras de negócio de currículos: garantia do
// currículo-base, validação dos patches, criação de versões e isolamento por
// usuário. A persistência fica em Store.
type Service struct {
	store *Store
}

// NewService cria o serviço sobre o store informado.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// EnsureBase devolve o currículo-base do usuário, criando-o quando necessário.
// A operação é idempotente e segura sob concorrência.
func (s *Service) EnsureBase(ctx context.Context, userID string) (Resume, error) {
	return s.store.EnsureBase(ctx, userID)
}

// UpdateBase valida e aplica um patch ao currículo-base.
func (s *Service) UpdateBase(ctx context.Context, userID string, patch Patch) (Resume, error) {
	if err := patch.Validate(); err != nil {
		return Resume{}, err
	}
	return s.store.Update(ctx, userID, KindBase, "", patch)
}

// ListVersions lista as versões do usuário.
func (s *Service) ListVersions(ctx context.Context, userID string) ([]Summary, error) {
	return s.store.ListVersions(ctx, userID)
}

// CreateVersion cria uma versão independente a partir do currículo-base,
// garantindo antes que o base exista.
func (s *Service) CreateVersion(ctx context.Context, userID, name string) (Resume, error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return Resume{}, err
	}

	if _, err := s.store.EnsureBase(ctx, userID); err != nil {
		return Resume{}, err
	}

	return s.store.CreateVersion(ctx, userID, normalized)
}

// GetVersion devolve uma versão do usuário.
func (s *Service) GetVersion(ctx context.Context, userID, versionID string) (Resume, error) {
	if !ValidID(versionID) {
		return Resume{}, NewValidationError("identificador de versão inválido")
	}
	return s.store.GetVersion(ctx, userID, versionID)
}

// UpdateVersion valida e aplica um patch a uma versão do usuário.
func (s *Service) UpdateVersion(ctx context.Context, userID, versionID string, patch Patch) (Resume, error) {
	if !ValidID(versionID) {
		return Resume{}, NewValidationError("identificador de versão inválido")
	}
	if err := patch.Validate(); err != nil {
		return Resume{}, err
	}
	return s.store.Update(ctx, userID, KindVersion, versionID, patch)
}

// DeleteVersion exclui uma versão do usuário. O currículo-base nunca é
// excluído por este caminho.
func (s *Service) DeleteVersion(ctx context.Context, userID, versionID string) error {
	if !ValidID(versionID) {
		return NewValidationError("identificador de versão inválido")
	}
	return s.store.DeleteVersion(ctx, userID, versionID)
}

// Workspace reúne o currículo-base e a lista de versões para o carregamento
// inicial da interface. O usuário é anexado pelo handler, a partir da sessão.
func (s *Service) Workspace(ctx context.Context, userID string) (Resume, []Summary, error) {
	base, err := s.store.EnsureBase(ctx, userID)
	if err != nil {
		return Resume{}, nil, err
	}

	versions, err := s.store.ListVersions(ctx, userID)
	if err != nil {
		return Resume{}, nil, fmt.Errorf("montar workspace: %w", err)
	}

	return base, versions, nil
}
