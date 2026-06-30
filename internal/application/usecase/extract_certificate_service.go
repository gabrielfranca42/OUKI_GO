package usecase

import (
	"context"
	"fmt"

	"certextractor/internal/domain/entity"
	port "certextractor/internal/domain/port"
)

// ExtractCertificateService representa o caso de uso de extração de dados.
// DDD: Application Service que coordena a Infraestrutura (IA/Balanceador) e o Domínio.
type ExtractCertificateService struct {
	extractor port.CertificateExtractor
}

// NewExtractCertificateService cria uma instância do serviço injetando o Extractor (Port).
func NewExtractCertificateService(extractor port.CertificateExtractor) *ExtractCertificateService {
	return &ExtractCertificateService{extractor: extractor}
}

// Execute orquestra a chamada de extração e retorna a Entidade de Domínio.
func (s *ExtractCertificateService) Execute(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	// A responsabilidade de extrair o texto, estruturar os dados ou ler a imagem via visão
	// foi toda abstraída para a camada de infraestrutura via adapter.
	cert, err := s.extractor.ExtractData(ctx, imagePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao extrair dados do certificado na IA: %w", err)
	}

	return cert, nil
}
