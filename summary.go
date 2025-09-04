package main

/*
SISTEM DE REZUMATE MULTI-NIVEL

DIFERENȚE IMPORTANTE:

1. REZUMAT GENERAL:
   - Primește TOT textul PDF-ului
   - Analizează întregul document pentru temă centrală
   - Foarte concis (3-4 propoziții)

2. REZUMAT PE CAPITOLE:
   - Primește TOT textul PDF-ului
   - Detectează capitole/secțiuni și le analizează individual
   - Moderat (5-8 propoziții per capitol)

3. REZUMATE PE NIVELURI (1-10):
   - Lucrează cu CHUNK-URI de pagini
   - Fiecare chunk este procesat separat
   - Nivel 1: 3 pagini/chunk (foarte general)
   - Nivel 10: 20 pagini/chunk (foarte detaliat)
   - Pentru fiecare nivel se combină rezumatele chunk-urilor

FLUX:
- Extract PDF → Text complet
- Rezumat general ← Text complet
- Rezumat capitole ← Text complet
- Rezumate niveluri ← Chunk-uri de text
*/

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// SummaryLevel reprezintă un nivel de rezumat
type SummaryLevel struct {
	Level         int    `json:"level"`
	Description   string `json:"description"`
	PagesPerChunk int    `json:"pages_per_chunk"`
	Summary       string `json:"summary"`
}

// SummaryResult reprezintă rezultatul complet al rezumării
type SummaryResult struct {
	OriginalPages  int            `json:"original_pages"`
	GeneralSummary string         `json:"general_summary"`
	ChapterSummary []ChapterInfo  `json:"chapter_summary,omitempty"`
	Levels         []SummaryLevel `json:"levels"`
	GeneratedAt    time.Time      `json:"generated_at"`
	ProcessingTime string         `json:"processing_time"`
}

// ChapterInfo reprezintă informații despre un capitol
type ChapterInfo struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Pages   string `json:"pages"`
	Summary string `json:"summary"`
}

// SummaryRequest reprezintă cererea pentru generarea rezumatului
type SummaryRequest struct {
	Text            string `json:"text"`
	TotalPages      int    `json:"total_pages"`
	Language        string `json:"language,omitempty"`
	IncludeChapters bool   `json:"include_chapters,omitempty"`
}

// calculateSummaryLevels calculează configurarea pentru fiecare nivel
func calculateSummaryLevels(totalPages int) []SummaryLevel {
	levels := make([]SummaryLevel, 10)

	// Nivel 1: 3 pagini per chunk (cel mai general)
	levels[0] = SummaryLevel{
		Level:         1,
		Description:   "Rezumat foarte general (3 pagini per chunk)",
		PagesPerChunk: 3,
	}

	// Niveluri 2-9: progresiv mai detaliate
	for i := 1; i < 9; i++ {
		pagesPerChunk := int(math.Max(3, float64(totalPages)/math.Pow(2, float64(10-i))))
		levels[i] = SummaryLevel{
			Level:         i + 1,
			Description:   fmt.Sprintf("Rezumat nivel %d (%d pagini per chunk)", i+1, pagesPerChunk),
			PagesPerChunk: pagesPerChunk,
		}
	}

	// Nivel 10: cel mai detaliat (20 pagini per chunk pentru 400+ pagini)
	pagesPerChunk := 20
	if totalPages < 100 {
		pagesPerChunk = int(math.Max(5, float64(totalPages)/10))
	}
	levels[9] = SummaryLevel{
		Level:         10,
		Description:   fmt.Sprintf("Rezumat foarte detaliat (%d pagini per chunk)", pagesPerChunk),
		PagesPerChunk: pagesPerChunk,
	}

	return levels
}

