# Document Text Extraction & OCR Server

🚀 Server Fiber pentru extragerea de text din documente PDF și imagini cu suport OCR (Tesseract).

## Caracteristici

- **Suport multiple formate**: PDF, PNG, JPG, JPEG, TIFF, BMP
- **OCR avansat**: Tesseract cu suport pentru multiple limbi
- **Extragere directă PDF**: Text din PDF-uri căutabile
- **API flexibil**: Returnează fie JSON structurat, fie text simplu
- **Upload mari**: Suportă fișiere până la 100MB
- **CORS activat**: Ready pentru aplicații web frontend
- **Docker ready**: Deployment simplu pe Railway, Heroku, etc.

## Endpoints

### 🏠 GET `/`
Informații despre server și endpoints disponibili.

### 🔍 POST `/extract`
Extrage text din PDF și returnează JSON structurat cu pagini.

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
Extrage text din PDF și returnează text simplu concatenat.

### 👁️ POST `/extract/ocr`
Extrage text din PDF-uri scanate sau imagini folosind OCR.

**Request:**
- Multipart form cu fields: `file`, `lang` (opțional, default: "eng")

**Response:**
```json
{
  "success": true,
  "file_type": "pdf",
  "num_pages": 2,
  "language": "eng",
  "pages": ["OCR text from page 1...", "OCR text from page 2..."],
  "text": "Combined OCR text...",
  "timestamp": "2025-08-26T14:12:59+03:00"
}
```

### ℹ️ GET `/ocr/info`
Informații despre capabilitățile OCR și limbile disponibile.

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

## 🚀 Railway Deployment

### Quick Deploy
[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/your-template-id)

### Manual Deploy Steps

1. **Connect Repository**
   ```bash
   # Using Railway CLI
   railway login
   railway link
   railway up
   ```

2. **Or use Railway Dashboard**
   - Go to [railway.app](https://railway.app)
   - Click "New Project" → "Deploy from GitHub repo"
   - Select this repository
   - Railway auto-detects Dockerfile

3. **Environment Variables** (Optional)
   - Railway sets `PORT` automatically
   - `TESSERACT_CMD` and `PDFTOPPM_CMD` not needed (pre-installed in Docker)

### Docker Testing Local
```bash
# Build image
docker build -t pdf-extractor .

# Run container
docker run -p 3000:3000 pdf-extractor

# Test endpoints
curl http://localhost:3000/health
curl -F "file=@test.pdf" http://localhost:3000/extract/ocr
```

## Configurare

Aplicația respectă următoarele variabile de mediu:
- **PORT**: Port server (default: 3000, Railway setează automat)
- **TESSERACT_CMD**: Cale custom către tesseract executable (opțional)
- **PDFTOPPM_CMD**: Cale custom către pdftoppm executable (opțional)

### Setări aplicație:
- **Body limit**: 100MB pentru upload-uri mari
- **CORS**: Activat pentru toate origin-urile  
- **Logging**: Middleware Fiber pentru request logging

## Dependențe

### Go packages:
- `github.com/gofiber/fiber/v2` - Web framework rapid
- `github.com/ledongthuc/pdf` - PDF reader pur Go
- Go standard library

### System dependencies (Docker):
- **Tesseract OCR** cu pachete limbi multiple
- **Poppler utilities** pentru conversie PDF→imagine
- **Ubuntu 22.04** base image

## Limbile OCR suportate

- `eng` - English
- `fra` - French  
- `deu` - German
- `spa` - Spanish
- `ita` - Italian
- `por` - Portuguese
- `rus` - Russian
- `chi_sim` - Chinese Simplified
- `jpn` - Japanese
- `kor` - Korean

## Limitări

- **Dimensiune fișier**: Max 100MB
- **Timeout**: 30s pentru procesare (Railway default)
- **Formate suportate**: PDF, PNG, JPG, JPEG, TIFF, BMP
- **OCR**: Depinde de calitatea imaginii și limba setată

## Troubleshooting

1. **Large files**: Verifică că fișierul e sub 100MB
2. **OCR errors**: Verifică că limba e instalată (`/ocr/info` endpoint)
3. **PDF conversion**: Verifică că fișierul nu e corupt
4. **Railway logs**: Check deployment logs în Railway dashboard

## Contribuții

Pentru îmbunătățiri ale algoritmului sau suport pentru alte formate, deschide un issue sau PR.
