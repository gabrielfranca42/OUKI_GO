package local_test

import (
	"context"
	"os"
	"testing"

	"certextractor/internal/infra/local"
)

type mockOcrProvider struct {
	text string
}

func (m *mockOcrProvider) ExtractText(ctx context.Context, imagePath string) (string, error) {
	return m.text, nil
}

func TestLocalRegexExtractor(t *testing.T) {
	ctx := context.Background()

	t.Run("extração de certificado de curso padrão via OCR mock", func(t *testing.T) {
		ocrText := `
			CERTIFICADO DE CONCLUSÃO
			Certificamos que Beatriz Silva Santos concluiu com êxito o Curso de Machine Learning e Inteligência Artificial,
			com carga horária de 40 horas, realizado em 15/08/2024.
		`
		extractor := local.NewLocalRegexExtractor(&mockOcrProvider{text: ocrText})

		// Cria arquivo temporário falso
		tmp, _ := os.CreateTemp("", "cert*.png")
		defer os.Remove(tmp.Name())

		cert, err := extractor.ExtractData(ctx, tmp.Name())
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		if cert.StudentName != "Beatriz Silva Santos" {
			t.Errorf("esperava 'Beatriz Silva Santos', obteve '%s'", cert.StudentName)
		}
		if cert.DeclaredHours != 40 {
			t.Errorf("esperava 40h declaradas, obteve %d", cert.DeclaredHours)
		}
		if cert.ApprovedHours != 20 {
			t.Errorf("esperava 20h aproveitadas (limite máximo de 20h), obteve %d", cert.ApprovedHours)
		}
		if cert.Semester != "2024.2" {
			t.Errorf("esperava semestre 2024.2, obteve '%s'", cert.Semester)
		}
	})

	t.Run("extração de declaração de estágio", func(t *testing.T) {
		ocrText := `
			DECLARAÇÃO DE ESTÁGIO
			Declaramos para os devidos fins que Carlos Eduardo Rocha cumpriu com excelência as atividades de Estágio Supervisionado,
			totalizando 120 horas, concluído em 10 de maio de 2025.
		`
		extractor := local.NewLocalRegexExtractor(&mockOcrProvider{text: ocrText})

		tmp, _ := os.CreateTemp("", "estagio*.png")
		defer os.Remove(tmp.Name())

		cert, err := extractor.ExtractData(ctx, tmp.Name())
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		if cert.StudentName != "Carlos Eduardo Rocha" {
			t.Errorf("esperava 'Carlos Eduardo Rocha', obteve '%s'", cert.StudentName)
		}
		if cert.Category != "Estágio" {
			t.Errorf("esperava categoria 'Estágio', obteve '%s'", cert.Category)
		}
		if cert.ApprovedHours != 20 {
			t.Errorf("esperava 20h aproveitadas para estágio, obteve %d", cert.ApprovedHours)
		}
		if cert.Semester != "2025.1" {
			t.Errorf("esperava semestre 2025.1, obteve '%s'", cert.Semester)
		}
	})
}
