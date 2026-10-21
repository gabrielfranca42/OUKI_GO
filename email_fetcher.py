import os
import re
import sys
import json
import base64
import logging
import imaplib
import email
from email.header import decode_header
import requests
from dotenv import load_dotenv

# Configuração de logging estruturado
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger("EmailFetcher")

load_dotenv()

def clean_filename(filename):
    """Remove caracteres inválidos do nome do arquivo."""
    return "".join(c for c in filename if c.isalnum() or c in "._- ")

def decode_mime_words(s):
    """Decodifica cabeçalhos MIME com acentos e codificações variadas."""
    if not s:
        return ""
    try:
        decoded_fragments = decode_header(s)
        result = []
        for fragment, encoding in decoded_fragments:
            if isinstance(fragment, bytes):
                result.append(fragment.decode(encoding or "utf-8", errors="replace"))
            else:
                result.append(str(fragment))
        return "".join(result)
    except Exception as e:
        logger.warning(f"Falha ao decodificar cabeçalho MIME '{s}': {e}")
        return str(s)

def resolve_imap_server(email_user, server_override=None):
    """Detecta automaticamente o servidor IMAP se não for explicitado."""
    if server_override and server_override.strip():
        return server_override.strip()

    email_lower = (email_user or "").strip().lower()
    if email_lower.endswith("@gmail.com") or email_lower.endswith("@googlemail.com"):
        logger.info("Detectado provedor Gmail. Usando servidor imap.gmail.com")
        return "imap.gmail.com"
    else:
        logger.info("Detectado provedor Microsoft 365 / Outlook. Usando servidor outlook.office365.com")
        return "outlook.office365.com"

def download_google_drive_file(file_id, dest_path):
    """
    Faz download de arquivo público ou compartilhado do Google Drive via endpoint direto.
    """
    url = "https://docs.google.com/uc?export=download"
    session = requests.Session()
    logger.info(f"Tentando download do Google Drive: ID={file_id}")

    try:
        response = session.get(url, params={'id': file_id}, stream=True, timeout=30)

        token = None
        for key, value in response.cookies.items():
            if key.startswith('download_warning'):
                token = value
                break

        if token:
            logger.info(f"Arquivo grande detectado no Google Drive. Confirmando token de segurança...")
            params = {'id': file_id, 'confirm': token}
            response = session.get(url, params=params, stream=True, timeout=30)

        if response.status_code == 200:
            content_type = response.headers.get('Content-Type', '').lower()
            if 'html' in content_type:
                logger.warning(
                    f"Google Drive retornou HTML para o ID {file_id}. "
                    "Motivo comum: o link requer permissão de acesso ou foi excluído."
                )
                return False

            total_bytes = 0
            with open(dest_path, "wb") as f:
                for chunk in response.iter_content(chunk_size=32768):
                    if chunk:
                        f.write(chunk)
                        total_bytes += len(chunk)

            logger.info(f"Download do Google Drive concluído com sucesso: {dest_path} ({total_bytes} bytes)")
            return True
        else:
            logger.error(f"Erro no download do Google Drive (ID={file_id}): HTTP {response.status_code}")
            return False

    except Exception as e:
        logger.error(f"Erro no download do Drive (ID={file_id}): {e}")
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
        for m in re.findall(p, text):
            ids.add(m)
    return list(ids)

