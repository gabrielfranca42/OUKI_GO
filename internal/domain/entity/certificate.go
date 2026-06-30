package entity

import "fmt"

// Certificate representa as informações extraídas da imagem do certificado.
// DDD: Aggregate Root da camada de Domínio.
type Certificate struct {
	StudentName string `json:"student_name"` // Nome do aluno
	Hours       int    `json:"hours"`        // Carga horária em horas
	CourseType  string `json:"course_type"`  // Tipo de curso (Presencial, EAD, Extensão, etc.)
}

// NewCertificate cria um Certificate válido, aplicando regras de negócio.
func NewCertificate(name string, hours int, courseType string) (*Certificate, error) {
	if name == "" {
		return nil, fmt.Errorf("nome do aluno não pode ser vazio")
	}
	if hours <= 0 {
		return nil, fmt.Errorf("carga horária deve ser maior que zero")
	}
	if courseType == "" {
		return nil, fmt.Errorf("tipo de curso não pode ser vazio")
	}
	return &Certificate{
		StudentName: name,
		Hours:       hours,
		CourseType:  courseType,
	}, nil
}
