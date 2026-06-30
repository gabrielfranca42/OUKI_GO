package port

import (
	"context"

	"certextractor/internal/domain/entity"
)

// CertificateExtractor define a interface para extração estruturada do certificado pelas IAs.
type CertificateExtractor interface {
	ExtractData(ctx context.Context, imagePath string) (*entity.Certificate, error)
	ProviderName() string
}
