package usecase_test

import (
	"context"
	"errors"
	"testing"

	"certextractor/internal/application/usecase"
	"certextractor/internal/domain/entity"
)

type mockExtractor struct {
	cert *entity.Certificate
	err  error
}

func (m *mockExtractor) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cert, nil
}

func (m *mockExtractor) ProviderName() string {
	return "MockExtractor"
}

func TestExtractCertificateService(t *testing.T) {
	ctx := context.Background()

	t.Run("sucesso na extração", func(t *testing.T) {
		expectedCert, _ := entity.NewCertificate("Aluno Teste", "Curso Go", 10, "EAD", "10/05/2024", "Curso")
		service := usecase.NewExtractCertificateService(&mockExtractor{cert: expectedCert})

		cert, err := service.Execute(ctx, "caminho/falso.jpg")
		if err != nil {
			t.Fatalf("esperava sucesso, obteve erro: %v", err)
		}
		if cert.StudentName != "Aluno Teste" {
			t.Errorf("esperava 'Aluno Teste', obteve %s", cert.StudentName)
		}
	})

	t.Run("falha na extração", func(t *testing.T) {
		service := usecase.NewExtractCertificateService(&mockExtractor{err: errors.New("timeout na API")})

		_, err := service.Execute(ctx, "caminho/falso.jpg")
		if err == nil {
			t.Fatal("esperava erro, mas retornou nil")
		}
	})
}
