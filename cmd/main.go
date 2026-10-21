package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"certextractor/internal/application/usecase"
	"certextractor/internal/domain/entity"
	"certextractor/internal/infra/ai"
	"certextractor/internal/infra/ocr"

	"github.com/joho/godotenv"
)

var mu sync.Mutex

// loadProcessedHashes lê o arquivo e retorna um mapa de hashes para barrar duplicatas
func loadProcessedHashes() map[string]bool {
	hashes := make(map[string]bool)
	file, err := os.Open("dados_extraidos.jsonl")
	if err != nil {
		return hashes
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		var cert entity.Certificate
		if err := json.Unmarshal([]byte(line), &cert); err == nil && cert.Hash != "" {
			hashes[cert.Hash] = true
		}
	}
	return hashes
}

func getPythonCommand() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

func main() {
	_ = godotenv.Load()

	// 1. Instanciar dependências (DDD)
	tesseractOCR := ocr.NewTesseractProvider()
	gemini := ai.NewGeminiExtractor()
	groq := ai.NewGroqExtractor(tesseractOCR)
	balancer := ai.NewExtractorBalancer(gemini, groq)
	service := usecase.NewExtractCertificateService(balancer)

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("====================================================\n")
	fmt.Print("🤖 OUKI GO - SISTEMA DE HORAS COMPLEMENTARES COM IA 🤖\n")
	fmt.Print("====================================================\n\n")
	fmt.Println("Selecione o modo de operação:")
	fmt.Println("  [1] Processar certificados de uma pasta local")
	fmt.Println("  [2] Baixar do Outlook Corporativo (Anexos + Google Drive) e processar")
	fmt.Println("  [3] Apenas regenerar planilhas (XLSX institucional e CSV)")
	fmt.Print("\nOpção (padrão 1): ")

	opcao, _ := reader.ReadString('\n')
	opcao = strings.TrimSpace(opcao)
	if opcao == "" {
		opcao = "1"
	}

	if opcao == "3" {
		gerarPlanilhas()
		return
	}

	var dirPath string

	if opcao == "2" {
		fmt.Println("\n📥 Conectando ao Outlook corporativo e buscando e-mails...")
		pyCmd := getPythonCommand()
		cmd := exec.Command(pyCmd, "email_fetcher.py", "--output", "downloads_certificados")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("\n[ERRO] Falha ao executar o conector do Outlook: %v\n", err)
			fmt.Println("Dica: verifique se OUTLOOK_EMAIL e OUTLOOK_PASSWORD estão configurados no .env")
			return
		}
		dirPath = "downloads_certificados"
	} else {
		fmt.Print("\nDigite o caminho da pasta com os certificados: ")
		dirPath, _ = reader.ReadString('\n')
		dirPath = strings.TrimSpace(dirPath)
	}

	if dirPath == "" {
		fmt.Println("\n[ERRO] Caminho não pode ser vazio.")
		return
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		fmt.Printf("\n[ERRO] Não foi possível ler a pasta '%s': %v\n", dirPath, err)
		return
	}

	var imagensValidas []string
	validExtensions := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".pdf": true}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name()))
		if validExtensions[ext] {
			imagensValidas = append(imagensValidas, filepath.Join(dirPath, file.Name()))
		}
	}

	if len(imagensValidas) == 0 {
		fmt.Println("\n[AVISO] Nenhum certificado (imagem ou PDF) válido encontrado nesta pasta.")
		return
	}

	fmt.Printf("\n[INFO] Encontrados %d certificados para processamento.\n", len(imagensValidas))

	// Configuração de limite/cota (0 ou vazio = sem limite)
	cotaMaxima := 0
	if maxStr := os.Getenv("MAX_CERTIFICATES"); maxStr != "" {
		if parsed, err := strconv.Atoi(maxStr); err == nil {
			cotaMaxima = parsed
		}
	}

	var sucessos []string
	type Falha struct {
		Arquivo string
		Motivo  string
	}
	var falhas []Falha

	processedHashes := loadProcessedHashes()
	processados := 0

	for _, imgPath := range imagensValidas {
		if cotaMaxima > 0 && processados >= cotaMaxima {
			fmt.Printf("\n[AVISO] Limite de cota atingido (%d certificados). Pausando extração.\n", cotaMaxima)
			break
		}

		baseName := filepath.Base(imgPath)
		fmt.Printf("\n⏳ Processando: %s ...\n", baseName)

		cert, err := service.Execute(context.Background(), imgPath)
		if err != nil {
			falhas = append(falhas, Falha{Arquivo: baseName, Motivo: err.Error()})
		} else {
			if processedHashes[cert.Hash] {
				falhas = append(falhas, Falha{Arquivo: baseName, Motivo: "Certificado Duplicado: Este aluno já tem este curso/carga horária registrado."})
			} else {
				processedHashes[cert.Hash] = true
				salvarResultadoNoDisco(cert)
				sucessos = append(sucessos, fmt.Sprintf("%s | %s (%d/%d hrs, %s)",
					cert.StudentName, cert.CourseName, cert.ApprovedHours, cert.DeclaredHours, cert.Semester))
			}
		}
		processados++
	}

	// Relatório Final
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
	} else {
		fmt.Println("\n❌ RECUSADOS: 0")
	}

	// Gera as planilhas formatadas caso haja novos sucessos ou dados
	if len(sucessos) > 0 {
		gerarPlanilhas()
	}

	fmt.Println("=================================================")
}

func gerarPlanilhas() {
	fmt.Println("\n[INFO] Gerando planilha Excel formatada (modelo institucional) e CSV...")
	pyCmd := getPythonCommand()
	cmd := exec.Command(pyCmd, "gerar_planilha.py")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[ERRO PLANILHA] %v\nSaída: %s\n", err, string(output))
	} else {
		fmt.Printf("%s\n", string(output))
	}
}

func salvarResultadoNoDisco(cert *entity.Certificate) {
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
