package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

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
	IsCertificate  bool   `json:"is_certificate"`
	StudentName    string `json:"student_name"`
	CourseName     string `json:"course_name"`
	Hours          int    `json:"hours"`
	CourseType     string `json:"course_type"`
	CompletionDate string `json:"completion_date"`
	Category       string `json:"category"`
	Reason         string `json:"reason"`
}

func (g *GeminiExtractor) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Printf("[Gemini][WARN] GEMINI_API_KEY não encontrada no .env. Ativando modo MOCK.")
		return g.mockFallback(imagePath)
	}

	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		log.Printf("[Gemini][ERRO] Falha ao ler arquivo no disco: %s (Erro: %v)", imagePath, err)
		return nil, fmt.Errorf("erro ao ler arquivo de imagem: %w", err)
	}

	var filePart genai.Part
	lowPath := strings.ToLower(imagePath)
	detectedMime := "jpeg"

	if strings.HasSuffix(lowPath, ".pdf") {
		detectedMime = "application/pdf"
		filePart = genai.Blob{MIMEType: "application/pdf", Data: imgData}
	} else {
		if strings.HasSuffix(lowPath, ".png") {
			detectedMime = "png"
		} else if strings.HasSuffix(lowPath, ".webp") {
			detectedMime = "webp"
		}
		filePart = genai.ImageData(detectedMime, imgData)
	}

	log.Printf("[Gemini][INFO] Arquivo carregado: %s (%d bytes, formato: %s)", imagePath, len(imgData), detectedMime)

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Printf("[Gemini][ERRO] Falha ao inicializar cliente da API Google: %v", err)
		return nil, fmt.Errorf("falha ao criar cliente Gemini: %w", err)
	}
	defer client.Close()

	// Usando modelo ativo no Google AI Studio
	modelName := "gemini-flash-latest"
	model := client.GenerativeModel(modelName)
	model.ResponseMIMEType = "application/json"

	prompt := `
Você é um especialista rigoroso em analisar certificados educacionais.
Analise a imagem enviada.
REGRAS CRÍTICAS:
1. Se a imagem NÃO for um certificado, diploma ou declaração de horas (ex: foto de comida, tomate, paisagem, pessoas, print de meme), defina "is_certificate" como false e preencha o campo "reason" explicando o que a imagem realmente é.
2. Se FOR de fato um certificado, defina "is_certificate" como true, e extraia:
   - "student_name" (Nome completo do aluno)
   - "course_name" (O nome do curso realizado)
   - "hours" (A carga horária apenas em número inteiro, ex: 40)
   - "course_type" (O tipo de curso: EAD, Presencial, Workshop, etc)
   - "completion_date" (A data de conclusão ou emissão do certificado. Se encontrar, retorne no formato DD/MM/AAAA)
   - "category" (Categoria do certificado, DEVE SER UMA DESTAS: "Curso", "Palestra", "Evento", "Projeto de Extensão", "Estágio", ou "Outros")
Retorne APENAS o JSON válido seguindo essas chaves, sem nenhuma outra formatação.
`

	log.Printf("[Gemini][INFO] Enviando requisição para o modelo '%s'...", modelName)
	startTime := time.Now()

	resp, err := model.GenerateContent(ctx, genai.Text(prompt), filePart)
	duration := time.Since(startTime)

	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "Quota exceeded") {
			log.Printf("[Gemini][ERRO 429] Limite de requisições por minuto da API do Gemini atingido! Detalhe: %v", err)
			log.Printf("[Gemini][DICA] A API gratuita do Google possui limite de ~5 req/min. Aguarde alguns segundos e tente novamente.")
		} else {
			log.Printf("[Gemini][ERRO] Falha na comunicação com a API: %v", err)
		}
		return nil, fmt.Errorf("falha na API do Gemini: %w", err)
	}

	log.Printf("[Gemini][INFO] Resposta recebida com sucesso em %v", duration)

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		log.Printf("[Gemini][ERRO] Resposta da API retornou vazia (nenhum candidato)")
		return nil, fmt.Errorf("gemini retornou resposta vazia")
	}

	var jsonStr string
	if txt, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		jsonStr = string(txt)
	} else {
		log.Printf("[Gemini][ERRO] Conteúdo da resposta não é texto legível")
		return nil, fmt.Errorf("resposta em formato não reconhecido")
	}

	var resposta RespostaIA
	if err := json.Unmarshal([]byte(jsonStr), &resposta); err != nil {
		log.Printf("[Gemini][ERRO] Falha ao decodificar JSON retornado: %v (Conteúdo: %s)", err, jsonStr)
		return nil, fmt.Errorf("falha ao decodificar JSON do Gemini: %w", err)
	}

	// 1ª Barreira: Decisão da IA
	if !resposta.IsCertificate {
		log.Printf("[Gemini][RECUSA] IA rejeitou o documento. Motivo informado: %s", resposta.Reason)
		return nil, fmt.Errorf("A IA recusou a imagem. Motivo: %s", resposta.Reason)
	}

	log.Printf("[Gemini][SUCESSO] Dados brutos extraídos: Aluno='%s', Curso='%s', Horas=%d, Data='%s', Categoria='%s'",
		resposta.StudentName, resposta.CourseName, resposta.Hours, resposta.CompletionDate, resposta.Category)

	// 2ª Barreira: Validação de Regras de Negócio
	cert, err := entity.NewCertificate(
		resposta.StudentName,
		resposta.CourseName,
		resposta.Hours,
		resposta.CourseType,
		resposta.CompletionDate,
		resposta.Category,
	)
	if err != nil {
		log.Printf("[Gemini][ERRO-DOMINIO] Certificado não atendeu às regras de domínio: %v", err)
		return nil, err
	}

	log.Printf("[Gemini][REGRA-APLICADA] Horas Declaradas: %d -> Horas Aproveitadas: %d | Semestre: %s",
		cert.DeclaredHours, cert.ApprovedHours, cert.Semester)

	return cert, nil
}

func (g *GeminiExtractor) mockFallback(imagePath string) (*entity.Certificate, error) {
	log.Printf("[Gemini][MOCK] Processamento simulado ativado para: %s", imagePath)
	return entity.NewCertificate("Aluno Mock Teste", "Curso Simulado de Demonstração", 10, "EAD", "01/01/2026", "Curso")
}

func (g *GeminiExtractor) ProviderName() string {
	return "Gemini 1.5 Flash"
}