// chunkTextByPages împarte textul în chunk-uri bazate pe numărul de pagini
func chunkTextByPages(text string, totalPages int, pagesPerChunk int) []string {
	if totalPages <= 0 || pagesPerChunk <= 0 {
		return []string{text}
	}

	// Estimează lungimea medie per pagină
	avgCharsPerPage := len(text) / totalPages
	chunkSize := avgCharsPerPage * pagesPerChunk

	if chunkSize >= len(text) {
		return []string{text}
	}

	var chunks []string
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := text[i:end]
		// Încearcă să termine la sfârșitul unei propoziții
		if end < len(text) {
			lastDot := strings.LastIndex(chunk, ".")
			lastQuestion := strings.LastIndex(chunk, "?")
			lastExclamation := strings.LastIndex(chunk, "!")

			lastSentenceEnd := int(math.Max(float64(lastDot), math.Max(float64(lastQuestion), float64(lastExclamation))))
			if lastSentenceEnd > len(chunk)/2 { // Dacă găsim sfârșitul unei propoziții în a doua jumătate
				chunk = chunk[:lastSentenceEnd+1]
				i = i + lastSentenceEnd + 1 - chunkSize // Ajustează indexul
			}
		}

		chunks = append(chunks, strings.TrimSpace(chunk))
	}

	return chunks
}

// generateChunkSummary generează rezumatul pentru un chunk de text - PENTRU NIVELURI (1-10)
func generateChunkSummary(chunk string, chunkIndex int, totalChunks int, level int, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Ești un expert în rezumarea textelor. Fă un rezumat profesional al acestui CHUNK de text.

ATENȚIE: Primești un FRAGMENT (chunk %d din %d) din PDF, NU întreg documentul!

INFORMAȚII CONTEXT:
- Chunk %d din %d (fragment din PDF)
- Nivel de detaliu: %d (1=foarte general, 10=foarte detaliat)
- Limba: %s

INSTRUCȚIUNI REZUMAT PENTRU NIVELURI:
- Pentru nivel 1-3: Rezumat foarte concis, doar ideile principale din acest chunk
- Pentru nivel 4-6: Rezumat moderat, include detalii importante din acest chunk
- Pentru nivel 7-10: Rezumat detaliat, păstrează informații specifice din acest chunk

IMPORTANT: Rezumă DOAR conținutul din acest chunk, fără a presupune context din alte părți!

LIMBA: %s (folosește diacritice corecte pentru română)

Returnează DOAR rezumatul chunk-ului, fără introduceri sau explicații.

CHUNK DE REZUMAT:
%s`, chunkIndex+1, totalChunks, chunkIndex+1, totalChunks, level, language, language, chunk)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.3,
		MaxTokens:   1000,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert în rezumarea textelor în limba %s. Faci rezumate profesionale și clare.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	summary, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary for chunk %d: %v", chunkIndex+1, err)
	}

	return strings.TrimSpace(summary), nil
}

// generateLevelSummary generează rezumatul pentru un nivel specific
func generateLevelSummary(text string, totalPages int, level SummaryLevel, language string) (string, error) {
	fmt.Printf("📄 Generez rezumat pentru nivelul %d (%d pagini per chunk)...\n", level.Level, level.PagesPerChunk)

	chunks := chunkTextByPages(text, totalPages, level.PagesPerChunk)
	fmt.Printf("📄 Împărțit în %d chunk-uri pentru nivelul %d\n", len(chunks), level.Level)

	var summaries []string

	for i, chunk := range chunks {
		fmt.Printf("📄 Procesez chunk %d/%d pentru nivelul %d...\n", i+1, len(chunks), level.Level)

		summary, err := generateChunkSummary(chunk, i, len(chunks), level.Level, language)
		if err != nil {
			return "", err
		}

		summaries = append(summaries, summary)
	}

	// Combinare rezumate chunk-uri într-un rezumat final pentru nivel
	if len(summaries) == 1 {
		return summaries[0], nil
	}

	combinedSummaries := strings.Join(summaries, "\n\n")
	finalSummary, err := generateFinalSummaryForLevel(combinedSummaries, level.Level, language)
	if err != nil {
		return "", err
	}

	return finalSummary, nil
}

// generateFinalSummaryForLevel generează rezumatul final pentru un nivel
func generateFinalSummaryForLevel(combinedSummaries string, level int, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Ai primit mai multe rezumate parțiale pentru nivelul %d. Combină-le într-un rezumat coerent și complet.

NIVEL: %d
- Pentru nivel 1-3: Rezumat foarte concis
- Pentru nivel 4-6: Rezumat echilibrat  
- Pentru nivel 7-10: Rezumat detaliat

LIMBA: %s

Instrucțiuni:
1. Combină informațiile într-un mod logic și coerent
2. Elimină redundanțele
3. Păstrează fluxul narativ
4. Respectă nivelul de detaliu cerut

REZUMATE DE COMBINAT:
%s`, level, level, language, combinedSummaries)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.2,
		MaxTokens:   1500,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert în sinteza textelor în limba %s.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	finalSummary, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(finalSummary), nil
}

