package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"certextractor/internal/domain/entity"
	port "certextractor/internal/domain/port"
)

// GroqExtractor implementa port.CertificateExtractor conectando à API real da Groq Cloud
type GroqExtractor struct {
	ocrProvider port.Provider
	httpClient  *http.Client
}

func NewGroqExtractor(ocrProvider port.Provider) *GroqExtractor {
	return &GroqExtractor{
		ocrProvider: ocrProvider,
		httpClient:  &http.Client{Timeout: 45 * time.Second},
	}
}

type groqMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type groqRequest struct {
	Model          string          `json:"model"`
	Messages       []groqMessage   `json:"messages"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Temperature    float64         `json:"temperature"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (g *GroqExtractor) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		log.Printf("[Groq][WARN] GROQ_API_KEY não configurada no .env.")
		return nil, fmt.Errorf("GROQ_API_KEY ausente no arquivo .env")
	}

	lowPath := strings.ToLower(imagePath)
	isImage := strings.HasSuffix(lowPath, ".png") || strings.HasSuffix(lowPath, ".jpg") ||
		strings.HasSuffix(lowPath, ".jpeg") || strings.HasSuffix(lowPath, ".webp")

	// 1. Se for imagem, tenta usar o modelo Multimodal Llama 3.2 Vision da Groq
	if isImage {
		cert, err := g.extractViaVision(ctx, apiKey, imagePath)
		if err == nil {
			return cert, nil
		}
		log.Printf("[Groq][WARN] Visão direta falhou (%v). Tentando via Tesseract OCR...", err)
	}

	// 2. Se for PDF ou se a visão falhar, extrai texto com OCR e usa Llama 3.3 Versatile
	return g.extractViaOCR(ctx, apiKey, imagePath)
}

func (g *GroqExtractor) extractViaVision(ctx context.Context, apiKey string, imagePath string) (*entity.Certificate, error) {
	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler imagem: %w", err)
	}

	mimeType := "image/jpeg"
	low := strings.ToLower(imagePath)
	if strings.HasSuffix(low, ".png") {
		mimeType = "image/png"
	} else if strings.HasSuffix(low, ".webp") {
		mimeType = "image/webp"
	}

	base64Data := base64.StdEncoding.EncodeToString(imgData)
	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

	prompt := `
Você é um especialista rigoroso em analisar certificados educacionais.
Analise a imagem enviada.
REGRAS CRÍTICAS:
1. Se a imagem NÃO for um certificado, diploma ou declaração de horas, defina "is_certificate" como false e preencha "reason" explicando o que a imagem é.
2. Se FOR certificado, defina "is_certificate" como true, e extraia:
   - "student_name" (Nome completo do aluno)
   - "course_name" (Nome do curso/atividade)
   - "hours" (Carga horária apenas em número inteiro, ex: 40)
   - "course_type" (EAD, Presencial, Workshop, etc)
   - "completion_date" (Data de conclusão no formato DD/MM/AAAA)
   - "category" (Deve ser: "Curso", "Palestra", "Evento", "Projeto de Extensão", "Estágio", ou "Outros")
Retorne APENAS um JSON válido com essas chaves exatas.
`

	contentParts := []map[string]interface{}{
		{"type": "text", "text": prompt},
		{"type": "image_url", "image_url": map[string]string{"url": dataURI}},
	}

	reqBody := groqRequest{
		Model: "llama-3.2-11b-vision-preview",
		Messages: []groqMessage{
			{Role: "user", Content: contentParts},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
		Temperature:    0.1,
	}

	log.Printf("[Groq][INFO] Enviando imagem para Groq Vision (llama-3.2-11b-vision-preview)...")
	return g.sendGroqRequest(ctx, apiKey, reqBody)
}

func (g *GroqExtractor) extractViaOCR(ctx context.Context, apiKey string, imagePath string) (*entity.Certificate, error) {
	if g.ocrProvider == nil {
		return nil, fmt.Errorf("provedor de OCR não configurado")
	}

	text, err := g.ocrProvider.ExtractText(ctx, imagePath)
	if err != nil {
		return nil, fmt.Errorf("falha no OCR local: %w", err)
	}

	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("OCR não encontrou nenhum texto legível no documento")
	}

	systemPrompt := `
Você é um especialista rigoroso em analisar texto de certificados educacionais.
Analise o texto cru extraído via OCR.
REGRAS CRÍTICAS:
1. Se o texto NÃO for de um certificado, diploma ou declaração de horas, defina "is_certificate" como false e preencha "reason".
2. Se FOR certificado, defina "is_certificate" como true, e extraia:
   - "student_name" (Nome completo do aluno)
   - "course_name" (Nome do curso ou atividade realizada)
   - "hours" (Carga horária em número inteiro, ex: 20)
   - "course_type" (EAD, Presencial, etc)
   - "completion_date" (Data no formato DD/MM/AAAA)
   - "category" ("Curso", "Palestra", "Evento", "Projeto de Extensão", "Estágio", ou "Outros")
Retorne APENAS um JSON válido seguindo essas chaves.
`

	userPrompt := fmt.Sprintf("TEXTO EXTRAÍDO DO DOCUMENTO:\n%s", text)

	reqBody := groqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []groqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
		Temperature:    0.1,
	}

	log.Printf("[Groq][INFO] Enviando texto OCR para Groq LLM (llama-3.3-70b-versatile)...")
	return g.sendGroqRequest(ctx, apiKey, reqBody)
}

func (g *GroqExtractor) sendGroqRequest(ctx context.Context, apiKey string, reqBody groqRequest) (*entity.Certificate, error) {
	jsonPayload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("falha ao serializar payload da Groq: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("falha ao criar requisição HTTP: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("falha na conexão com a API da Groq: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler resposta da Groq: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Groq][ERRO] HTTP %d da Groq: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("erro da API da Groq (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var groqResp groqResponse
	if err := json.Unmarshal(bodyBytes, &groqResp); err != nil {
		return nil, fmt.Errorf("falha ao parsear resposta da Groq: %w", err)
	}

	if len(groqResp.Choices) == 0 {
		return nil, fmt.Errorf("groq retornou escolhas vazias")
	}

	jsonContent := groqResp.Choices[0].Message.Content
	log.Printf("[Groq][INFO] Resposta da Groq recebida em %v: %s", time.Since(startTime), jsonContent)

	var resposta RespostaIA
	if err := json.Unmarshal([]byte(jsonContent), &resposta); err != nil {
		return nil, fmt.Errorf("falha ao decodificar JSON do Llama: %w (Recebido: %s)", err, jsonContent)
	}

	if !resposta.IsCertificate {
		log.Printf("[Groq][RECUSA] IA rejeitou o documento: %s", resposta.Reason)
		return nil, fmt.Errorf("A IA da Groq recusou o documento. Motivo: %s", resposta.Reason)
	}

	return entity.NewCertificate(
		resposta.StudentName,
		resposta.CourseName,
		resposta.Hours,
		resposta.CourseType,
		resposta.CompletionDate,
		resposta.Category,
	)
}

func (g *GroqExtractor) ProviderName() string {
	return "Groq Llama 3"
}
