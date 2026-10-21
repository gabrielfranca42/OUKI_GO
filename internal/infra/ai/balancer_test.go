package ai_test

import (
	"context"
	"errors"
	"testing"

	"certextractor/internal/domain/entity"
	"certextractor/internal/infra/ai"
)

type mockProvider struct {
	name string
	cert *entity.Certificate
	err  error
}

func (m *mockProvider) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cert, nil
}

func (m *mockProvider) ProviderName() string {
	return m.name
}

func TestExtractorBalancer(t *testing.T) {
	ctx := context.Background()
	certA, _ := entity.NewCertificate("Aluno A", "Curso A", 10, "EAD", "01/01/2024", "Curso")
	certB, _ := entity.NewCertificate("Aluno B", "Curso B", 20, "EAD", "01/02/2024", "Curso")

	t.Run("Round Robin com dois provedores", func(t *testing.T) {
		p1 := &mockProvider{name: "P1", cert: certA}
		p2 := &mockProvider{name: "P2", cert: certB}
		balancer := ai.NewExtractorBalancer(p1, p2)

		res1, err := balancer.ExtractData(ctx, "img1.png")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		res2, err := balancer.ExtractData(ctx, "img2.png")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		if res1.StudentName == res2.StudentName {
			t.Errorf("esperava alternância de provedores (Round Robin)")
		}
	})

	t.Run("Fallback quando primário falha", func(t *testing.T) {
		p1 := &mockProvider{name: "Falho", err: errors.New("falha na rede")}
		p2 := &mockProvider{name: "Sucesso", cert: certB}
		balancer := ai.NewExtractorBalancer(p1, p2)

		res, err := balancer.ExtractData(ctx, "img.png")
		if err != nil {
			t.Fatalf("esperava que o fallback tivesse sucesso, obteve erro: %v", err)
		}
		if res.StudentName != "Aluno B" {
			t.Errorf("esperava dados do provedor de fallback")
		}
	})

	t.Run("Todos os provedores falham", func(t *testing.T) {
		p1 := &mockProvider{name: "Falho 1", err: errors.New("erro 1")}
		p2 := &mockProvider{name: "Falho 2", err: errors.New("erro 2")}
		balancer := ai.NewExtractorBalancer(p1, p2)

		_, err := balancer.ExtractData(ctx, "img.png")
		if err == nil {
			t.Fatal("esperava erro ao falhar todos os provedores")
		}
	})

	t.Run("Sem provedores configurados", func(t *testing.T) {
		balancer := ai.NewExtractorBalancer()
		_, err := balancer.ExtractData(ctx, "img.png")
		if err == nil {
			t.Fatal("esperava erro para balanceador vazio")
		}
	})
}
