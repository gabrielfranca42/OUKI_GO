package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"certextractor/internal/domain/entity"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiExtractor implementa port.CertificateExtractor
type GeminiExtractor struct{}

func NewGeminiExtractor() *GeminiExtractor {
	return &GeminiExtractor{}
}

// RespostaIA mapeia exatamente a estrutura de JSON que pediremos pro LLM
type RespostaIA struct {
	IsCertificate bool   `json:"is_certificate"`
	StudentName   string `json:"student_name"`
	CourseName    string `json:"course_name"`
	Hours         int    `json:"hours"`
	CourseType    string `json:"course_type"`
	Reason        string `json:"reason"`
}

func (g *GeminiExtractor) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		// Modo MOCK ativado. A chave nunca deve subir hardcoded para o Git!
		return g.mockFallback(imagePath)
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("falha ao criar cliente Gemini: %w", err)
	}
	defer client.Close()

	// Usando o modelo mais rápido e barato (Flash) que suporta imagens
	// Corrigido para a versão da API ativa do Google
	model := client.GenerativeModel("gemini-flash-latest")
	model.ResponseMIMEType = "application/json" // Força o Gemini a cuspir JSON puro
	
	// ==========================================
	// O PROMPT: O Cérebro Anti-Tomate 
	// ==========================================
	prompt := `
Você é um especialista rigoroso em analisar certificados educacionais.
Analise a imagem enviada.
REGRAS CRÍTICAS:
1. Se a imagem NÃO for um certificado, diploma ou declaração de horas (ex: foto de comida, tomate, paisagem, pessoas, print de meme), defina "is_certificate" como false e preencha o campo "reason" explicando o que a imagem realmente é.
2. Se FOR de fato um certificado, defina "is_certificate" como true, e extraia:
   - "student_name" (Nome completo do aluno)
   - "course_name" (O nome do curso realizado)
   - "hours" (A carga horária apenas em número inteiro, ex: 40)
   - "course_type" (O tipo de curso: EAD, Presencial, Workshop, Palestra, etc)
Retorne APENAS o JSON válido seguindo essas chaves, sem nenhuma outra formatação.
`

	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo de imagem: %w", err)
	}

	var filePart genai.Part
	lowPath := strings.ToLower(imagePath)
	if strings.HasSuffix(lowPath, ".pdf") {
		filePart = genai.Blob{MIMEType: "application/pdf", Data: imgData}
	} else {
		mimeType := "jpeg"
		if strings.HasSuffix(lowPath, ".png") {
			mimeType = "png"
		} else if strings.HasSuffix(lowPath, ".webp") {
			mimeType = "webp"
		}
		filePart = genai.ImageData(mimeType, imgData)
	}

	// Dispara a requisição pra IA
	resp, err := model.GenerateContent(ctx, genai.Text(prompt), filePart)
	if err != nil {
		return nil, fmt.Errorf("falha na API do Gemini: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini retornou resposta vazia")
	}

	// Extrai o texto da resposta
	var jsonStr string
	if txt, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		jsonStr = string(txt)
	} else {
		return nil, fmt.Errorf("resposta em formato não reconhecido")
	}

	// Decodifica o JSON retornado pela IA para nossa Struct Go
	var resposta RespostaIA
	if err := json.Unmarshal([]byte(jsonStr), &resposta); err != nil {
		return nil, fmt.Errorf("falha ao decodificar JSON do Gemini: %w (Recebido: %s)", err, jsonStr)
	}

	// === 1ª BARREIRA: A DECISÃO DA IA ===
	if !resposta.IsCertificate {
		return nil, fmt.Errorf("A IA recusou a imagem. Motivo: %s", resposta.Reason)
	}

	// === 2ª BARREIRA: A REGRA DE NEGÓCIO DO GO ===
	return entity.NewCertificate(resposta.StudentName, resposta.CourseName, resposta.Hours, resposta.CourseType)
}

func (g *GeminiExtractor) mockFallback(imagePath string) (*entity.Certificate, error) {
	fmt.Printf("[Gemini] (MOCK - Chave ausente) Lendo: %s\n", imagePath)
	return entity.NewCertificate("Mocked sem API Key", "Curso Mock", 10, "Mock")
}

func (g *GeminiExtractor) ProviderName() string {
	return "Gemini 1.5 Flash"
}
