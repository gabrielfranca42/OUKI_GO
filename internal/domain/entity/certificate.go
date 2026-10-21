package entity

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Certificate representa as informações extraídas do certificado educacional.
// DDD: Aggregate Root da camada de Domínio.
type Certificate struct {
	StudentName    string `json:"student_name"`    // Nome do aluno
	CourseName     string `json:"course_name"`     // Descrição da Atividade / Título do Certificado
	Hours          int    `json:"hours"`           // Carga horária aproveitada (mantido para compatibilidade)
	DeclaredHours  int    `json:"declared_hours"`  // Carga horária declarada
	ApprovedHours  int    `json:"approved_hours"`  // Carga horária aproveitada (máx 20h; estágio = 20h)
	CourseType     string `json:"course_type"`     // Tipo de curso (Presencial, EAD, etc)
	CompletionDate string `json:"completion_date"` // Data de conclusão (DD/MM/AAAA)
	Semester       string `json:"semester"`        // Semestre calculado (ex: 2024.1, 2024.2)
	Category       string `json:"category"`        // Categoria (Curso, Palestra, Evento, Estágio...)
	Hash           string `json:"hash"`            // Hash único para evitar duplicidade
}

// CalculateSemester infere o semestre acadêmico (ex: 2024.1 ou 2024.2) a partir de uma data
func CalculateSemester(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return ""
	}

	// Tenta extrair ano e mês com regex
	// Padrões como: DD/MM/AAAA, AAAA-MM-DD, MM/AAAA, ou apenas AAAA
	reFull := regexp.MustCompile(`(\d{1,2})[/\.-](\d{1,2})[/\.-](\d{4})`)
	if matches := reFull.FindStringSubmatch(dateStr); len(matches) == 4 {
		month, _ := strconv.Atoi(matches[2])
		year := matches[3]
		if month <= 6 {
			return fmt.Sprintf("%s.1", year)
		}
		return fmt.Sprintf("%s.2", year)
	}

	reMonthYear := regexp.MustCompile(`(\d{1,2})[/\.-](\d{4})`)
	if matches := reMonthYear.FindStringSubmatch(dateStr); len(matches) == 3 {
		month, _ := strconv.Atoi(matches[1])
		year := matches[2]
		if month <= 6 {
			return fmt.Sprintf("%s.1", year)
		}
		return fmt.Sprintf("%s.2", year)
	}

	reYear := regexp.MustCompile(`(20\d{2})`)
	if matches := reYear.FindStringSubmatch(dateStr); len(matches) == 2 {
		return fmt.Sprintf("%s.1", matches[1])
	}

	return ""
}

// NewCertificate cria um Certificate aplicando rigorosamente as regras de negócio:
// 1. Cursos/Eventos: Horas Aproveitadas = min(HorasDeclaradas, 20)
// 2. Estágio: Horas Aproveitadas = 20 imediatamente (pool de estágio é independente)
func NewCertificate(name string, courseName string, hours int, courseType string, completionDate string, category string) (*Certificate, error) {
	name = strings.TrimSpace(name)
	courseName = strings.TrimSpace(courseName)

	if name == "" {
		return nil, fmt.Errorf("nome do aluno não pode ser vazio")
	}
	if courseName == "" {
		return nil, fmt.Errorf("nome do curso não pode ser vazio")
	}

	lowCategory := strings.ToLower(category)
	lowCourse := strings.ToLower(courseName)

	isEstagio := strings.Contains(lowCategory, "estágio") ||
		strings.Contains(lowCategory, "estagio") ||
		strings.Contains(lowCourse, "estágio") ||
		strings.Contains(lowCourse, "estagio")

	declaredHours := hours
	var approvedHours int

	if isEstagio {
		category = "Estágio"
		if declaredHours <= 0 {
			declaredHours = 20
		}
		// Regra: Estágio joga imediatamente como 20 horas aproveitadas
		approvedHours = 20
	} else {
		if declaredHours <= 0 {
			return nil, fmt.Errorf("carga horária deve ser maior que zero")
		}
		// Regra: Limite de 20 horas aproveitadas por atividade
		if declaredHours > 20 {
			approvedHours = 20
		} else {
			approvedHours = declaredHours
		}
	}

	semester := CalculateSemester(completionDate)

	// Hash para evitar duplicatas: se for estágio, pool é separado
	var hashInput string
	if isEstagio {
		hashInput = fmt.Sprintf("%s|ESTAGIO|%s", name, courseName)
	} else {
		hashInput = fmt.Sprintf("%s|%s|%d", name, courseName, declaredHours)
	}

	return &Certificate{
		StudentName:    name,
		CourseName:     courseName,
		Hours:          approvedHours,
		DeclaredHours:  declaredHours,
		ApprovedHours:  approvedHours,
		CourseType:     courseType,
		CompletionDate: completionDate,
		Semester:       semester,
		Category:       category,
		Hash:           hashInput,
	}, nil
}

