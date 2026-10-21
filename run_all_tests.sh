#!/usr/bin/env bash
set -e

echo "=========================================="
echo "🧪 EXECUTANDO SUÍTE COMPLETA DE TESTES"
echo "=========================================="

echo -e "\n1. Testes Unitários Go (Domain, UseCase, AI, OCR):"
go test -v ./...

echo -e "\n2. Testes Python (Geração de Planilhas e Simulação de E-mails / Google Drive):"
python3 -m unittest -v test_planilha.py test_email_fetcher.py

echo -e "\n3. Teste de Build do Binário Go:"
go build -o /tmp/cert_test_bin ./cmd/main.go
rm -f /tmp/cert_test_bin
echo "✅ Build do executável final concluído com sucesso!"

echo -e "\n=========================================="
echo "🎉 TODOS OS TESTES PASSARAM COM SUCESSO!"
echo "=========================================="
