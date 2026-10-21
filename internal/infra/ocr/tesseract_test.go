package ocr_test

import (
	"context"
	"os"
	"testing"

	"certextractor/internal/infra/ocr"
)

func TestTesseractExtractText(t *testing.T) {
	provider := ocr.NewTesseractProvider()
	imgPath := "../../../certificados_simulados/cert_1_beatriz_python.png"
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		t.Skip("imagem simulada não encontrada")
	}

	text, err := provider.ExtractText(context.Background(), imgPath)
	if err != nil {
		t.Fatalf("erro no tesseract: %v", err)
	}
	if len(text) == 0 {
		t.Error("esperava texto extraído, obteve string vazia")
	}
}
