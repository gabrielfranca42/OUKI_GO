import json
import os
import sys
import csv
import argparse
import logging
from datetime import datetime

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger("PlanilhaGenerator")

def parse_date_str(date_val):
    if not date_val:
        return ""
    if isinstance(date_val, datetime):
        return date_val.strftime("%d/%m/%Y")
    return str(date_val).strip()

def gerar_planilha_institucional(jsonl_path, csv_path=None, xlsx_path=None, turma_nome="TADS 035-5MN - Acadweb"):
    logger.info(f"Iniciando geração de relatórios. Origem: {jsonl_path}")

    if not os.path.exists(jsonl_path):
        logger.error(f"Arquivo de dados extraídos não encontrado no caminho: {jsonl_path}")
        return False

    records = []
    invalid_lines = 0
    with open(jsonl_path, 'r', encoding='utf-8') as f:
        for line_num, line in enumerate(f, start=1):
            line = line.strip()
            if line:
                try:
                    records.append(json.loads(line))
                except Exception as e:
                    invalid_lines += 1
                    logger.warning(f"Linha #{line_num} possui JSON corrompido e foi ignorada: {e}")

    if invalid_lines > 0:
        logger.warning(f"Total de linhas corrompidas ignoradas: {invalid_lines}")

    if not records:
        logger.warning(f"Nenhum registro válido encontrado em {jsonl_path}. Nenhuma planilha será gerada.")
        return False

    logger.info(f"Total de registros válidos carregados: {len(records)}")

    # Unificação inteligente de nomes: encontra o nome mais completo e representativo
    # Ex: se existir 'LUCAS GABRIEL DA SILVA SANTANA' e 'LUCAS SANTANA', unifica no nome completo
    nomes_originais = [r.get("student_name", "").strip() for r in records if r.get("student_name")]
    nome_canônico = {}

    # Encontra nomes reais completos (que não começam com 'Aluno Identificado')
    nomes_reais = [n for n in set(nomes_originais) if not n.lower().startswith("aluno identificado") and len(n.split()) >= 2]
    nomes_reais.sort(key=lambda x: len(x), reverse=True)  # Mais longo primeiro

    for r in records:
        aluno = r.get("student_name", "Aluno Não Identificado").strip()
        aluno_final = aluno

        # Se for "Aluno Identificado...", tenta associar ao nome real mais provável
        if aluno.lower().startswith("aluno identificado") and nomes_reais:
            aluno_final = nomes_reais[0]
        else:
            # Verifica se é uma abreviação ou substring de um nome mais longo (ex: LUCAS SANTANA em LUCAS GABRIEL DA SILVA SANTANA)
            for nr in nomes_reais:
                palavras_aluno = set(aluno.lower().split())
                palavras_nr = set(nr.lower().split())
                if palavras_aluno.issubset(palavras_nr) or (len(palavras_aluno & palavras_nr) >= 2 and palavras_aluno.intersection({"silva", "santos", "da", "de"})):
                    aluno_final = nr
                    break

        r["student_name"] = aluno_final

    # Agrupa por aluno normalizado
    alunos = {}
    for r in records:
        aluno = r.get("student_name", "Aluno Não Identificado")
        if aluno not in alunos:
            alunos[aluno] = []
        alunos[aluno].append(r)

    logger.info(f"Total de alunos distintos identificados após normalização: {len(alunos)} ({list(alunos.keys())})")

    # Tenta importar openpyxl para gerar também os .xlsx
    openpyxl_disponivel = True
    try:
        import openpyxl
        from openpyxl.styles import Font, Alignment
    except ImportError:
        openpyxl_disponivel = False
        logger.warning(
            "\n[AVISO]: A biblioteca 'openpyxl' não está instalada no Python."
            "\nOs arquivos CSV formatados serão gerados normalmente. Para gerar os .xlsx:"
            "\npip install openpyxl\n"
        )

    for aluno_nome, certs in alunos.items():
        nome_sanitizado = "".join(x for x in aluno_nome if x.isalnum() or x in " _-").strip().replace(" ", "_")
        if not nome_sanitizado:
            nome_sanitizado = "Aluno"

        # Definir caminhos por pessoa
        if len(alunos) == 1 and csv_path and xlsx_path:
            arq_csv = csv_path
            arq_xlsx = xlsx_path
        else:
            arq_csv = f"Soma_Horas_Comp_{nome_sanitizado}.csv"
            arq_xlsx = f"Soma_Horas_Comp_{nome_sanitizado}.xlsx"

        # ==========================================================
        # 1. GERAÇÃO DE CSV COM MESMO PADRÃO VISUAL DO MODELO INSTITUCIONAL
        # ==========================================================
        try:
            with open(arq_csv, 'w', encoding='utf-8-sig', newline='') as f_csv:
                writer = csv.writer(f_csv, delimiter=';')
                
                # Linha 1 e 2: Cabeçalhos Institucionais
                writer.writerow(["ATIVIDADES COMPLEMENTARES", "", "", "", ""])
                writer.writerow([f"Turma: {turma_nome}", "", "", "", ""])
                writer.writerow(["", "", "", "", ""])
                writer.writerow([f"Aluno: {aluno_nome}", "", "", "", ""])
                writer.writerow(["", "", "", "", ""])

                # Linha 6: Títulos das Colunas
                headers = [
                    "DESCRIÇÃO DA ATIVIDADE / TÍTULO DO CERTIFICADO",
                    "HORAS DECLARADAS",
                    "DATA",
                    "SEMESTRE",
                    "HORAS APROVEITADAS"
                ]
                writer.writerow(headers)

                # Linhas 7 em diante: Certificados do Aluno
                # Regra Institucional Global:
                # O teto de 20 horas aproveitadas é sobre a SOMA DE TODOS os certificados de cursos (não por certificado).
                # Estágio possui pool separado independente de 20h.
                saldo_cursos_disponivel = 20
                saldo_estagio_disponivel = 20
                start_row = 7

                linhas_dados = []
                for c in certs:
                    c_name = c.get("course_name", "")
                    declared = int(c.get("declared_hours", c.get("hours", 0)))
                    c_date = parse_date_str(c.get("completion_date", ""))
                    c_semester = c.get("semester", "")
                    cat = c.get("category", "")
                    is_estagio = "estágio" in cat.lower() or "estagio" in cat.lower() or "estágio" in c_name.lower() or "estagio" in c_name.lower()

                    approved = 0
                    if is_estagio:
                        if saldo_estagio_disponivel > 0:
                            approved = min(declared if declared > 0 else 20, saldo_estagio_disponivel)
                            saldo_estagio_disponivel -= approved
                    else:
                        if saldo_cursos_disponivel > 0:
                            approved = min(declared, saldo_cursos_disponivel)
                            saldo_cursos_disponivel -= approved

                    linhas_dados.append({
                        "name": c_name,
                        "declared": declared,
                        "date": c_date,
                        "semester": c_semester,
                        "approved": approved
                    })
                    writer.writerow([c_name, declared, c_date, c_semester, approved])

                # Linha em branco
                writer.writerow(["", "", "", "", ""])

                # Linha Final: TOTAL DE HORAS COMPLETADAS
                end_row = start_row + len(certs) - 1
                formula_total = f"=SUM(E{start_row}:E{end_row})"
                writer.writerow(["TOTAL DE HORAS COMPLETADAS", "", "", "", formula_total])

            logger.info(f"CSV institucional gerado com sucesso por aluno: {arq_csv} ({len(certs)} certificados)")
        except Exception as e:
            logger.error(f"Erro ao gerar CSV para o aluno '{aluno_nome}': {e}", exc_info=True)

        # ==========================================================
        # 2. GERAÇÃO DE EXCEL (.xlsx) FIEL AO MODELO INSTITUCIONAL
        # ==========================================================
        if openpyxl_disponivel:
            try:
                wb = openpyxl.Workbook()
                ws = wb.active
                ws.title = "Planilha1"

                font_header_title = Font(name="Calibri", size=11, bold=True)
                font_table_header = Font(name="Calibri", size=11, bold=True)
                font_data = Font(name="Calibri", size=11)
                font_total = Font(name="Calibri", size=11, bold=True)

                align_center = Alignment(horizontal="center", vertical="center")
                align_left = Alignment(horizontal="left", vertical="center")

                # Cabeçalho Institucional
                ws["A1"] = "ATIVIDADES COMPLEMENTARES"
                ws["A1"].font = font_header_title

                ws["A2"] = f"Turma: {turma_nome}"
                ws["A2"].font = font_header_title

                ws["A4"] = f"Aluno: {aluno_nome}"
                ws["A4"].font = font_header_title

                # Títulos das Colunas (Linha 6)
                for col_idx, header in enumerate(headers, start=1):
                    cell = ws.cell(row=6, column=col_idx, value=header)
                    cell.font = font_table_header
                    cell.alignment = align_center

                # Inserção das Linhas de Dados (Linha 7 em diante)
                start_row = 7
                current_row = start_row
                for d in linhas_dados:
                    ws.cell(row=current_row, column=1, value=d["name"]).alignment = align_left
                    ws.cell(row=current_row, column=2, value=d["declared"]).alignment = align_center
                    ws.cell(row=current_row, column=3, value=d["date"]).alignment = align_center
                    ws.cell(row=current_row, column=4, value=d["semester"]).alignment = align_center
                    ws.cell(row=current_row, column=5, value=d["approved"]).alignment = align_center

                    for col_idx in range(1, 6):
                        ws.cell(row=current_row, column=col_idx).font = font_data

                    current_row += 1

                # Linha em branco e depois Total
                end_data_row = current_row - 1
                total_row = current_row + 1

                ws.cell(row=total_row, column=1, value="TOTAL DE HORAS COMPLETADAS").font = font_total
                total_cell = ws.cell(row=total_row, column=5, value=f"=SUM(E{start_row}:E{end_data_row})")
                total_cell.font = font_total
                total_cell.alignment = align_center

                # Ajuste das larguras das colunas
                ws.column_dimensions['A'].width = 38.0
                ws.column_dimensions['B'].width = 18.0
                ws.column_dimensions['C'].width = 16.0
                ws.column_dimensions['D'].width = 14.0
                ws.column_dimensions['E'].width = 22.0

                wb.save(arq_xlsx)
                logger.info(f"Excel institucional gerado com sucesso por aluno: {arq_xlsx} ({len(certs)} certificados)")
            except Exception as e:
                logger.error(f"Erro ao gerar Excel institucional para o aluno '{aluno_nome}': {e}", exc_info=True)

    return True

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Exporta dados de certificados para CSV e XLSX institucional por aluno.")
    parser.add_argument('--input', default='dados_extraidos.jsonl', help="Caminho do arquivo JSONL de entrada")
    parser.add_argument('--output-csv', default=None, help="Caminho do arquivo CSV de saída (se único aluno)")
    parser.add_argument('--output-xlsx', default=None, help="Caminho do arquivo XLSX de saída (se único aluno)")
    parser.add_argument('--turma', default='TADS 035-5MN - Acadweb', help="Nome/código da turma institucional")

    args = parser.parse_args()
    gerar_planilha_institucional(args.input, args.output_csv, args.output_xlsx, args.turma)
