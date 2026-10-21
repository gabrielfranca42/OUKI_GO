package local

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"certextractor/internal/domain/entity"
	port "certextractor/internal/domain/port"
	"github.com/ledongthuc/pdf"
)

// LocalRegexExtractor implementa port.CertificateExtractor sem usar nenhuma IA de nuvem.
// Extrai texto diretamente de PDFs e imagens e analisa usando Heurística e Regex.
type LocalRegexExtractor struct {
	ocrProvider        port.Provider
	defaultStudentName string
}

// NewLocalRegexExtractor instancia o extrator local
func NewLocalRegexExtractor(ocrProvider port.Provider) *LocalRegexExtractor {
	return &LocalRegexExtractor{
		ocrProvider: ocrProvider,
	}
}

func (l *LocalRegexExtractor) ExtractData(ctx context.Context, filePath string) (*entity.Certificate, error) {
	log.Printf("[LocalExtractor][INFO] Iniciando análise local (Sem IA) para: %s", filepath.Base(filePath))
	startTime := time.Now()

	text, err := l.extractRawText(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao extrair texto do documento: %w", err)
	}

	cleanText := strings.TrimSpace(text)
	if cleanText == "" {
		return nil, fmt.Errorf("nenhum texto legível encontrado no documento (pode ser imagem digitalizada sem OCR)")
	}

	log.Printf("[LocalExtractor][INFO] Texto extraído com sucesso (%d caracteres). Aplicando heurística...", len(cleanText))

	studentName := l.extractStudentName(cleanText, filePath)
	if studentName == "" {
		return nil, fmt.Errorf("não foi possível identificar o nome do aluno no certificado e nenhum aluno padrão foi definido para o lote")
	}

	courseName := l.extractCourseName(cleanText, filePath)
	hours := l.extractHours(cleanText)
	completionDate := l.extractDate(cleanText)
	category := l.extractCategory(cleanText, courseName)
	courseType := l.extractCourseType(cleanText)

	log.Printf("[LocalExtractor][DADOS] Aluno='%s', Curso='%s', Horas=%d, Data='%s', Categoria='%s'",
		studentName, courseName, hours, completionDate, category)

	cert, err := entity.NewCertificate(
		studentName,
		courseName,
		hours,
		courseType,
		completionDate,
		category,
	)
	if err != nil {
		return nil, fmt.Errorf("regras de validação do certificado não atendidas: %w", err)
	}

	log.Printf("[LocalExtractor][SUCESSO] Processado em %v -> Declaradas: %d | Aproveitadas: %d | Semestre: %s",
		time.Since(startTime), cert.DeclaredHours, cert.ApprovedHours, cert.Semester)

	return cert, nil
}

func (l *LocalRegexExtractor) extractRawText(ctx context.Context, filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	// 1. Se for PDF, tenta ler nativamente as camadas de texto do PDF
	if ext == ".pdf" {
		pdfText, err := readPdfText(filePath)
		if err == nil && len(strings.TrimSpace(pdfText)) > 20 {
			return pdfText, nil
		}
		log.Printf("[LocalExtractor][WARN] PDF não possui camada de texto direto (pode ser escaneado). Tentando OCR...")
	}

	// 2. Se for imagem ou PDF escaneado, usa o OCR do Tesseract se disponível
	if l.ocrProvider != nil {
		return l.ocrProvider.ExtractText(ctx, filePath)
	}

	return "", fmt.Errorf("nenhum método disponível para ler o texto de %s", filePath)
}

func readPdfText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	_, err = buf.ReadFrom(b)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// SetDefaultStudentName define um nome de aluno padrão herdado (por exemplo, do lote/pasta)
func (l *LocalRegexExtractor) SetDefaultStudentName(name string) {
	l.defaultStudentName = strings.TrimSpace(name)
}

func (l *LocalRegexExtractor) GetDefaultStudentName() string {
	return l.defaultStudentName
}

