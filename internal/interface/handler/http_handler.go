package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"certextractor/internal/application/usecase"
	"certextractor/internal/infra/ai"
	"certextractor/internal/infra/ocr"
	"github.com/gin-gonic/gin"
)

var (
	service *usecase.ExtractCertificateService
	mu      sync.Mutex
)

// Inicializa o serviço e os providers na subida da aplicação
func init() {
	// 1. Instancia os Providers
	tesseractOCR := ocr.NewTesseractProvider()
	gemini := ai.NewGeminiExtractor()
	groq := ai.NewGroqExtractor(tesseractOCR) // Groq usa Tesseract para ter texto

	// 2. Instancia o Balanceador
	balancer := ai.NewExtractorBalancer(gemini, groq)

	// 3. Cria o UseCase de Domínio
	service = usecase.NewExtractCertificateService(balancer)
}

func ExtractHandler(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "campo 'image' obrigatório"})
		return
	}
	
	tmpPath := "./tmp_" + file.Filename
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "não foi possível salvar a imagem"})
		return
	}
	defer func() { _ = os.Remove(tmpPath) }()

	cert, err := service.Execute(c.Request.Context(), tmpPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Persistir resultado para o script Python (em JSONL)
	salvarResultadoNoDisco(cert)

	c.JSON(http.StatusOK, cert)
}

// salvarResultadoNoDisco salva os dados no formato JSONL (JSON por linha) para processamento em batch
func salvarResultadoNoDisco(cert interface{}) {
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile("dados_extraidos.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	bytes, _ := json.Marshal(cert)
	f.Write(bytes)
	f.WriteString("\n")
}