// detectChapters încearcă să detecteze capitolele din text
func detectChapters(text string) []ChapterInfo {
	// Simplă detecție bazată pe pattern-uri comune
	var chapters []ChapterInfo

	lines := strings.Split(text, "\n")
	chapterNum := 1

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Pattern-uri pentru capitole
		if strings.Contains(strings.ToLower(line), "capitol") ||
			strings.Contains(strings.ToLower(line), "chapter") ||
			strings.HasPrefix(line, "Cap.") ||
			strings.HasPrefix(line, "Ch.") {

			chapters = append(chapters, ChapterInfo{
				Number: chapterNum,
				Title:  line,
				Pages:  fmt.Sprintf("Pagina %d+", i/50+1), // Estimare
			})
			chapterNum++
		}
	}

	return chapters
}

// generateChapterSummaries generează rezumate pentru capitole - PRIMEȘTE TOT TEXTUL PDF
func generateChapterSummaries(text string, language string) ([]ChapterInfo, error) {
	fmt.Printf("📄 Detectez și generez rezumate pentru capitole...\n")

	// Detectează capitolele
	chapters := detectChapters(text)

	if len(chapters) == 0 {
		// Dacă nu găsim capitole, împărțim textul în secțiuni logice
		return generateLogicalSections(text, language)
	}

	// Generează rezumate pentru fiecare capitol detectat
	for i := range chapters {
		summary, err := generateSingleChapterSummary(text, chapters[i], language)
		if err != nil {
			fmt.Printf("⚠️ Eroare la generarea rezumatului pentru capitolul %d: %v\n", chapters[i].Number, err)
			chapters[i].Summary = "Eroare la generarea rezumatului"
			continue
		}
		chapters[i].Summary = summary
	}

	return chapters, nil
}

// generateLogicalSections împarte textul în secțiuni logice dacă nu sunt capitole
func generateLogicalSections(text string, language string) ([]ChapterInfo, error) {
	// Împarte textul în 3-5 secțiuni egale
	sections := 4
	textLen := len(text)
	sectionSize := textLen / sections

	var chapters []ChapterInfo

	for i := 0; i < sections; i++ {
		start := i * sectionSize
		end := start + sectionSize
		if i == sections-1 {
			end = textLen // Ultima secțiune ia tot ce rămâne
		}

		sectionText := text[start:end]

		// Generează rezumat pentru secțiune
		summary, err := generateSectionSummary(sectionText, i+1, language)
		if err != nil {
			summary = "Eroare la generarea rezumatului"
		}

		chapters = append(chapters, ChapterInfo{
			Number:  i + 1,
			Title:   fmt.Sprintf("Secțiunea %d", i+1),
			Pages:   fmt.Sprintf("Secțiunea %d din %d", i+1, sections),
			Summary: summary,
		})
	}

	return chapters, nil
}

