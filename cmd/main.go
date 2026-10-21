package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"certextractor/internal/application/usecase"
	"certextractor/internal/domain/entity"
	port "certextractor/internal/domain/port"
	"certextractor/internal/infra/ai"
	"certextractor/internal/infra/local"
	"certextractor/internal/infra/ocr"

	"github.com/joho/godotenv"
)

var mu sync.Mutex

type Falha struct {
	Arquivo string
	Motivo  string
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[OUKI-GO] ")
}

// loadProcessedHashes lê o arquivo e retorna um mapa de hashes para barrar duplicatas
func loadProcessedHashes() map[string]bool {
	hashes := make(map[string]bool)
	file, err := os.Open("dados_extraidos.jsonl")
	if err != nil {
		log.Printf("[INFO] Arquivo 'dados_extraidos.jsonl' ainda não existe. Iniciando base limpa.")
		return hashes
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		var cert entity.Certificate
		if err := json.Unmarshal([]byte(line), &cert); err == nil && cert.Hash != "" {
			hashes[cert.Hash] = true
			count++
		}
	}
	log.Printf("[INFO] %d certificados já processados foram carregados do histórico.", count)
	return hashes
}

func getPythonCommand() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("[WARN] Arquivo .env não encontrado. Utilizando variáveis do ambiente do sistema.")
	} else {
		log.Printf("[INFO] Configurações do arquivo .env carregadas com sucesso.")
	}

	// Instanciar provedores (Clean Architecture / DDD)
	tesseractOCR := ocr.NewTesseractProvider()
	gemini := ai.NewGeminiExtractor()
	groq := ai.NewGroqExtractor(tesseractOCR)
	balancerAI := ai.NewExtractorBalancer(gemini, groq)
	localExtractor := local.NewLocalRegexExtractor(tesseractOCR)

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n====================================================\n")
	fmt.Print("🤖 OUKI GO - SISTEMA DE HORAS COMPLEMENTARES 🤖\n")
	fmt.Print("====================================================\n\n")
	fmt.Println("Selecione o modo de processamento:")
	fmt.Println("  [1] Análise LOCAL (Sem IA - Ilimitado, Ultra-Rápido, Sem Cotas)")
	fmt.Println("  [2] Análise com IA em Nuvem (Gemini / Groq)")
	fmt.Println("  [3] Baixar do E-mail (Gmail / Outlook / Google Drive) e processar")
	fmt.Println("  [4] Apenas regenerar planilhas (XLSX institucional e CSV)")
	fmt.Print("\nOpção (padrão 1): ")

	opcao, _ := reader.ReadString('\n')
	opcao = strings.TrimSpace(opcao)
	if opcao == "" {
		opcao = "1"
	}

	if opcao == "4" {
		gerarPlanilhas()
		return
	}

	var extractor port.CertificateExtractor
	isCloudAI := false

	switch opcao {
	case "2":
		extractor = balancerAI
		isCloudAI = true
		log.Printf("[INFO] Modo selecionado: IA em Nuvem (Gemini + Groq Fallback)")
	case "3":
		extractor = localExtractor
		log.Printf("[INFO] Modo selecionado: Coleta por E-mail + Extração Local")
	default:
		extractor = localExtractor
		log.Printf("[INFO] Modo selecionado: Extração Local Heurística (Sem IA - Ilimitado)")
	}

	service := usecase.NewExtractCertificateService(extractor)

	var dirPath string

	if opcao == "3" {
		log.Printf("[INFO] Iniciando conector de e-mail (anexos e links do Google Drive)...")
		pyCmd := getPythonCommand()
		cmd := exec.Command(pyCmd, "email_fetcher.py", "--output", "downloads_certificados")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		startEmailTime := time.Now()
		if err := cmd.Run(); err != nil {
			log.Printf("[ERRO] Falha crítica ao executar o conector de e-mails: %v", err)
			log.Printf("[DICA] Verifique se as credenciais estão corretas no .env")
			return
		}
		log.Printf("[INFO] Coleta de e-mails concluída em %v", time.Since(startEmailTime))
		dirPath = "downloads_certificados"
	} else {
		fmt.Print("\nDigite o caminho da pasta com os certificados: ")
		dirPath, _ = reader.ReadString('\n')
		dirPath = strings.TrimSpace(dirPath)
	}

	if dirPath == "" {
		log.Printf("[ERRO] Caminho do diretório não pode ser vazio.")
		return
	}

	var imagensValidas []string
	validExtensions := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".pdf": true}

	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if validExtensions[ext] {
				imagensValidas = append(imagensValidas, path)
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("[ERRO] Erro ao varrer diretório '%s': %v", dirPath, err)
		return
	}

	if len(imagensValidas) == 0 {
		log.Printf("[AVISO] Nenhum certificado válido (.pdf, .png, .jpg, .webp) encontrado na pasta '%s'.", dirPath)
		return
	}

	log.Printf("[INFO] Encontrados %d arquivos válidos para processamento na pasta e subpastas de '%s'.", len(imagensValidas), dirPath)

	// Configuração de limite/cota
	cotaMaxima := 0
	if maxStr := os.Getenv("MAX_CERTIFICATES"); maxStr != "" {
		if parsed, err := strconv.Atoi(maxStr); err == nil && parsed > 0 {
			cotaMaxima = parsed
			log.Printf("[INFO] Limite configurado: máximo de %d certificados nesta execução.", cotaMaxima)
		}
	}

	var sucessos []string
	var falhas []Falha

	processedHashes := loadProcessedHashes()
	processados := 0
	totalStartTime := time.Now()

	for _, imgPath := range imagensValidas {
		if cotaMaxima > 0 && processados >= cotaMaxima {
			log.Printf("[AVISO] Limite configurado de %d certificados atingido. Interrompendo lote.", cotaMaxima)
			break
		}

		baseName := filepath.Base(imgPath)
		currentSubdir := filepath.Dir(imgPath)
		log.Printf("---------------------------------------------------------")
		log.Printf("[PROCESSO] Início da extração: %s (%d/%d)", baseName, processados+1, len(imagensValidas))

		// Limpa herança de lote global para não contaminar pastas diferentes
		localExtractor.SetDefaultStudentName("")

		itemStart := time.Now()
		cert, err := service.Execute(context.Background(), imgPath)
		itemDuration := time.Since(itemStart)

		if err != nil {
			log.Printf("[FALHA] '%s' falhou após %v. Motivo: %v", baseName, itemDuration, err)
			falhas = append(falhas, Falha{Arquivo: baseName, Motivo: err.Error()})
		} else {
			if processedHashes[cert.Hash] {
				motivoDup := fmt.Sprintf("Certificado Duplicado: Aluno '%s' já possui registro idêntico para '%s' (%s).",
					cert.StudentName, cert.CourseName, cert.Category)
				log.Printf("[DUPLICATA] %s", motivoDup)
				falhas = append(falhas, Falha{Arquivo: baseName, Motivo: motivoDup})
			} else {
				processedHashes[cert.Hash] = true
				salvarResultadoNoDisco(cert)
				log.Printf("[SUCESSO] '%s' concluído em %v -> Aluno: %s | Horas: %d declaradas / %d aproveitadas | Semestre: %s",
					baseName, itemDuration, cert.StudentName, cert.DeclaredHours, cert.ApprovedHours, cert.Semester)

				_ = currentSubdir

				sucessos = append(sucessos, fmt.Sprintf("%s | %s (%d/%d hrs, %s)",
					cert.StudentName, cert.CourseName, cert.ApprovedHours, cert.DeclaredHours, cert.Semester))
			}
		}

		processados++

		// Se estiver usando IA em nuvem, aplica pausa de 10s para não estourar a cota gratuita
		if isCloudAI && processados < len(imagensValidas) {
			log.Printf("[RATE-LIMIT] Aguardando 10 segundos para respeitar a cota da API do Gemini (Free Tier)...")
			time.Sleep(10 * time.Second)
		}
	}

	log.Printf("=========================================================")
	log.Printf("[RESUMO] Execução total finalizada em %v", time.Since(totalStartTime))
	log.Printf("Total processados: %d | Sucessos: %d | Falhas/Recusas: %d", processados, len(sucessos), len(falhas))

	// Relatório Final em Tela
	fmt.Print("\n================ RELATÓRIO FINAL ================\n")
	if len(sucessos) > 0 {
		fmt.Printf("✅ SUCESSOS (%d):\n", len(sucessos))
		for _, s := range sucessos {
			fmt.Printf("   - %s\n", s)
		}
	} else {
		fmt.Println("✅ SUCESSOS: 0")
	}

	if len(falhas) > 0 {
		fmt.Printf("\n❌ RECUSADOS / FALHAS (%d):\n", len(falhas))
		for _, f := range falhas {
			fmt.Printf("   - %s\n     Motivo: %s\n", f.Arquivo, f.Motivo)
		}
		salvarLogFalhas(falhas)
	} else {
		fmt.Println("\n❌ RECUSADOS: 0")
	}

	// Gera ou atualiza as planilhas
	if len(sucessos) > 0 {
		gerarPlanilhas()
	}

	fmt.Println("=================================================")
}

