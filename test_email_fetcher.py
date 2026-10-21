import os
import shutil
import unittest
from unittest.mock import patch, MagicMock
import email
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from email.mime.application import MIMEApplication
from email.mime.image import MIMEImage

from email_fetcher import (
    fetch_outlook_emails,
    download_google_drive_file,
    extract_drive_ids
)

class TestEmailFetcher(unittest.TestCase):
    def setUp(self):
        self.output_dir = "test_downloads"
        os.makedirs(self.output_dir, exist_ok=True)

    def tearDown(self):
        if os.path.exists(self.output_dir):
            shutil.rmtree(self.output_dir)

    def test_extract_drive_ids(self):
        text = """
        Olá, segue meu certificado no Google Drive:
        https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view?usp=sharing
        E outro arquivo:
        https://drive.google.com/open?id=2CzjNWt1YSB6oGNeLwCeCakhVVrqumct85PhwF3vqnt
        """
        ids = extract_drive_ids(text)
        self.assertEqual(len(ids), 2)
        self.assertIn("1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms", ids)
        self.assertIn("2CzjNWt1YSB6oGNeLwCeCakhVVrqumct85PhwF3vqnt", ids)

    @patch("requests.Session.get")
    def test_download_google_drive_file_success(self, mock_get):
        # Simula resposta do Google Drive com arquivo PDF binário
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.headers = {'Content-Type': 'application/pdf'}
        mock_response.cookies = {}
        mock_response.iter_content = lambda chunk_size: [b"%PDF-1.4 Fake PDF Content"]
        mock_get.return_value = mock_response

        dest_file = os.path.join(self.output_dir, "teste_drive.pdf")
        ok = download_google_drive_file("fake_id_123", dest_file)

        self.assertTrue(ok)
        self.assertTrue(os.path.exists(dest_file))
        with open(dest_file, "rb") as f:
            content = f.read()
            self.assertEqual(content, b"%PDF-1.4 Fake PDF Content")

    @patch("imaplib.IMAP4_SSL")
    @patch("email_fetcher.download_google_drive_file")
    def test_fetch_outlook_emails_simulated(self, mock_drive_download, mock_imap_cls):
        # Configura download simulado do drive
        def fake_drive_dl(file_id, dest):
            with open(dest, "wb") as f:
                f.write(b"%PDF simulated drive")
            return True
        mock_drive_download.side_effect = fake_drive_dl

        # 1. Cria mock de E-mail 1 com anexo PDF
        msg1 = MIMEMultipart()
        msg1["Subject"] = "Certificado de Horas - Aluno 1"
        msg1["From"] = "aluno1@universidade.edu.br"
        pdf_part = MIMEApplication(b"%PDF content test", _subtype="pdf")
        pdf_part.add_header("Content-Disposition", "attachment", filename="cert_python.pdf")
        msg1.attach(pdf_part)

        # 2. Cria mock de E-mail 2 com link do Google Drive no corpo
        msg2 = MIMEMultipart()
        msg2["Subject"] = "Horas complementares via Drive"
        msg2["From"] = "aluno2@universidade.edu.br"
        body_text = "Segue link: https://drive.google.com/file/d/drive_cert_xyz123/view"
        msg2.attach(MIMEText(body_text, "plain"))

        # Configura Mock IMAP
        mock_imap = MagicMock()
        mock_imap_cls.return_value = mock_imap
        mock_imap.select.return_value = ("OK", [b"10"])
        mock_imap.search.return_value = ("OK", [b"1 2"])

        def fake_fetch(num, spec):
            if num == b"1":
                return ("OK", [(None, msg1.as_bytes())])
            else:
                return ("OK", [(None, msg2.as_bytes())])

        mock_imap.fetch.side_effect = fake_fetch

        # Executa a busca simulada de e-mails
        downloaded = fetch_outlook_emails(
            email_user="test@corporativo.com",
            email_pass="senha123",
            output_dir=self.output_dir
        )

        mock_imap.login.assert_called_with("test@corporativo.com", "senha123")
        mock_imap.select.assert_called_with("INBOX")
        self.assertEqual(len(downloaded), 2)

        # Valida que os dois arquivos foram salvos na pasta
        for f in downloaded:
            self.assertTrue(os.path.exists(f))

if __name__ == "__main__":
    unittest.main()