// generateSingleChapterSummary generează rezumatul pentru un capitol specific
func generateSingleChapterSummary(fullText string, chapter ChapterInfo, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Analizează ÎNTREG documentul PDF și fă un rezumat pentru capitolul/secțiunea specificată.

ATENȚIE: Primești TOT TEXTUL PDF-ului și trebuie să identifici și să rezumi DOAR partea relevantă pentru:
CAPITOLUL: %s (Numărul %d)

LIMBA: %s (folosește diacritice corecte pentru română)

Instrucțiuni pentru REZUMAT CAPITOL:
- Identifică în textul complet partea care se referă la acest capitol
- Fă un rezumat moderat (5-8 propoziții) DOAR pentru acest capitol
- Concentrează-te pe ideile principale din acest capitol specific
- NU incluzi informații din alte capitole
- Stil profesional și clar

TEXT COMPLET PDF:
%s`, chapter.Title, chapter.Number, language, fullText)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.3,
		MaxTokens:   500,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert în analiza și rezumarea capitolelor din documente în limba %s.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	summary, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(summary), nil
}

// generateSectionSummary generează rezumatul pentru o secțiune logică
func generateSectionSummary(sectionText string, sectionNum int, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Fă un rezumat pentru această secțiune din document.

SECȚIUNEA: %d
LIMBA: %s (folosește diacritice corecte pentru română)

Instrucțiuni:
- Rezumat moderat (5-8 propoziții)
- Identifică ideile principale din această secțiune
- Stil profesional și clar

TEXT SECȚIUNE:
%s`, sectionNum, language, sectionText)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.3,
		MaxTokens:   400,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert în rezumarea secțiunilor de text în limba %s.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	summary, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(summary), nil
}

// generateGeneralSummary generează un rezumat general foarte scurt - PRIMEȘTE TOT TEXTUL PDF
func generateGeneralSummary(text string, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	// Pentru rezumatul general, folosim tot textul dar îl comprimăm inteligent
	// Luăm primele 3000 de caractere, mijlocul și ultimele 3000
	var textForSummary string
	textLen := len(text)

	if textLen <= 8000 {
		textForSummary = text
	} else {
		start := text[:3000]
		middle := text[textLen/2-1500 : textLen/2+1500]
		end := text[textLen-3000:]
		textForSummary = start + "\n\n[...mijloc document...]\n\n" + middle + "\n\n[...sfârșit document...]\n\n" + end
	}

	prompt := fmt.Sprintf(`Analizează ÎNTREG documentul PDF și fă un rezumat foarte concis și general. 

ATENȚIE: Primești TOT TEXTUL PDF-ului, nu doar un fragment!

LIMBA: %s (folosește diacritice corecte pentru română)

Instrucțiuni pentru REZUMAT GENERAL:
- Maxim 3-4 propoziții
- Doar ideile principale și tema centrală din ÎNTREG documentul
- Identifică subiectul principal al întregului PDF
- Stil profesional și clar
- NU menționează că este un fragment sau chunk

TEXT COMPLET PDF:
%s`, language, textForSummary)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.2,
		MaxTokens:   200,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert în rezumarea concisă de texte în limba %s.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	summary, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(summary), nil
}

// generateCustomGeneralSummary generează rezumat general personalizat (o linie sau o pagină)
func generateCustomGeneralSummary(text string, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	// Pentru rezumatul general, folosim tot textul dar îl comprimăm inteligent
	var textForSummary string
	textLen := len(text)

	if textLen <= 8000 {
		textForSummary = text
	} else {
		start := text[:3000]
		middle := text[textLen/2-1500 : textLen/2+1500]
		end := text[textLen-3000:]
		textForSummary = start + "\n\n[...mijloc document...]\n\n" + middle + "\n\n[...sfârșit document...]\n\n" + end
	}

	var instructions string
	var maxTokens int

	instructions = `
	Scrie-mi un rezumat profesional despre acest document:
	- Stil profesional și complet`

	maxTokens = 5000

	prompt := fmt.Sprintf(`Analizează ÎNTREG documentul PDF și fă un rezumat personalizat.

ATENȚIE: Primești TOT TEXTUL PDF-ului, nu doar un fragment!

LIMBA: %s (folosește diacritice corecte pentru română)

%s

IMPORTANT: NU menționează că este un fragment sau chunk. Analizează documentul complet!

TEXT COMPLET PDF:
%s`, language, instructions, textForSummary)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.2,
		MaxTokens:   maxTokens,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert în rezumarea concisă de texte în limba %s.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	summary, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(summary), nil
}

