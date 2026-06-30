package port

import "context"

// Provider define a interface (port) para extração de texto de imagens.
// DDD: Port da camada de Domínio — implementação concreta fica em Infra.
type Provider interface {
	ExtractText(ctx context.Context, imagePath string) (string, error)
}
