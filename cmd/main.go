package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"certextractor/internal/application/usecase"
	"certextractor/internal/infra/ai"
	"certextractor/internal/infra/ocr"
)

var mu sync.Mutex

func main() {
	// 1. Instanciar dependências (DDD)
	tesseractOCR := ocr.NewTesseractProvider()
	gemini := ai.NewGeminiExtractor()
	groq := ai.NewGroqExtractor(tesseractOCR)
	balancer := ai.NewExtractorBalancer(gemini, groq)
	service := usecase.NewExtractCertificateService(balancer)

	// 2. Interface Interativa via Terminal
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("=========================================\n")
	fmt.Print("🤖 EXTRATOR DE CERTIFICADOS COM IA 🤖\n")
	fmt.Print("=========================================\n\n")
	fmt.Print("Digite o caminho completo da pasta com os certificados: ")

	dirPath, _ := reader.ReadString('\n')
	dirPath = strings.TrimSpace(dirPath) // Limpa quebras de linha ou espaços

	if dirPath == "" {
		fmt.Println("\n[ERRO] Caminho não pode ser vazio.")
		return
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		fmt.Printf("\n[ERRO] Não foi possível ler a pasta. Verifique se o caminho está correto.\nDetalhe: %v\n", err)
		return
	}

	var imagensValidas []string
	// Aceitando imagens (e PDF se a IA de visão for configurada para ler, mas o Tesseract precisa de imagem)
	validExtensions := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

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
		fmt.Println("\n[AVISO] Nenhuma imagem válida (PNG, JPG, JPEG) foi encontrada nesta pasta.")
		return
	}

	fmt.Printf("\n[INFO] Encontrados %d certificados válidos para processamento.\n", len(imagensValidas))

	var sucessos []string
	type Falha struct {
		Arquivo string
		Motivo  string
	}
	var falhas []Falha

	// Você pediu um limite de cota igual a 2.
	cotaMaxima := 2
	processados := 0

	for _, imgPath := range imagensValidas {
		if processados >= cotaMaxima {
			fmt.Printf("\n[AVISO] Limite de cota atingido (%d certificados). Pausando extração.\n", cotaMaxima)
			break
		}

		fmt.Printf("\n⏳ Processando: %s ...\n", filepath.Base(imgPath))
		
		cert, err := service.Execute(context.Background(), imgPath)
		if err != nil {
			falhas = append(falhas, Falha{Arquivo: filepath.Base(imgPath), Motivo: err.Error()})
		} else {
			salvarResultadoNoDisco(cert)
			sucessos = append(sucessos, filepath.Base(imgPath))
		}
		processados++
	}

	// ==========================================
	// RELATÓRIO FINAL EXPLÍCITO
	// ==========================================
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

	// Se houver algum sucesso, roda o Python para exportar pra CSV
	if len(sucessos) > 0 {
		fmt.Println("\n[INFO] Gerando planilha CSV usando Python...")
		cmd := exec.Command("python", "gerar_planilha.py")
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("[ERRO PYTHON] Não foi possível gerar CSV: %v\nSaída: %s\n", err, string(output))
		} else {
			fmt.Printf("%s\n", string(output))
		}
	}
	
	fmt.Println("=================================================")
}

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