def fetch_office365_graph(email_user=None, output_dir="downloads_certificados"):
    """
    Autenticação moderna OAuth2 Device Code Flow para Microsoft 365 (SENAC / Corporativo).
    Permite login seguro via navegador corporativo mesmo com Basic Auth desativado e MFA ativado.
    """
    logger.info("Iniciando conector Microsoft Graph API (OAuth2 Device Code Flow)...")
    try:
        import msal
    except ImportError:
        logger.error("Biblioteca 'msal' não instalada. Execute: pip install msal")
        return []

    # Client ID oficial da Microsoft para ferramentas públicas CLI
    client_id = os.getenv("AZURE_CLIENT_ID", "14d82eec-204b-4a2f-b3e4-5407740e43ad")
    scopes = ["Mail.Read", "User.Read"]

    app = msal.PublicClientApplication(client_id, authority="https://login.microsoftonline.com/common")
    flow = app.initiate_device_flow(scopes=scopes)

    if "user_code" not in flow:
        logger.error(f"Falha ao iniciar Device Code Flow: {flow.get('error_description')}")
        return []

    print("\n" + "="*65)
    print("🔐 AUTENTICAÇÃO MICROSOFT 365 / SENAC NECESSÁRIA")
    print("="*65)
    print(flow["message"])
    print("="*65 + "\n")
    sys.stdout.flush()

    result = app.acquire_token_by_device_flow(flow)

    if "access_token" not in result:
        logger.error(f"Falha na autenticação OAuth2: {result.get('error_description')}")
        return []

    logger.info("Autenticação OAuth2 realizada com SUCESSO via Microsoft Graph!")
    token = result["access_token"]
    headers = {"Authorization": f"Bearer {token}"}

    os.makedirs(output_dir, exist_ok=True)
    downloaded_files = []
    valid_extensions = {".pdf", ".png", ".jpg", ".jpeg", ".webp"}

    # Busca as últimas mensagens da caixa de entrada com anexos expandidos
    graph_url = "https://graph.microsoft.com/v1.0/me/messages?$top=30&$expand=attachments"
    logger.info("Buscando mensagens e anexos via Microsoft Graph API...")

    resp = requests.get(graph_url, headers=headers, timeout=30)
    if resp.status_code != 200:
        logger.error(f"Erro ao consultar mensagens no Microsoft Graph: HTTP {resp.status_code} - {resp.text}")
        return []

    messages = resp.json().get("value", [])
    logger.info(f"Total de e-mails obtidos: {len(messages)}")

    for idx, msg in enumerate(messages, start=1):
        subject = msg.get("subject", "Sem Assunto")
        from_ = msg.get("from", {}).get("emailAddress", {}).get("address", "Desconhecido")
        body_content = msg.get("body", {}).get("content", "")

        logger.info(f"[{idx}/{len(messages)}] E-mail: '{subject}' | De: {from_}")

        # 1. Trata Anexos diretos
        attachments = msg.get("attachments", [])
        for att in attachments:
            att_name = clean_filename(att.get("name", ""))
            ext = os.path.splitext(att_name)[1].lower()
            if ext in valid_extensions and "contentBytes" in att:
                file_bytes = base64.b64decode(att["contentBytes"])
                dest_path = os.path.join(output_dir, f"graph_{msg['id'][:8]}_{att_name}")
                with open(dest_path, "wb") as f:
                    f.write(file_bytes)
                logger.info(f"   [ANEXO BAIXADO] {att_name} ({len(file_bytes)} bytes) -> {dest_path}")
                downloaded_files.append(dest_path)

        # 2. Trata Links do Google Drive no corpo
        if body_content:
            drive_ids = extract_drive_ids(body_content)
            for d_id in drive_ids:
                logger.info(f"   [GOOGLE DRIVE ENCONTRADO] ID detectado: {d_id}")
                drive_filename = f"drive_{msg['id'][:8]}_{d_id}.pdf"
                drive_filepath = os.path.join(output_dir, drive_filename)
                if download_google_drive_file(d_id, drive_filepath):
                    downloaded_files.append(drive_filepath)

    logger.info(f"Total de certificados baixados via Microsoft Graph: {len(downloaded_files)}")
    return downloaded_files

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
    server = resolve_imap_server(email_user, server or os.getenv("OUTLOOK_SERVER"))
    port = int(port or os.getenv("OUTLOOK_PORT", 993))

    logger.info("Iniciando processo de verificação de e-mails...")
    logger.info(f"Configurações: Usuário={email_user} | Servidor={server}:{port} | Pasta={folder}")

    if not email_user:
        logger.error("E-mail não configurado no .env!")
        return []

    # 1. Conexão IMAP
    try:
        logger.info(f"Estabelecendo conexão segura SSL com {server}:{port}...")
        mail = imaplib.IMAP4_SSL(server, port)
    except Exception as e:
        logger.error(f"Falha ao conectar via SSL ao servidor {server}:{port}: {e}")
        return []

    # 2. Autenticação IMAP
    try:
        logger.info("Enviando credenciais de autenticação...")
        mail.login(email_user, email_pass)
        logger.info("Autenticação IMAP realizada com SUCESSO!")
    except imaplib.IMAP4.error as e:
        err_msg = str(e)
        logger.error(f"Falha na autenticação com o servidor IMAP: {err_msg}")

        # Se for Office 365 / SENAC com Basic Auth desativado, ativa o conector moderno Microsoft Graph!
        if "Basic authentication is disabled" in err_msg or "AUTHENTICATIONFAILED" in err_msg:
            if "outlook" in server or "office365" in server or "senac" in email_user.lower():
                logger.info(
                    "O servidor Microsoft 365 bloqueou Basic Auth (senha simples). "
                    "Iniciando automaticamente o fluxo moderno oficial da Microsoft (OAuth2 Device Flow)..."
                )
                return fetch_office365_graph(email_user, output_dir)
        return []
    except Exception as e:
        logger.error(f"Erro inesperado durante login: {e}")
        return []

    # Se o IMAP tiver sucesso, prossegue normalmente
    os.makedirs(output_dir, exist_ok=True)
    downloaded_files = []

    status, folder_data = mail.select(folder)
    if status != "OK":
        logger.error(f"Não foi possível abrir a pasta '{folder}'.")
        mail.logout()
        return []

    status, messages = mail.search(None, search_query)
    if status != "OK" or not messages[0]:
        logger.info("Nenhum e-mail encontrado.")
        mail.logout()
        return []

    mail_ids = messages[0].split()
    valid_extensions = {".pdf", ".png", ".jpg", ".jpeg", ".webp"}

    for num in mail_ids:
        status, data = mail.fetch(num, "(RFC822)")
        if status != "OK" or not data:
            continue

        msg = email.message_from_bytes(data[0][1])
        for part in msg.walk():
            filename = part.get_filename()
            if filename:
                clean_name = clean_filename(decode_mime_words(filename))
                ext = os.path.splitext(clean_name)[1].lower()
                if ext in valid_extensions:
                    filepath = os.path.join(output_dir, f"mail_{num.decode()}_{clean_name}")
                    payload = part.get_payload(decode=True)
                    if payload:
                        with open(filepath, "wb") as f:
                            f.write(payload)
                        downloaded_files.append(filepath)

            if part.get_content_type() in ["text/plain", "text/html"]:
                payload = part.get_payload(decode=True)
                if payload:
                    body_text = payload.decode("utf-8", errors="replace")
                    for d_id in extract_drive_ids(body_text):
                        drive_filepath = os.path.join(output_dir, f"drive_{num.decode()}_{d_id}.pdf")
                        if download_google_drive_file(d_id, drive_filepath):
                            downloaded_files.append(drive_filepath)

    mail.close()
    mail.logout()
    return downloaded_files

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Baixa certificados do Outlook corporativo e Google Drive.")
    parser.add_argument("--query", default="ALL", help="Consulta IMAP (ex: ALL, UNSEEN)")
    parser.add_argument("--folder", default="INBOX", help="Pasta do e-mail")
    parser.add_argument("--output", default="downloads_certificados", help="Diretório de destino")

    args = parser.parse_args()
    files = fetch_outlook_emails(folder=args.folder, output_dir=args.output, search_query=args.query)
    print(json.dumps(files, indent=2))
