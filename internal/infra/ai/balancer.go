package ai

import (
	"context"
	"fmt"
	"sync/atomic"

	"certextractor/internal/domain/entity"
	port "certextractor/internal/domain/port"
)

// ExtractorBalancer implementa port.CertificateExtractor e faz round-robin
type ExtractorBalancer struct {
	providers []port.CertificateExtractor
	counter   uint64
}

func NewExtractorBalancer(providers ...port.CertificateExtractor) *ExtractorBalancer {
	return &ExtractorBalancer{providers: providers}
}

func (b *ExtractorBalancer) ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error) {
	if len(b.providers) == 0 {
		return nil, fmt.Errorf("nenhum provedor de IA configurado no balanceador")
	}

	count := atomic.AddUint64(&b.counter, 1)
	providerIndex := int(count % uint64(len(b.providers)))
	primary := b.providers[providerIndex]
	
	cert, err := primary.ExtractData(ctx, imagePath)
	if err == nil {
		fmt.Printf("[BALANCEADOR SUCESSO] Usando %s\n", primary.ProviderName())
		return cert, nil
	}

	fmt.Printf("[BALANCEADOR FALHA] %s falhou. \n   -> ERRO: %v\nTentando outros...\n", primary.ProviderName(), err)

	// Tenta os outros como Fallback
	for i, provider := range b.providers {
		if i == providerIndex {
			continue
		}
		cert, errFallback := provider.ExtractData(ctx, imagePath)
		if errFallback == nil {
			fmt.Printf("[BALANCEADOR FALLBACK SUCESSO] Usando %s\n", provider.ProviderName())
			return cert, nil
		} else {
			fmt.Printf("[BALANCEADOR FALHA FALLBACK] %s falhou. \n   -> ERRO: %v\n", provider.ProviderName(), errFallback)
		}
	}

	return nil, fmt.Errorf("todos os provedores de IA falharam para a imagem: %s", imagePath)
}

func (b *ExtractorBalancer) ProviderName() string {
	return "Load Balancer"
}
