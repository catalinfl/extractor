# API Documentație - Sistem de Rezumate cu PDF FormFile

## 🎯 Modificări noi

Toate endpoint-urile pentru rezumate acceptă acum **PDF ca FormFile** și detectează automat:
- **Text-ul** din PDF
- **Numărul de pagini** 
- **Limba** din conținutul PDF folosind AI

## 📋 Endpoint-uri disponibile

### 1. REZUMAT PE CAPITOLE

**POST `/summary/chapters`** - Generează rezumat pe capitole
```bash
curl -X POST http://localhost:3000/summary/chapters \
  -F "file=@document.pdf"
```

**POST `/summary/chapters/download`** - Descarcă PDF cu rezumat pe capitole
```bash
curl -X POST http://localhost:3000/summary/chapters/download \
  -F "file=@document.pdf" \
  --output capitole.pdf
```

**Caracteristici:**
- ✅ Primește TOT textul PDF-ului
- ✅ Detectează capitole/secțiuni automat
- ✅ Creează secțiuni logice dacă nu găsește capitole
- ✅ Rezumat moderat (5-8 propoziții per capitol)

---

### 2. REZUMAT GENERAL

**POST `/summary/general`** - Generează rezumat general
```bash
# Pentru o pagină completă
curl -X POST http://localhost:3000/summary/general \
  -F "file=@document.pdf" \
  -F "one_line=false"

# Pentru o singură linie
curl -X POST http://localhost:3000/summary/general \
  -F "file=@document.pdf" \
  -F "one_line=true"
```

**POST `/summary/general/download`** - Descarcă PDF cu rezumat general
```bash
curl -X POST http://localhost:3000/summary/general/download \
  -F "file=@document.pdf" \
  -F "one_line=false" \
  --output general.pdf
```

**Parametri:**
- `one_line=true`: O singură propoziție (maxim 25-30 cuvinte)
- `one_line=false`: O pagină completă (200-250 cuvinte, 3-5 paragrafe)

**Caracteristici:**
- ✅ Primește TOT textul PDF-ului
- ✅ Analizează întreg documentul pentru tema centrală
- ✅ Două moduri: concis (o linie) sau detaliat (o pagină)

---

### 3. REZUMAT PE NIVELE

**POST `/summary/level`** - Generează rezumat pentru nivel specific
```bash
curl -X POST http://localhost:3000/summary/level \
  -F "file=@document.pdf" \
  -F "level=5"
```

**POST `/summary/level/download`** - Descarcă PDF cu rezumat pe nivel
```bash
curl -X POST http://localhost:3000/summary/level/download \
  -F "file=@document.pdf" \
  -F "level=10" \
  --output nivel10.pdf
```

**Parametri:**
- `level=1-10`: Nivelul de detaliu dorit

**Caracteristici:**
- ✅ Lucrează cu CHUNK-URI de pagini
- ✅ Nivel 1: Foarte general (3 pagini/chunk)
- ✅ Nivel 10: Foarte detaliat (20 pagini/chunk)
- ✅ Fiecare chunk procesat separat, apoi combinat

---

## 🔄 Flux de procesare

```
1. Upload PDF → FormFile
2. Extract text → extractTextPages() 
3. Detect language → detectLanguageFromText() cu AI
4. Generate summary → Funcție specifică tipului
5. Return JSON/PDF → Răspuns formatat
```

## 🌍 Detectare automată limbă

AI-ul detectează limba din conținutul PDF și returnează:
- `romanian` (default)
- `english`
- `spanish` 
- `french`
- `german`
- `italian`

## 📊 Exemple de răspuns JSON

### Rezumat capitole:
```json
{
  "success": true,
  "type": "chapter_summary",
  "filename": "document.pdf",
  "original_pages": 150,
  "language": "romanian",
  "chapters": [
    {
      "number": 1,
      "title": "Introducere",
      "pages": "Secțiunea 1 din 4",
      "summary": "Capitolul introduce conceptele principale..."
    }
  ],
  "total_chapters": 4
}
```

### Rezumat general:
```json
{
  "success": true,
  "type": "general_summary", 
  "filename": "document.pdf",
  "original_pages": 150,
  "language": "romanian",
  "summary": "Documentul analizează impactul tehnologiei...",
  "one_line": false
}
```

### Rezumat nivel:
```json
{
  "success": true,
  "type": "level_summary",
  "filename": "document.pdf", 
  "original_pages": 150,
  "language": "romanian",
  "level": {
    "level": 5,
    "description": "Rezumat nivel 5 (15 pagini per chunk)",
    "pages_per_chunk": 15,
    "summary": "Analiza detaliată arată că..."
  }
}
```

## 🚀 Testare rapidă

```bash
# Test toate tipurile cu același PDF
PDF_FILE="test.pdf"

# 1. Capitole
curl -X POST http://localhost:3000/summary/chapters -F "file=@$PDF_FILE"

# 2. General (o linie)
curl -X POST http://localhost:3000/summary/general -F "file=@$PDF_FILE" -F "one_line=true"

# 3. Nivel detaliat
curl -X POST http://localhost:3000/summary/level -F "file=@$PDF_FILE" -F "level=8"

# 4. Descărcare PDF nivel
curl -X POST http://localhost:3000/summary/level/download -F "file=@$PDF_FILE" -F "level=10" --output detaliat.pdf
```

## ⚡ Avantaje noi

### ✅ Simplificare utilizare
- Un singur fișier PDF ca input
- Fără calcule manuale pentru `total_pages`
- Fără specificare manuală limbă

### ✅ Procesare inteligentă  
- Detectare automată limba cu AI
- Extragere text optimizată cu MuPDF
- Validare automată fișier PDF

### ✅ Flexibilitate
- 3 tipuri distincte de rezumat
- Parametri simpli (level, one_line)
- Descărcare directă PDF formatat

### ✅ Scalabilitate
- Funcționează cu PDF-uri de orice dimensiune
- Optimizat pentru Railway/Docker
- Managemenet memorie eficient

## 🎯 Cazuri de utilizare

**Pentru documente scurte (< 50 pagini):**
```bash
# Rezumat rapid o linie
curl -X POST http://localhost:3000/summary/general -F "file=@document.pdf" -F "one_line=true"
```

**Pentru analize detaliate:**
```bash
# Nivel foarte detaliat
curl -X POST http://localhost:3000/summary/level -F "file=@document.pdf" -F "level=10"
```

**Pentru structură document:**
```bash  
# Capitole și secțiuni
curl -X POST http://localhost:3000/summary/chapters -F "file=@document.pdf"
```