// generateChaptersPDF creează PDF pentru rezumatul pe capitole
func generateChaptersPDF(chapters []ChapterInfo, totalPages int, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Titlu
	pdf.Cell(0, 10, "Rezumat pe Capitole")
	pdf.Ln(15)

	// Informații generale
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pagini originale: %d", totalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Capitole detectate: %d", len(chapters)))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generat la: %s", time.Now().Format("02/01/2006 15:04")))
	pdf.Ln(15)

	// Capitole
	for _, chapter := range chapters {
		pdf.SetFont("Arial", "B", 14)
		pdf.Cell(0, 10, fmt.Sprintf("Capitol %d: %s", chapter.Number, chapter.Title))
		pdf.Ln(8)

		pdf.SetFont("Arial", "I", 10)
		pdf.Cell(0, 6, chapter.Pages)
		pdf.Ln(8)

		pdf.SetFont("Arial", "", 11)
		pdf.MultiCell(0, 6, chapter.Summary, "", "", false)
		pdf.Ln(10)

		// Pagină nouă la fiecare 2 capitole
		if chapter.Number%2 == 0 && chapter.Number < len(chapters) {
			pdf.AddPage()
		}
	}

	return pdf.OutputFileAndClose(filename)
}

// generateGeneralSummaryPDF creează PDF pentru rezumatul general
func generateGeneralSummaryPDF(summary string, totalPages int, oneLine bool, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Titlu
	title := "Rezumat General"
	if oneLine {
		title += " (O Linie)"
	} else {
		title += " (O Pagină)"
	}
	pdf.Cell(0, 10, title)
	pdf.Ln(15)

	// Informații
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pagini originale: %d", totalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generat la: %s", time.Now().Format("02/01/2006 15:04")))
	pdf.Ln(15)

	// Rezumat
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Rezumat:")
	pdf.Ln(10)

	if oneLine {
		pdf.SetFont("Arial", "", 14)
		pdf.MultiCell(0, 8, summary, "", "C", false) // Centrat pentru o linie
	} else {
		pdf.SetFont("Arial", "", 12)
		pdf.MultiCell(0, 7, summary, "", "", false)
	}

	return pdf.OutputFileAndClose(filename)
}

// generateLevelSummaryPDF creează PDF pentru rezumatul pe nivel
func generateLevelSummaryPDF(level SummaryLevel, totalPages int, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Titlu
	pdf.Cell(0, 10, fmt.Sprintf("Rezumat Nivel %d", level.Level))
	pdf.Ln(15)

	// Informații
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pagini originale: %d", totalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Nivel: %d", level.Level))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Pagini per chunk: %d", level.PagesPerChunk))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Descriere: %s", level.Description))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generat la: %s", time.Now().Format("02/01/2006 15:04")))
	pdf.Ln(15)

	// Rezumat
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Rezumat:")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 11)
	pdf.MultiCell(0, 6, level.Summary, "", "", false)

	return pdf.OutputFileAndClose(filename)
}

// generateSummaryPDF creează un PDF cu rezumatul
func generateSummaryPDF(result *SummaryResult, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Titlu
	pdf.Cell(0, 10, "Rezumat PDF")
	pdf.Ln(15)

	// Informații generale
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pagini originale: %d", result.OriginalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generat la: %s", result.GeneratedAt.Format("02/01/2006 15:04")))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Timp procesare: %s", result.ProcessingTime))
	pdf.Ln(12)

	// Rezumat general
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Rezumat General")
	pdf.Ln(8)
	pdf.SetFont("Arial", "", 11)
	pdf.MultiCell(0, 6, result.GeneralSummary, "", "", false)
	pdf.Ln(10)

	// Rezumate pe niveluri
	for _, level := range result.Levels {
		if level.Summary != "" {
			pdf.SetFont("Arial", "B", 12)
			pdf.Cell(0, 8, fmt.Sprintf("Nivel %d - %s", level.Level, level.Description))
			pdf.Ln(6)
			pdf.SetFont("Arial", "", 10)
			pdf.MultiCell(0, 5, level.Summary, "", "", false)
			pdf.Ln(8)

			// Pagină nouă la fiecare 2 niveluri pentru lizibilitate
			if level.Level%2 == 0 && level.Level < 10 {
				pdf.AddPage()
			}
		}
	}

	return pdf.OutputFileAndClose(filename)
}

