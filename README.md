# Document Text Extraction Server

🚀 Server Fiber pentru extragerea de text din documente PDF și ODT.

## Caracteristici

- **Suport multiple formate**: PDF, ODT
- **API flexibil**: Returnează fie JSON structurat, fie text simplu
- **Algoritm îmbunătățit**: Spațiere inteligentă între cuvinte pentru PDF-uri
- **Upload mari**: Suportă fișiere până la 100MB
- **CORS activat**: Ready pentru aplicații web frontend

## Endpoints

### 🏠 GET `/`
Informații despre server și endpoints disponibili.

### 🔍 POST `/extract`
Extrage text și returnează JSON structurat cu pagini.

**Request:**
- Multipart form cu field `file`
- SAU raw binary în body

**Response:**
```json
{
  "success": true,
  "file_type": "pdf",
  "num_pages": 3,
  "pages": [
    "Text from page 1...",
    "Text from page 2...",
    "Text from page 3..."
  ]
}
```

### 📄 POST `/extract/text`
Extrage text și returnează text simplu concatenat.

**Request:** Același ca `/extract`

**Response:** Plain text cu pagini separate prin `\n\n`

### ❤️ GET `/health`
Health check pentru monitoring.

## Utilizare

### Pornirea serverului
```bash
go run .
```

Serverul va rula pe `http://localhost:3000`

### Testare cu curl

#### Upload multipart form:
```bash
# JSON response cu pagini
curl -F "file=@document.pdf" http://localhost:3000/extract

# Plain text response
curl -F "file=@document.pdf" http://localhost:3000/extract/text
```

#### Upload raw binary:
```bash
# JSON response
curl --data-binary "@document.pdf" \
     -H "Content-Type: application/pdf" \
     http://localhost:3000/extract

# Plain text response  
curl --data-binary "@document.pdf" \
     -H "Content-Type: application/pdf" \
     http://localhost:3000/extract/text
```

### Testare cu PowerShell
```powershell
# Upload cu Invoke-RestMethod
$response = Invoke-RestMethod -Uri "http://localhost:3000/extract" `
                              -Method Post `
                              -InFile ".\document.pdf" `
                              -ContentType "application/pdf"

# Afișare JSON frumos formatat
$response | ConvertTo-Json -Depth 10
```

## Formате suportate

### 📕 PDF
- Extrage text cu spațiere inteligentă între cuvinte
- Păstrează structura pe pagini
- Suportă PDF-uri cu text (nu face OCR pentru imagini scanate)

### 📘 ODT (OpenDocument Text)
- Extrage text din content.xml
- Returnează conținut ca o singură "pagină"
- Elimină formatarea XML și păstrează doar textul

## Algoritm de extragere PDF

Serverul folosește o logică avansată pentru reconstruirea textului din PDF-uri:

1. **Grupare pe rânduri**: Fragmentele de text sunt grupate pe coordonata Y
2. **Sortare pe X**: În fiecare rând, fragmentele sunt sortate de la stânga la dreapta  
3. **Spațiere inteligentă**: Adaugă spații bazat pe:
   - Gap-ul fizic între fragmente
   - Mărimea fontului
   - Caractere de punctuație
   - Diferențe de font

## Build și Deploy

### Development
```bash
# Install dependencies
go mod tidy

# Run with hot reload
air

# Build
go build -o document-extractor
```

### Production
```bash
# Build pentru Windows
go build -o document-extractor.exe

# Build pentru Linux
GOOS=linux GOARCH=amd64 go build -o document-extractor-linux

# Run
./document-extractor
```

## Configurare

Aplicația folosește următoarele setări:
- **Port**: 3000 (hardcoded)
- **Body limit**: 100MB pentru upload-uri mari
- **CORS**: Activat pentru toate origin-urile
- **Logging**: Middleware Fiber pentru request logging

## Dependențe

- `github.com/gofiber/fiber/v2` - Web framework rapid
- `rsc.io/pdf` - PDF reader pur Go (fără CGO)
- Go standard library pentru ODT (archive/zip)

## Limitări

- **PDF-uri scanate**: Nu face OCR; necesită text selectabil
- **ODT complex**: Extrage doar textul de bază, fără formatare avansată
- **Fonturi**: Rezultatele pot varia în funcție de fonturile folosite în PDF

## Contribuții

Pentru îmbunătățiri ale algoritmului de spațiere sau suport pentru alte formate, deschide un issue sau PR.