func (l *LocalRegexExtractor) extractStudentName(text string, filePath string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:certificamos\s+que|declaramos\s+que|confere\s+a\s+presente\s+a|atesta\s+que|certifico\s+que|declaramos\s+para\s+os\s+devidos\s+fins\s+que|certifica\s+que|conferido\s+a|outorgado\s+a)\s+([A-ZÀ-Ú][a-zà-úA-ZÀ-Ú\s]{3,60}?)(?:,|\s+com\s+cpf|\s+sob\s+o\s+cpf|\s+concluiu|\s+participou|\s+cumpriu|\s+portador|\s+rg|\.|\n|\r)`),
		regexp.MustCompile(`(?i)(?:aluno|participante|outorgado|estudante)\s*:\s*([A-ZÀ-Ú][a-zà-úA-ZÀ-Ú\s]{3,60}?)(?:\n|\r|,|\.)`),
		regexp.MustCompile(`(?i)(?:concedido\s+a|emitido\s+para)\s+([A-ZÀ-Ú][a-zà-úA-ZÀ-Ú\s]{3,60}?)(?:,|\s+com|\s+por|\.|\n|\r)`),
	}

	for _, p := range patterns {
		if matches := p.FindStringSubmatch(text); len(matches) > 1 {
			name := strings.TrimSpace(matches[1])
			name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
			nameLower := strings.ToLower(name)
			if len(name) >= 3 && !strings.Contains(nameLower, "carga") && !strings.Contains(nameLower, "curso") && !strings.Contains(nameLower, "senac") {
				return name
			}
		}
	}

	// Fallback 1: Procura linha em caixa alta após frases clássicas ("Certificamos que", "Conferido a")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "certificamos que") || strings.Contains(lower, "declaramos que") || strings.Contains(lower, "conferido a") {
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				cand := strings.TrimSpace(lines[j])
				// Se for um nome próprio de 2 a 5 palavras (ex: LUCAS GABRIEL DA SILVA SANTANA)
				words := strings.Fields(cand)
				if len(words) >= 2 && len(words) <= 7 && len(cand) >= 5 && len(cand) <= 60 {
					candLower := strings.ToLower(cand)
					if !strings.Contains(candLower, "curso") && !strings.Contains(candLower, "carga") && !strings.Contains(candLower, "horas") && !strings.Contains(candLower, "senac") {
						return cand
					}
				}
			}
		}
	}

	// Fallback 2: Se já temos um aluno identificado no lote/pasta, herda para unificar
	if l.defaultStudentName != "" {
		log.Printf("[LocalExtractor][HERANÇA] Nome do aluno não explicitado no certificado. Herdando aluno do lote: '%s'", l.defaultStudentName)
		return l.defaultStudentName
	}

	// Fallback 3: Tenta usar o nome da pasta onde o certificado está localizado (ex: "CERTIFICADO/Lucas_Santana/cert.pdf" -> "Lucas_Santana")
	parentDir := filepath.Base(filepath.Dir(filePath))
	parentLower := strings.ToLower(parentDir)
	genericNames := map[string]bool{
		"certificado": true, "certificados": true, "downloads": true, "download": true,
		"documents": true, "documentos": true, "desktop": true, ".": true, "/": true, "\\": true,
		"downloads_certificados": true,
	}

	if parentDir != "" && !genericNames[parentLower] {
		// Substitui sublinhados por espaços e limpa
		nomeFormatado := strings.ReplaceAll(parentDir, "_", " ")
		nomeFormatado = strings.TrimSpace(nomeFormatado)
		if len(nomeFormatado) >= 3 {
			log.Printf("[LocalExtractor][PASTA] Nome do aluno extraído do nome da subpasta: '%s'", nomeFormatado)
			return nomeFormatado
		}
	}

	// Se não achou nenhum e não tem aluno padrão no lote ou subpasta, não inventa nome pelo arquivo
	return ""
}

func (l *LocalRegexExtractor) extractCourseName(text string, filePath string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:concluiu\s+(?:com\s+[êe]xito\s+)?(?:o\s+)?(?:curso\s+(?:de\s+)?)?|participou\s+(?:do\s+)?(?:workshop\s+(?:de\s+)?)?|participa[çc][ãa]o\s+no\s+|atividades\s+de\s+|capacita[çc][ãa]o\s+em\s+)(["“']?[A-ZÀ-Úa-zà-ú0-9\s\-\.\+/#]{4,80}?["”']?)(?:,|\s+com\s+carga|\s+no\s+per[íi]odo|\s+realizado|\.|\n|\r)`),
		regexp.MustCompile(`(?i)(?:curso|evento|workshop|treinamento)\s*:\s*([A-ZÀ-Úa-zà-ú0-9\s\-\.\+/#]{4,80}?)(?:\n|\r|,|\.)`),
	}

	for _, p := range patterns {
		if matches := p.FindStringSubmatch(text); len(matches) > 1 {
			course := strings.TrimSpace(matches[1])
			course = strings.Trim(course, `"'“”) `)
			if len(course) >= 4 && !strings.Contains(strings.ToLower(course), "carga horária") {
				return course
			}
		}
	}

	// Fallback para estágio
	if strings.Contains(strings.ToLower(text), "estágio") || strings.Contains(strings.ToLower(text), "estagio") {
		return "Estágio Supervisionado"
	}

	// Fallback do nome do arquivo
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return "Atividade: " + strings.TrimSpace(base)
}