// processSummaryRequest procesează cererea de rezumat
func processSummaryRequest(request SummaryRequest) (*SummaryResult, error) {
	startTime := time.Now()

	fmt.Printf("📄 Începe procesarea rezumatului pentru %d pagini...\n", request.TotalPages)

	language := request.Language
	if language == "" {
		language = "romanian"
	}

	result := &SummaryResult{
		OriginalPages: request.TotalPages,
		GeneratedAt:   startTime,
	}

	// 1. Generează rezumatul general (primește TOT textul PDF)
	fmt.Printf("📄 Generez rezumatul general cu TOT textul PDF...\n")
	generalSummary, err := generateGeneralSummary(request.Text, language)
	if err != nil {
		return nil, fmt.Errorf("failed to generate general summary: %v", err)
	}
	result.GeneralSummary = generalSummary

	// 2. Generează rezumate pe capitole (primește TOT textul PDF)
	if request.IncludeChapters {
		fmt.Printf("📄 Generez rezumate pe capitole cu TOT textul PDF...\n")
		chapterSummaries, err := generateChapterSummaries(request.Text, language)
		if err != nil {
			fmt.Printf("⚠️ Eroare la generarea rezumatelor pe capitole: %v\n", err)
		} else {
			result.ChapterSummary = chapterSummaries
		}
	}

	// 3. Calculează nivelurile de rezumat (vor lucra cu chunk-uri)
	levels := calculateSummaryLevels(request.TotalPages)

	// 4. Generează rezumate pentru fiecare nivel (lucrează cu chunk-uri)
	fmt.Printf("📄 Generez rezumate pe niveluri (chunk-uri)...\n")
	for i := range levels {
		summary, err := generateLevelSummary(request.Text, request.TotalPages, levels[i], language)
		if err != nil {
			fmt.Printf("⚠️ Eroare la nivelul %d: %v\n", levels[i].Level, err)
			continue
		}
		levels[i].Summary = summary
	}

	result.Levels = levels
	result.ProcessingTime = time.Since(startTime).String()

	fmt.Printf("✅ Rezumat generat cu succes în %s\n", result.ProcessingTime)
	return result, nil
}

// saveSummaryResult salvează rezultatul într-un fișier JSON
func saveSummaryResult(result *SummaryResult, filename string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// detectLanguageFromText detectează limba din textul PDF folosind AI
func detectLanguageFromText(text string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "romanian", nil // Default fallback
	}

	// Folosește primele 2000 de caractere pentru detecția limbii
	sampleText := text
	if len(text) > 2000 {
		sampleText = text[:2000]
	}

	prompt := fmt.Sprintf(`Detectează limba principală din următorul text și returnează DOAR numele limbii în engleză.

Răspunde cu UNA din următoarele opțiuni exacte:
- romanian
- english  
- spanish
- french
- german
- italian

Returnează DOAR numele limbii, fără explicații.

TEXT:
%s`, sampleText)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.1,
		MaxTokens:   10,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: "Ești un expert în detectarea limbilor. Răspunzi doar cu numele limbii în engleză.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	response, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		fmt.Printf("⚠️ Eroare la detectarea limbii: %v, folosesc română ca default\n", err)
		return "romanian", nil
	}

	language := strings.ToLower(strings.TrimSpace(response))

	// Validează răspunsul
	validLanguages := []string{"romanian", "english", "spanish", "french", "german", "italian"}
	for _, valid := range validLanguages {
		if language == valid {
			fmt.Printf("🌍 Limbă detectată: %s\n", language)
			return language, nil
		}
	}

	fmt.Printf("🌍 Limbă nedeterminată, folosesc română ca default\n")
	return "romanian", nil
}
