import pandas as pd
import json
import sys
import os

def gerar_csv(jsonl_path, csv_path):
    if not os.path.exists(jsonl_path):
        print(f"Erro: Arquivo {jsonl_path} não encontrado.")
        sys.exit(1)

    try:
        data = []
        # Lê o JSONL gerado pelo Go
        with open(jsonl_path, 'r', encoding='utf-8') as f:
            for line in f:
                if line.strip():
                    data.append(json.loads(line))
        
        df = pd.DataFrame(data)
        if df.empty:
            print("Nenhum dado para exportar.")
            return

        # Remove a coluna 'hash' se existir (é só pra validação interna)
        if 'hash' in df.columns:
            df.drop(columns=['hash'], inplace=True)

        # Renomeia as chaves do struct Go para PT-BR formatado
        df.rename(columns={
            'student_name': 'Nome do Aluno',
            'course_name': 'Nome do Curso',
            'hours': 'Carga Horária (h)',
            'course_type': 'Formato',
            'completion_date': 'Data de Conclusão',
            'category': 'Categoria'
        }, inplace=True)

        # Salva em CSV padronizado (com delimitador ';')
        df.to_csv(csv_path, index=False, encoding='utf-8', sep=';')
        print(f"Sucesso! Planilha gerada em: {csv_path} com {len(df)} registros.")

    except Exception as e:
        print(f"Erro ao gerar a planilha: {str(e)}")

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Converte os dados extraídos em JSONL para CSV.")
    parser.add_argument('--input', default='dados_extraidos.jsonl', help="Arquivo JSONL de entrada")
    parser.add_argument('--output', default='planilha_certificados.csv', help="Arquivo CSV de saída")
    
    args = parser.parse_args()
    gerar_csv(args.input, args.output)
