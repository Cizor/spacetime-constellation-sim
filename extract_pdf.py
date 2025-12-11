import sys
from pathlib import Path
try:
    from pypdf import PdfReader
    pdf_path = Path(sys.argv[1])
    reader = PdfReader(str(pdf_path))
    text = ""
    for page in reader.pages:
        text += page.extract_text() + "\n"
    print(text)
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
