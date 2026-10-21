package entity_test

import (
	"testing"

	"certextractor/internal/domain/entity"
)

func TestCertificateRules(t *testing.T) {
	// Regra 1: Se mandou 15 horas de curso, tem 15 declaradas e 15 aproveitadas
	cert1, err := entity.NewCertificate("Aluno Teste", "Curso de Python", 15, "EAD", "10/03/2024", "Curso")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if cert1.DeclaredHours != 15 || cert1.ApprovedHours != 15 {
		t.Errorf("esperado 15/15, obtido declaradas=%d, aproveitadas=%d", cert1.DeclaredHours, cert1.ApprovedHours)
	}
	if cert1.Semester != "2024.1" {
		t.Errorf("esperado 2024.1, obtido %s", cert1.Semester)
	}

	// Regra 2: Se mandou 70 horas de curso, tem 70 declaradas e 20 aproveitadas
	cert2, err := entity.NewCertificate("Aluno Teste", "Curso Completo Backend", 70, "EAD", "15/09/2024", "Curso")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if cert2.DeclaredHours != 70 || cert2.ApprovedHours != 20 {
		t.Errorf("esperado 70/20, obtido declaradas=%d, aproveitadas=%d", cert2.DeclaredHours, cert2.ApprovedHours)
	}
	if cert2.Semester != "2024.2" {
		t.Errorf("esperado 2024.2, obtido %s", cert2.Semester)
	}

	// Regra 3: Estágio - limite é 20 horas e se for estágio imediatamente joga como 20 horas aproveitadas
	cert3, err := entity.NewCertificate("Aluno Teste", "Estágio Supervisionado", 100, "Presencial", "01/09/2025", "Estágio")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if cert3.ApprovedHours != 20 {
		t.Errorf("esperado aproveitadas=20 para estágio, obtido %d", cert3.ApprovedHours)
	}
	if cert3.Semester != "2025.2" {
		t.Errorf("esperado 2025.2, obtido %s", cert3.Semester)
	}

	// Regra 4: Pools separados - hash de estágio vs curso não conflitam
	if cert1.Hash == cert3.Hash {
		t.Errorf("hashes não deveriam ser iguais")
	}
}
