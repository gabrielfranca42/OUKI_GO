import os
import re
import sys
import json
import imaplib
import email
from email.header import decode_header
import requests
from dotenv import load_dotenv

# Carrega variáveis do .env
load_dotenv()

def clean_filename(filename):
    """Remove caracteres inválidos do nome do arquivo."""
    return "".join(c for c in filename if c.isalnum() or c in "._- ")

def decode_mime_words(s):
    """Decodifica cabeçalhos MIME com acentos e codificações variadas."""
    if not s:
        return ""
    decoded_fragments = decode_header(s)
    result = []
    for fragment, encoding in decoded_fragments:
        if isinstance(fragment, bytes):
            result.append(fragment.decode(encoding or "utf-8", errors="replace"))
        else:
            result.append(str(fragment))
    return "".join(result)

def download_google_drive_file(file_id, dest_path):
    """
    Faz download de arquivo público ou compartilhado do Google Drive via endpoint direto.
    """
    url = "https://docs.google.com/uc?export=download"
    session = requests.Session()
    response = session.get(url, params={'id': file_id}, stream=True, timeout=30)

    # Verifica se o Google Drive pediu confirmação para download de arquivo grande
    token = None
    for key, value in response.cookies.items():
        if key.startswith('download_warning'):
            token = value
            break

    if token:
        params = {'id': file_id, 'confirm': token}
        response = session.get(url, params=params, stream=True, timeout=30)

    if response.status_code == 200:
        content_type = response.headers.get('Content-Type', '').lower()
        if 'html' in content_type:
            print(f"   [AVISO] Google Drive retornou HTML (arquivo pode ser restrito ou necessitar permissão): ID {file_id}")
            return False

        with open(dest_path, "wb") as f:
            for chunk in response.iter_content(chunk_size=32768):
                if chunk:
                    f.write(chunk)
        return True
    return False

def extract_drive_ids(text):
    """Localiza IDs de arquivos do Google Drive no texto ou HTML."""
    if not text:
        return []
    patterns = [
        r'drive\.google\.com/file/d/([a-zA-Z0-9_-]+)',
        r'drive\.google\.com/open\?id=([a-zA-Z0-9_-]+)',
        r'drive\.google\.com/uc\?id=([a-zA-Z0-9_-]+)',
        r'docs\.google\.com/file/d/([a-zA-Z0-9_-]+)'
    ]
    ids = set()
    for p in patterns:
        matches = re.findall(p, text)
        for m in matches:
            ids.add(m)
    return list(ids)

def fetch_outlook_emails(
    email_user=None,
    email_pass=None,
    server=None,
    port=None,
    folder="INBOX",
    output_dir="downloads_certificados",
    search_query="ALL"
):
    email_user = email_user or os.getenv("OUTLOOK_EMAIL")
    email_pass = email_pass or os.getenv("OUTLOOK_PASSWORD")
    server = server or os.getenv("OUTLOOK_SERVER", "outlook.office365.com")
    port = int(port or os.getenv("OUTLOOK_PORT", 993))

    if not email_user or not email_pass:
        print("[ERRO] Credenciais do Outlook não encontradas no .env ou argumentos.")
        print("Configure OUTLOOK_EMAIL e OUTLOOK_PASSWORD no arquivo .env")
        return []

    os.makedirs(output_dir, exist_ok=True)
    downloaded_files = []

    print(f"🔌 Conectando ao servidor IMAP {server}:{port}...")
    try:
        mail = imaplib.IMAP4_SSL(server, port)
        mail.login(email_user, email_pass)
        print("✅ Login realizado com sucesso no Outlook!")
    except Exception as e:
        print(f"[ERRO] Falha ao autenticar no Outlook: {e}")
        return []

    status, _ = mail.select(folder)
    if status != "OK":
        print(f"[ERRO] Pasta {folder} não encontrada no Outlook.")
        mail.logout()
        return []

    print(f"📥 Buscando e-mails na pasta '{folder}' com filtro '{search_query}'...")
    status, messages = mail.search(None, search_query)
    if status != "OK" or not messages[0]:
        print("[INFO] Nenhum e-mail encontrado com os critérios.")
        mail.logout()
        return []

    mail_ids = messages[0].split()
    print(f"📬 Total de e-mails encontrados: {len(mail_ids)}")

    valid_extensions = {".pdf", ".png", ".jpg", ".jpeg", ".webp"}

    for num in mail_ids:
        status, data = mail.fetch(num, "(RFC822)")
        if status != "OK" or not data:
            continue

        raw_email = data[0][1]
        msg = email.message_from_bytes(raw_email)

        subject = decode_mime_words(msg.get("Subject", "Sem Assunto"))
        from_ = decode_mime_words(msg.get("From", "Desconhecido"))

        print(f"\n📧 Processando: '{subject}' | De: {from_}")

        # Varre as partes do e-mail
        for part in msg.walk():
            content_type = part.get_content_type()
            content_disposition = str(part.get("Content-Disposition", ""))
            filename = part.get_filename()

            # 1. Trata Anexos de Arquivos
            if filename:
                filename = decode_mime_words(filename)
                clean_name = clean_filename(filename)
                ext = os.path.splitext(clean_name)[1].lower()

                if ext in valid_extensions:
                    filepath = os.path.join(output_dir, f"mail_{num.decode()}_{clean_name}")
                    payload = part.get_payload(decode=True)
                    if payload:
                        with open(filepath, "wb") as f:
                            f.write(payload)
                        print(f"   📎 Anexo baixado: {clean_name}")
                        downloaded_files.append(filepath)

            # 2. Trata Links do Google Drive no corpo do e-mail
            if content_type in ["text/plain", "text/html"]:
                payload = part.get_payload(decode=True)
                if payload:
                    body_text = payload.decode("utf-8", errors="replace")
                    drive_ids = extract_drive_ids(body_text)
                    for d_id in drive_ids:
                        print(f"   🔗 Link do Google Drive detectado: ID {d_id}")
                        drive_filename = f"drive_{num.decode()}_{d_id}.pdf"
                        drive_filepath = os.path.join(output_dir, drive_filename)
                        if download_google_drive_file(d_id, drive_filepath):
                            print(f"   ⬇️ Arquivo do Google Drive baixado: {drive_filename}")
                            downloaded_files.append(drive_filepath)

    mail.close()
    mail.logout()
    print(f"\n🎉 Total de arquivos de certificados prontos para IA: {len(downloaded_files)}")
    return downloaded_files

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Baixa certificados do Outlook corporativo e Google Drive.")
    parser.add_argument("--query", default="ALL", help="Consulta IMAP (ex: ALL, UNSEEN, SUBJECT 'certificado')")
    parser.add_argument("--folder", default="INBOX", help="Pasta do Outlook")
    parser.add_argument("--output", default="downloads_certificados", help="Diretório de destino")

    args = parser.parse_args()
    files = fetch_outlook_emails(folder=args.folder, output_dir=args.output, search_query=args.query)
    print(json.dumps(files, indent=2))
