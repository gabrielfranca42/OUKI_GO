package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TesseractProvider implementa ocr.Provider usando o binário Tesseract OCR via CLI.
// DDD: Adapter da camada de Infraestrutura.
// Não utiliza CGO — chama o executável `tesseract` diretamente.
type TesseractProvider struct{}

// NewTesseractProvider cria uma nova instância do provider Tesseract.
func NewTesseractProvider() *TesseractProvider {
	return &TesseractProvider{}
}

// ExtractText lê uma imagem e retorna o texto extraído via Tesseract CLI.
func (t *TesseractProvider) ExtractText(ctx context.Context, imagePath string) (string, error) {
	// Arquivo temporário para a saída do Tesseract (ele acrescenta .txt automaticamente)
	tmpOut := imagePath + "_out"
	defer func() { _ = os.Remove(tmpOut + ".txt") }()

	// Monta o comando: tesseract <imagem> <saída> -l por
	cmd := exec.CommandContext(ctx, "tesseract", imagePath, tmpOut, "-l", "por")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("erro ao executar tesseract: %w — saída: %s", err, string(output))
	}

	// Lê o arquivo de saída gerado pelo Tesseract
	data, err := os.ReadFile(tmpOut + ".txt")
	if err != nil {
		return "", fmt.Errorf("erro ao ler saída do tesseract: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}
