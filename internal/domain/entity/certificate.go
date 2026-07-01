package entity

import "fmt"

// Certificate representa as informações extraídas da imagem do certificado.
// DDD: Aggregate Root da camada de Domínio.
type Certificate struct {
	StudentName    string `json:"student_name"` // Nome do aluno
	CourseName     string `json:"course_name"`  // Nome do curso
	Hours          int    `json:"hours"`        // Carga horária em horas
	CourseType     string `json:"course_type"`  // Tipo de curso (Presencial, EAD, etc)
	CompletionDate string `json:"completion_date"` // Data de conclusão (DD/MM/AAAA)
	Category       string `json:"category"`     // Categoria (Curso, Palestra, Evento, Estágio...)
	Hash           string `json:"hash"`         // Hash único para evitar duplicidade
}

// NewCertificate cria um Certificate válido, aplicando regras de negócio.
func NewCertificate(name string, courseName string, hours int, courseType string, completionDate string, category string) (*Certificate, error) {
	if name == "" {
		return nil, fmt.Errorf("nome do aluno não pode ser vazio")
	}
	if hours <= 0 {
		return nil, fmt.Errorf("carga horária deve ser maior que zero")
	}
	if courseType == "" {
		return nil, fmt.Errorf("tipo de curso não pode ser vazio")
	}
	if courseName == "" {
		return nil, fmt.Errorf("nome do curso não pode ser vazio")
	}

	// Gera um hash simples baseado em Aluno + Curso + Horas para identificar duplicidade
	hashInput := fmt.Sprintf("%s|%s|%d", name, courseName, hours)
	
	return &Certificate{
		StudentName:    name,
		CourseName:     courseName,
		Hours:          hours,
		CourseType:     courseType,
		CompletionDate: completionDate,
		Category:       category,
		Hash:           hashInput, // (Para produção poderíamos usar SHA256 aqui)
	}, nil
}
