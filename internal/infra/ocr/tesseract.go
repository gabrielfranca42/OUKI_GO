package ocr

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TesseractProvider implementa ocr.Provider usando o binário Tesseract OCR via CLI.
// DDD: Adapter da camada de Infraestrutura.
type TesseractProvider struct{}

// NewTesseractProvider cria uma nova instância do provider Tesseract.
func NewTesseractProvider() *TesseractProvider {
	return &TesseractProvider{}
}

// ExtractText lê uma imagem e retorna o texto extraído via Tesseract CLI.
func (t *TesseractProvider) ExtractText(ctx context.Context, imagePath string) (string, error) {
	tmpOut := imagePath + "_out"
	defer func() { _ = os.Remove(tmpOut + ".txt") }()

	log.Printf("[Tesseract][INFO] Iniciando OCR para: %s", imagePath)
	startTime := time.Now()

	// Tenta executar com '-l por', com fallback para idioma padrão caso não esteja instalado
	cmd := exec.CommandContext(ctx, "tesseract", imagePath, tmpOut, "-l", "por")
	if _, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[Tesseract][WARN] Falha com '-l por' (%v). Tentando idioma padrão do sistema...", err)
		cmdFallback := exec.CommandContext(ctx, "tesseract", imagePath, tmpOut)
		if outputFallback, errFallback := cmdFallback.CombinedOutput(); errFallback != nil {
			log.Printf("[Tesseract][ERRO] Falha geral ao executar o binário tesseract: %v (Saída: %s)", errFallback, string(outputFallback))
			return "", fmt.Errorf("erro ao executar tesseract: %w — saída: %s", errFallback, string(outputFallback))
		}
		log.Printf("[Tesseract][INFO] Sucesso no OCR usando idioma padrão do sistema.")
	} else {
		log.Printf("[Tesseract][INFO] Sucesso no OCR usando pacote de idioma português ('por').")
	}

	data, err := os.ReadFile(tmpOut + ".txt")
	if err != nil {
		log.Printf("[Tesseract][ERRO] Falha ao ler arquivo de saída temporário: %v", err)
		return "", fmt.Errorf("erro ao ler saída do tesseract: %w", err)
	}

	resultText := strings.TrimSpace(string(data))
	log.Printf("[Tesseract][SUCESSO] Concluído em %v (%d caracteres extraídos)", time.Since(startTime), len(resultText))

	return resultText, nil
}