func (l *LocalRegexExtractor) extractHours(text string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:carga\s+hor[áa]ria(?:\s+de)?|totalizando|dura[çc][ãa]o(?:\s+de)?)\s*:?\s*(\d{1,4})\s*(?:h|horas|hrs?)`),
		regexp.MustCompile(`(?i)(\d{1,3})\s*(?:horas|hrs|h)\b`),
	}

	for _, p := range patterns {
		if matches := p.FindStringSubmatch(text); len(matches) > 1 {
			if h, err := strconv.Atoi(matches[1]); err == nil && h > 0 {
				return h
			}
		}
	}

	// Se for estágio e não tiver horas explícitas, padrão é 20
	if strings.Contains(strings.ToLower(text), "estágio") || strings.Contains(strings.ToLower(text), "estagio") {
		return 20
	}

	return 20 // Padrão seguro institucional
}

func (l *LocalRegexExtractor) extractDate(text string) string {
	// Formato numérico DD/MM/AAAA
	reNum := regexp.MustCompile(`(\d{1,2})[\/\.-](\d{1,2})[\/\.-](\d{4})`)
	if matches := reNum.FindStringSubmatch(text); len(matches) == 4 {
		day, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		year := matches[3]
		return fmt.Sprintf("%02d/%02d/%s", day, month, year)
	}

	// Formato por extenso (ex: 15 de outubro de 2024)
	reExt := regexp.MustCompile(`(?i)(\d{1,2})\s+de\s+(janeiro|fevereiro|mar[çc]o|abril|maio|junho|julho|agosto|setembro|outubro|novembro|dezembro)\s+de\s+(\d{4})`)
	if matches := reExt.FindStringSubmatch(text); len(matches) == 4 {
		day, _ := strconv.Atoi(matches[1])
		meses := map[string]int{
			"janeiro": 1, "fevereiro": 2, "março": 3, "marco": 3, "abril": 4, "maio": 5, "junho": 6,
			"julho": 7, "agosto": 8, "setembro": 9, "outubro": 10, "novembro": 11, "dezembro": 12,
		}
		month := meses[strings.ToLower(matches[2])]
		year := matches[3]
		return fmt.Sprintf("%02d/%02d/%s", day, month, year)
	}

	return time.Now().Format("02/01/2006")
}

func (l *LocalRegexExtractor) extractCategory(text string, courseName string) string {
	combined := strings.ToLower(text + " " + courseName)

	if strings.Contains(combined, "estágio") || strings.Contains(combined, "estagio") {
		return "Estágio"
	}
	if strings.Contains(combined, "palestra") {
		return "Palestra"
	}
	if strings.Contains(combined, "workshop") || strings.Contains(combined, "seminário") || strings.Contains(combined, "evento") {
		return "Evento"
	}
	if strings.Contains(combined, "extensão") || strings.Contains(combined, "extensao") {
		return "Projeto de Extensão"
	}

	return "Curso"
}

func (l *LocalRegexExtractor) extractCourseType(text string) string {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "ead") || strings.Contains(lower, "online") || strings.Contains(lower, "distância") {
		return "EAD"
	}
	if strings.Contains(lower, "presencial") {
		return "Presencial"
	}
	return "Online"
}

func (l *LocalRegexExtractor) ProviderName() string {
	return "Extrator Local Heurístico (Sem IA - Ilimitado)"
}