func salvarLogFalhas(falhas []Falha) {
	logFileName := "certificados_nao_lidos.txt"
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[ERRO] Falha ao abrir '%s': %v", logFileName, err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("\n========== RELATÓRIO DE FALHAS [%s] ==========\n", timestamp))
	for _, fail := range falhas {
		f.WriteString(fmt.Sprintf("[%s] Arquivo: %s | Motivo: %s\n", timestamp, fail.Arquivo, fail.Motivo))
	}
	f.WriteString("=================================================================\n")
	log.Printf("[LOG] Arquivo de erros atualizado com sucesso: %s (%d falhas registradas)", logFileName, len(falhas))
}

func gerarPlanilhas() {
	log.Printf("[INFO] Disparando gerador de planilhas formatadas (.xlsx e .csv)...")
	pyCmd := getPythonCommand()
	cmd := exec.Command(pyCmd, "gerar_planilha.py")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERRO] Falha ao executar gerar_planilha.py: %v\nSaída do script: %s", err, string(output))
	} else {
		fmt.Printf("\n%s\n", string(output))
	}
}

func salvarResultadoNoDisco(cert *entity.Certificate) {
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile("dados_extraidos.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[ERRO] Não foi possível abrir 'dados_extraidos.jsonl' para gravação: %v", err)
		return
	}
	defer f.Close()

	bytes, err := json.Marshal(cert)
	if err != nil {
		log.Printf("[ERRO] Falha ao serializar entidade de certificado em JSON: %v", err)
		return
	}
	f.Write(bytes)
	f.WriteString("\n")
	log.Printf("[DISCO] Registro salvo com sucesso no banco de dados JSONL (Hash: %s)", cert.Hash)
}
