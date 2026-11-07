package resume

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Testes de regra de negócio que devem ser rejeitados antes de qualquer acesso
// ao banco. O serviço é criado sem store para provar que a validação acontece
// na borda do domínio.
func TestServiceValidatesBeforePersistence(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	ctx := context.Background()

	validID := "3f2504e0-4f89-11d3-9a0c-0305e82c3301"

	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "criar versão sem nome",
			call: func() error {
				_, err := service.CreateVersion(ctx, "user-1", "   ")
				return err
			},
		},
		{
			name: "criar versão com nome longo",
			call: func() error {
				_, err := service.CreateVersion(ctx, "user-1", strings.Repeat("a", MaxNameLength+1))
				return err
			},
		},
		{
			name: "atualizar base sem campos",
			call: func() error {
				_, err := service.UpdateBase(ctx, "user-1", Patch{})
				return err
			},
		},
		{
			name: "atualizar base com revisão inválida",
			call: func() error {
				revision := int64(0)
				_, err := service.UpdateBase(ctx, "user-1", Patch{Name: stringPtr("x"), Revision: &revision})
				return err
			},
		},
		{
			name: "buscar versão com id inválido",
			call: func() error {
				_, err := service.GetVersion(ctx, "user-1", "nao-e-uuid")
				return err
			},
		},
		{
			name: "atualizar versão com id inválido",
			call: func() error {
				_, err := service.UpdateVersion(ctx, "user-1", "nao-e-uuid", Patch{Name: stringPtr("x")})
				return err
			},
		},
		{
			name: "excluir versão com id inválido",
			call: func() error {
				return service.DeleteVersion(ctx, "user-1", "nao-e-uuid")
			},
		},
		{
			name: "atualizar versão válida com patch inválido",
			call: func() error {
				_, err := service.UpdateVersion(ctx, "user-1", validID, Patch{})
				return err
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.call()
			if err == nil {
				t.Fatal("erro = nil, esperado ValidationError")
			}

			var validationErr ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("erro = %T (%v), esperado ValidationError", err, err)
			}
		})
	}
}
