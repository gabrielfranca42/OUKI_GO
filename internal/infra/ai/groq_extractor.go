package ai

import (
	"context"
	"fmt"

	"certextractor/internal/domain/entity"
	port "certextractor/internal/domain/port"
)

// GroqExtractor implementa port.CertificateExtractor
type GroqExtractor struct {
	ocrProvider port.Provider // Instância do Tesseract para ler texto
}

func NewGroqExtractor(ocrProvider port.Provider) *GroqExtractor {
	return &GroqExtractor{ocrProvider: ocrProvider}
}

func (g *GroqExtractor) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	// 1. Usa o Tesseract (Infra->Infra) para extrair o texto sujo
	text, err := g.ocrProvider.ExtractText(ctx, imagePath)
	if err != nil {
		return nil, fmt.Errorf("falha no Tesseract OCR: %w", err)
	}

	// 2. TODO: Enviar o texto extraído para a API da Groq estruturar
	
	preview := text
	if len(text) > 30 {
		preview = text[:30] + "..."
	}
	fmt.Printf("[Groq] Lendo texto cru via Tesseract: %s\n", preview)
	
	// Retorno mockado (substituir pela resposta do LLM Parseada)
	return entity.NewCertificate("Aluno Groq Teste", 20, "EAD")
}

func (g *GroqExtractor) ProviderName() string {
	return "Groq Llama 3"
}
