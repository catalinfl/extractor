# API Documentație - Sistem de Rezumate Multi-Nivel

## Diferențe între tipurile de rezumat

### 🎯 1. REZUMAT GENERAL
- **Input**: TOT textul PDF-ului
- **Proces**: Analizează întreg documentul pentru tema centrală
- **Output**: 3-4 propoziții foarte concise
- **Endpoint**: Inclus în toate răspunsurile de rezumat

### 📚 2. REZUMAT PE CAPITOLE
- **Input**: TOT textul PDF-ului
- **Proces**: Detectează capitole/secțiuni și analizează fiecare individual
- **Output**: 5-8 propoziții per capitol identificat
- **Endpoint**: Optional prin parametrul `include_chapters=true`

### 📊 3. REZUMATE PE NIVELURI (1-10)
- **Input**: CHUNK-URI de pagini din PDF
- **Proces**: Împarte textul în fragmente și procesează separat
- **Output**: Rezumate progressive de la general la foarte detaliat
- **Endpoint**: Inclus automat în toate răspunsurile

## Configurarea nivelurilor

| Nivel | Pagini/Chunk | Descriere | Detaliu |
|-------|--------------|-----------|---------|
| 1 | 3 | Foarte general | Minimal |
| 2-3 | Progressive | General | Scurt |
| 4-6 | Progressive | Moderat | Echilibrat |
| 7-9 | Progressive | Detaliat | Complet |
| 10 | 20* | Foarte detaliat | Maximal |

*Pentru documente de 400+ pagini

## Endpoint-uri disponibile

### POST `/summary/generate`
Generează toate tipurile de rezumat pentru un text.

**Request Body:**
```json
{
  "text": "Textul complet din PDF...",
  "total_pages": 150,
  "language": "romanian",
  "include_chapters": true
}
```

**Response:**
```json
{
  "original_pages": 150,
  "general_summary": "Rezumatul general foarte concis...",
  "chapter_summary": [
    {
      "number": 1,
      "title": "Capitolul 1",
      "pages": "Pagina 1+",
      "summary": "Rezumatul capitolului..."
    }
  ],
  "levels": [
    {
      "level": 1,
      "description": "Rezumat foarte general (3 pagini per chunk)",
      "pages_per_chunk": 3,
      "summary": "Rezumatul nivelului 1..."
    }
  ],
  "generated_at": "2025-09-04T15:30:00Z",
  "processing_time": "2m30s"
}
```

### POST `/summary/download`
Generează rezumatul și returnează PDF pentru descărcare.

**Request Body:** (același ca `/summary/generate`)

**Response:** 
- Content-Type: `application/pdf`
- PDF cu toate rezumatele formatate

### POST `/extract/summarize`
Extrage textul din PDF și generează rezumatul în één request.

**Request:** 
- Form-data cu fișierul PDF
- Optional: `language`, `include_chapters`

**Response:** (același ca `/summary/generate`)

## Exemple de utilizare

### 1. Rezumat rapid pentru document scurt
```bash
curl -X POST http://localhost:3000/summary/generate \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Text din PDF...",
    "total_pages": 10,
    "language": "romanian"
  }'
```

### 2. Rezumat complet cu capitole
```bash
curl -X POST http://localhost:3000/summary/generate \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Text din PDF...",
    "total_pages": 200,
    "language": "romanian",
    "include_chapters": true
  }'
```

### 3. Descărcare PDF cu rezumat
```bash
curl -X POST http://localhost:3000/summary/download \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Text din PDF...",
    "total_pages": 100,
    "language": "romanian"
  }' \
  --output rezumat.pdf
```

### 4. Extract și rezumat în one-shot
```bash
curl -X POST http://localhost:3000/extract/summarize \
  -F "file=@document.pdf" \
  -F "language=romanian" \
  -F "include_chapters=true"
```

## Avantaje sistem multi-nivel

### ✅ Pentru documente mici (< 50 pagini)
- Rezumatul general oferă perspectiva de ansamblu
- Nivelurile 1-5 sunt suficiente
- Procesare rapidă

### ✅ Pentru documente mari (200+ pagini)
- Rezumatul general pentru tema principală
- Capitolele pentru structură
- Nivelurile 6-10 pentru detalii specifice
- Scalabilitate optimă

### ✅ Pentru analize complexe
- Nivel 1: Prezentare executivă
- Nivel 5: Analiză managerială
- Nivel 10: Studiu detaliat

## Performanță și limitări

### ⏱️ Timp de procesare
- Document 50 pagini: ~1-2 minute
- Document 200 pagini: ~5-8 minute  
- Document 500 pagini: ~15-20 minute

### 💾 Consum resurse
- Memoria crește cu numărul de niveluri
- Chunk-urile sunt procesate secvențial
- Optimizat pentru Railway/Docker

### 🔧 Configurare
- Toate parametrii sunt ajustabili
- Nivelurile se calculează automat
- Limba se detectează automat dacă nu este specificată

## Exemple reale de output

### Rezumat General (pentru orice document)
```
"Pe baza documentelor furnizate, această lucrare analizează impactul tehnologiei blockchain asupra sistemelor financiare moderne, explorând avantajele descentralizării și provocările implementării la scară largă."
```

### Rezumat Nivel 1 (3 pagini/chunk)
```
"Primul capitol introduce conceptele fundamentale ale blockchain-ului și criptovalutelor. Se discută despre originile Bitcoin și principiile descentralizării financiare."
```

### Rezumat Nivel 10 (20 pagini/chunk)
```
"Capitolul detaliat despre implementarea tehnică prezintă arhitectura blockchain-ului, algoritmii de consens Proof-of-Work și Proof-of-Stake, mecanismele de validare a tranzacțiilor, structura Merkle Trees pentru verificarea integrității datelor, și procesele de mining incluzând calculele hash SHA-256..."
```
