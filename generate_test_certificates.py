import os
from PIL import Image, ImageDraw, ImageFont

def create_certificate(filename, title, recipient, description, hours, date_str, category="Curso"):
    os.makedirs(os.path.dirname(filename), exist_ok=True)
    width, height = 1200, 800
    image = Image.new("RGB", (width, height), color="#FDFBF7")
    draw = ImageDraw.Draw(image)

    # Moldura dourada elegante
    border_color = "#C5A059"
    draw.rectangle([20, 20, width - 20, height - 20], outline=border_color, width=6)
    draw.rectangle([35, 35, width - 35, height - 35], outline="#333333", width=2)

    # Tenta carregar fontes do sistema, com fallback para padrão
    font_paths = [
        "/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
        "/usr/share/fonts/dejavu/DejaVuSans-Bold.ttf"
    ]
    bold_path = None
    for p in font_paths:
        if os.path.exists(p):
            bold_path = p
            break

    try:
        if bold_path:
            title_font = ImageFont.truetype(bold_path, 46)
            name_font = ImageFont.truetype(bold_path, 38)
            body_font = ImageFont.truetype(bold_path.replace("-Bold", ""), 26)
            footer_font = ImageFont.truetype(bold_path.replace("-Bold", ""), 22)
        else:
            title_font = name_font = body_font = footer_font = ImageFont.load_default()
    except Exception:
        title_font = name_font = body_font = footer_font = ImageFont.load_default()

    # Desenhar Título Centralizado
    draw.text((width / 2, 120), title, fill="#1B365D", font=title_font, anchor="mm")
    draw.line([(width / 2 - 250, 160), (width / 2 + 250, 160)], fill=border_color, width=3)

    # Texto declaratório
    draw.text((width / 2, 230), "Certificamos para os devidos fins que", fill="#555555", font=body_font, anchor="mm")
    draw.text((width / 2, 300), recipient, fill="#1B365D", font=name_font, anchor="mm")
    draw.line([(width / 2 - 200, 330), (width / 2 + 200, 330)], fill="#333333", width=1)

    # Descrição do curso / estágio
    draw.text((width / 2, 400), description, fill="#333333", font=body_font, anchor="mm")

    # Carga Horária e Categoria
    hours_text = f"Carga Horária: {hours} horas | Categoria: {category}"
    draw.text((width / 2, 470), hours_text, fill="#222222", font=name_font, anchor="mm")

    # Data de Conclusão
    date_text = f"Data de Conclusão: {date_str}"
    draw.text((width / 2, 540), date_text, fill="#444444", font=body_font, anchor="mm")

    # Assinatura institucional fictícia
    draw.line([(width / 2 - 150, 680), (width / 2 + 150, 680)], fill="#333333", width=2)
    draw.text((width / 2, 700), "Coordenação de Atividades Complementares", fill="#666666", font=footer_font, anchor="mm")

    image.save(filename, "PNG")
    print(f"✅ Certificado simulado gerado: {filename}")

if __name__ == "__main__":
    out_dir = "certificados_simulados"
    os.makedirs(out_dir, exist_ok=True)

    create_certificate(
        os.path.join(out_dir, "cert_1_beatriz_python.png"),
        title="CERTIFICADO DE CONCLUSÃO",
        recipient="Beatriz Silva Santos",
        description="concluiu com êxito o Curso Completo de Programação em Python",
        hours=15,
        date_str="10/04/2024",
        category="Curso"
    )

    create_certificate(
        os.path.join(out_dir, "cert_2_beatriz_estagio.png"),
        title="DECLARAÇÃO DE ESTÁGIO",
        recipient="Beatriz Silva Santos",
        description="cumpriu com excelência as atividades de Estágio Supervisionado",
        hours=100,
        date_str="01/09/2024",
        category="Estágio"
    )

    create_certificate(
        os.path.join(out_dir, "cert_3_carlos_cloud.png"),
        title="CERTIFICADO DE PARTICIPAÇÃO",
        recipient="Carlos Eduardo Rocha",
        description="participou do Workshop Avançado de Cloud Computing e Kubernetes",
        hours=70,
        date_str="15/10/2024",
        category="Workshop"
    )
