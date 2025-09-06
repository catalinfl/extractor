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
	"sync"
	"time"
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
	DesiredLevel    int    `json:"desired_level,omitempty"` // 1..10, if 0 -> all levels
}

// calculateSummaryLevels calculează configurarea pentru fiecare nivel
func calculateSummaryLevels(totalPages int, desiredLevel int) SummaryLevel {
	// Clamp desiredLevel to maximum 4
	if desiredLevel <= 0 {
		desiredLevel = 1
	}
	if desiredLevel > 4 {
		desiredLevel = 4
	}

	makeLevel := func(level int) SummaryLevel {
		var pagesPerChunk int

		// Strategii diferite în funcție de numărul total de pagini (păstrăm comportamentul pentru nivelele 1..4)
		if totalPages <= 20 {
			switch level {
			case 1:
				pagesPerChunk = int(math.Max(1, float64(totalPages)/2))
			case 4:
				pagesPerChunk = int(math.Max(1, float64(totalPages)/4))
			default:
				ratio := float64(level-1) / 3
				pagesPerChunk = int(math.Max(1, float64(totalPages)*(0.1+ratio*0.4)))
			}
		} else if totalPages <= 100 {
			switch level {
			case 1:
				pagesPerChunk = int(math.Max(3, float64(totalPages)/3))
			case 4:
				pagesPerChunk = int(math.Max(3, float64(totalPages)/8))
			default:
				chunksTarget := 3 + (level - 1)
				pagesPerChunk = int(math.Max(2, float64(totalPages)/float64(chunksTarget)))
			}
		} else {
			switch level {
			case 1:
				pagesPerChunk = int(math.Max(5, float64(totalPages)/5))
			case 2:
				pagesPerChunk = int(math.Max(4, float64(totalPages)/8))
			case 3:
				pagesPerChunk = int(math.Max(3, float64(totalPages)/12))
			case 4:
				pagesPerChunk = int(math.Max(3, float64(totalPages)/15))
			}
		}

		if pagesPerChunk > totalPages {
			pagesPerChunk = totalPages
		}

		estimatedChunks := int(math.Ceil(float64(totalPages) / float64(pagesPerChunk)))

		return SummaryLevel{
			Level:         level,
			Description:   fmt.Sprintf("Rezumat nivel %d (%d pagini per chunk, ~%d chunks)", level, pagesPerChunk, estimatedChunks),
			PagesPerChunk: pagesPerChunk,
		}
	}

	return makeLevel(desiredLevel)
}

// chunkTextByPages împarte textul în chunk-uri bazate pe numărul de pagini
func chunkTextByPages(text string, totalPages int, pagesPerChunk int) []string {
	startTime := time.Now()

	if totalPages <= 0 || pagesPerChunk <= 0 {
		fmt.Printf("⏱️ chunkTextByPages: Invalid params, returning single chunk (took: %v)\n", time.Since(startTime))
		return []string{text}
	}

	// Estimează lungimea medie per pagină
	avgCharsPerPage := len(text) / totalPages
	chunkSize := avgCharsPerPage * pagesPerChunk

	fmt.Printf("📊 chunkTextByPages: text=%d chars, pages=%d, pagesPerChunk=%d, avgCharsPerPage=%d, chunkSize=%d\n",
		len(text), totalPages, pagesPerChunk, avgCharsPerPage, chunkSize)

	if chunkSize >= len(text) {
		fmt.Printf("⏱️ chunkTextByPages: Single chunk needed (took: %v)\n", time.Since(startTime))
		return []string{text}
	}

	var chunks []string
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := text[i:end]

		// Optimized sentence boundary detection - limit search to last 500 chars to avoid performance issues
		if end < len(text) {
			searchStart := len(chunk) - 500
			if searchStart < len(chunk)/2 {
				searchStart = len(chunk) / 2
			}

			searchChunk := chunk[searchStart:]
			lastDot := strings.LastIndex(searchChunk, ".")
			lastQuestion := strings.LastIndex(searchChunk, "?")
			lastExclamation := strings.LastIndex(searchChunk, "!")

			lastSentenceEnd := int(math.Max(float64(lastDot), math.Max(float64(lastQuestion), float64(lastExclamation))))
			if lastSentenceEnd > 0 {
				actualEnd := searchStart + lastSentenceEnd + 1
				chunk = chunk[:actualEnd]
				i = i + actualEnd - chunkSize // adjust index
			}
		}

		chunks = append(chunks, strings.TrimSpace(chunk))
	}

	duration := time.Since(startTime)
	fmt.Printf("⏱️ chunkTextByPages: Created %d chunks in %v\n", len(chunks), duration)

	return chunks
}

func generateChunkSummary(chunk string, chunkIndex int, totalChunks int, language string) (string, error) {
	startTime := time.Now()

	fmt.Printf("⏱️ [Chunk %d/%d] Starting chunk summary generation (%d chars)...\n", chunkIndex+1, totalChunks, len(chunk))

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	prompt := fmt.Sprintf(`Ești un expert în rezumarea textelor. Fă un rezumat profesional al acestui CHUNK de text.

ATENȚIE: Primești o PARTE (chunk %d din %d) dintr-un document mai mare pentru rezumat!

INFORMAȚII CONTEXT:
- Acesta este chunk-ul %d din %d pentru rezumat
- Limba: %s FOARTE FOARTE IMPORTANT!

INSTRUCȚIUNI IMPORTANTE:
- Acesta este UN FRAGMENT din rezumatul complet
- Rezumatul tău va fi UNIT direct cu alte chunk-uri (FĂRĂ procesare suplimentară)
- Incearca sa-l faci cat mai detaliat
- Scrie fluent și coerent pentru că va fi unit cu alte părți
- NU folosi introduceri ca "În acest fragment..." sau "Această parte..."
- Începe direct cu conținutul relevant

LIMBA: %s FOARTE FOARTE IMPORTANT!

TEXT CHUNK:
%s`, chunkIndex+1, totalChunks, chunkIndex+1, totalChunks, language, language, chunk)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.3,
		MaxTokens:   2000,
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

	startCall := time.Now()
	summary, err := callOpenRouter(reqBody, apiKey)
	callDuration := time.Since(startCall)
	totalDuration := time.Since(startTime)

	fmt.Printf("⏱️ [Chunk %d/%d] OpenRouter call took: %v, total chunk time: %v\n", chunkIndex+1, totalChunks, callDuration, totalDuration)

	if err != nil {
		return "", fmt.Errorf("failed to generate summary for chunk %d: %v", chunkIndex+1, err)
	}

	return strings.TrimSpace(summary), nil
}

// generateLevelSummary generează rezumatul pentru un nivel specific
func generateLevelSummary(text string, totalPages int, level SummaryLevel, language string) (string, error) {
	startTime := time.Now()
	fmt.Printf("📄 [LEVEL %d] Starting level summary generation (%d pagini per chunk)...\n", level.Level, level.PagesPerChunk)

	startChunking := time.Now()
	chunks := chunkTextByPages(text, totalPages, level.PagesPerChunk)
	chunkingDuration := time.Since(startChunking)
	fmt.Printf("⏱️ [LEVEL %d] Text chunking took: %v, created %d chunks\n", level.Level, chunkingDuration, len(chunks))

	// Process chunks in parallel with concurrency limit to avoid rate limiting
	const maxConcurrency = 200
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	summaries := make([]string, len(chunks))
	errors := make([]error, len(chunks))

	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, chunkText string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			fmt.Printf("📄 [LEVEL %d] Processing chunk %d/%d (size: %d chars) [PARALLEL]...\n", level.Level, index+1, len(chunks), len(chunkText))

			chunkStart := time.Now()
			summary, err := generateChunkSummary(chunkText, index, len(chunks), language)
			chunkDuration := time.Since(chunkStart)

			if err != nil {
				fmt.Printf("❌ [LEVEL %d] Error processing chunk %d: %v (took: %v)\n", level.Level, index+1, err, chunkDuration)
				errors[index] = err
			} else {
				fmt.Printf("⏱️ [LEVEL %d] Chunk %d/%d completed in: %v [PARALLEL]\n", level.Level, index+1, len(chunks), chunkDuration)
				summaries[index] = summary
			}
		}(i, chunk)
	}

	// Wait for all chunks to complete
	fmt.Printf("⏳ [LEVEL %d] Waiting for all %d chunks to complete...\n", level.Level, len(chunks))
	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			fmt.Printf("❌ [LEVEL %d] Failed at chunk %d: %v\n", level.Level, i+1, err)
			return "", err
		}
	}

	// Reunire directă a chunk-urilor FĂRĂ procesare suplimentară prin AI
	if len(summaries) == 1 {
		totalDuration := time.Since(startTime)
		fmt.Printf("⏱️ [LEVEL %d] Single chunk completed in total: %v\n", level.Level, totalDuration)
		return summaries[0], nil
	}

	// Combină chunk-urile direct cu separatori
	startCombining := time.Now()
	fmt.Printf("📄 [LEVEL %d] Combining %d chunks directly without AI processing...\n", level.Level, len(summaries))
	finalSummary := strings.Join(summaries, "\n\n")
	combiningDuration := time.Since(startCombining)
	totalDuration := time.Since(startTime)

	fmt.Printf("⏱️ [LEVEL %d] Combining took: %v, total level processing: %v\n", level.Level, combiningDuration, totalDuration)

	return finalSummary, nil
}

// generateChapterSummaries generates a list of chapters detected by AI and returns
// a structured JSON: []ChapterInfo. It sends the LLM the entire text (text) and asks it
// to detect the chapters, titles, page ranges (if possible), and a short summary
// for each chapter. The function sanitizes the response and decodes it.
func generateChapterSummaries(text string, language string) ([]ChapterInfo, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	// Prompt requires a json response with {number,title,pages,summary}
	prompt := fmt.Sprintf(`Ești un asistent care detectează capitolele și secțiunile dintr-un document.
Returnează DOAR un ARRAY JSON (începând cu '[') cu obiecte având exact câmpurile:
 - number (integer) -> numărul capitolului, în ordine
 - title (string) -> titlul capitolului (dacă nu are titlu, pune "Capitolul N")
 - pages (string) -> intervalul de pagini sau estimare (ex: "1-10")
 - summary (string) -> rezumat scurt al capitolului (5-8 propoziții)

Răspunde STRICT cu JSON, fără text explicativ, fără note, fără markdown.

LIMBA IN CARE RASPUNZI: %s !FOARTE IMPORTANT

TEXT COMPLET PDF (anexează tot textul următor):
%s
`, language, text)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.3,
		MaxTokens:   4000,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("Ești un expert care extrage capitole și rezumate din documente în limba %s. Returnezi doar JSON.", language),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	resp, err := callOpenRouter(reqBody, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenRouter for chapters: %v", err)
	}
	// Normalize response: remove common code fences and stray backticks
	raw := strings.TrimSpace(resp)
	raw = strings.ReplaceAll(raw, "```json", "")
	raw = strings.ReplaceAll(raw, "```", "")
	raw = strings.Trim(raw, "` \n\r\t")
	raw = strings.TrimSpace(raw)

	// Attempt to extract JSON array between first '[' and last ']' or fallback to object between '{' and '}'
	firstArray := strings.Index(raw, "[")
	lastArray := strings.LastIndex(raw, "]")

	var jsonPart string
	if firstArray != -1 && lastArray != -1 && lastArray > firstArray {
		jsonPart = raw[firstArray : lastArray+1]
	} else {
		// fallback: try a single object and wrap into array
		firstObj := strings.Index(raw, "{")
		lastObj := strings.LastIndex(raw, "}")
		if firstObj != -1 && lastObj != -1 && lastObj > firstObj {
			jsonPart = "[" + raw[firstObj:lastObj+1] + "]"
		} else {
			// give up extracting and use the whole cleaned raw
			jsonPart = raw
		}
	}

	// First attempt: sanitize JSON-friendly characters inside strings
	sanitized := sanitizeJSONString(jsonPart)

	var chapters []ChapterInfo
	if err := json.Unmarshal([]byte(sanitized), &chapters); err != nil {
		if err2 := json.Unmarshal([]byte(jsonPart), &chapters); err2 == nil {
		} else {
			orig := strings.TrimSpace(resp)
			orig = strings.ReplaceAll(orig, "```json", "")
			orig = strings.ReplaceAll(orig, "```", "")
			orig = strings.Trim(orig, "` \n\r\t")
			if err3 := json.Unmarshal([]byte(orig), &chapters); err3 == nil {
			} else {
				return nil, fmt.Errorf("failed to decode chapters JSON: %v; response: %s", err, resp)
			}
		}
	}

	for i := range chapters {
		if chapters[i].Number == 0 {
			chapters[i].Number = i + 1
		}
		if strings.TrimSpace(chapters[i].Title) == "" {
			chapters[i].Title = fmt.Sprintf("Capitolul %d", chapters[i].Number)
		}
		if strings.TrimSpace(chapters[i].Pages) == "" {
			chapters[i].Pages = "n/a"
		}
	}

	return chapters, nil
}

// generateGeneralSummary generează un rezumat general foarte scurt - PRIMEȘTE TOT TEXTUL PDF
func generateGeneralSummary(text string, language string) (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	var textForSummary string
	textLen := len(text)

	if textLen <= 9000 {
		textForSummary = text
	} else {
		start := text[:3000]
		middle := text[textLen/2-1500 : textLen/2+1500]
		end := text[textLen-3000:]
		textForSummary = start + "\n\n[...mijloc document...]\n\n" + middle + "\n\n[...sfârșit document...]\n\n" + end
	}

	prompt := fmt.Sprintf(`Analizează ÎNTREG documentul PDF și fă un rezumat foarte concis și general. 

ATENȚIE: Primești TOT TEXTUL PDF-ului, nu doar un fragment!

TREBUIE SCRIS NEAPARAT ÎN LIMBA: %s

Instrucțiuni pentru REZUMAT GENERAL:
- Doar ideile principale și tema centrală din ÎNTREG documentul
- Identifică subiectul principal al întregului PDF
- Stil profesional și clar, incearcă să fie cât mai lung, să exprimi cât mai mult
- NU menționează că este un fragment sau chunk

TEXT COMPLET PDF:
%s`, language, textForSummary)

	reqBody := OpenRouterRequest{
		Model:       OpenRouterModel,
		Temperature: 0.2,
		MaxTokens:   1000,
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

// detectLanguageFromText detects the language using AI Request
// func detectLanguageFromText(text string) (string, error) {
// 	apiKey := os.Getenv("OPENROUTER_API_KEY")
// 	if apiKey == "" {
// 		return "english", nil // Default fallback
// 	}

// 	sampleText := text
// 	if len(text) > 500 {
// 		sampleText = text[:500]
// 	}

// 	prompt := fmt.Sprintf(`Detectează limba principală din următorul text și returnează DOAR numele limbii în engleză.

// Răspunde cu UNA din următoarele opțiuni exacte, CA EXEMPLU:
// 	- english
// 	- romanian
// 	- french etc.
// Returnează DOAR numele limbii, fără explicații.

// TEXT:
// %s`, sampleText)

// 	reqBody := OpenRouterRequest{
// 		Model:       OpenRouterModel,
// 		Temperature: 0.1,
// 		MaxTokens:   10,
// 		Messages: []OpenRouterMessage{
// 			{
// 				Role:    "system",
// 				Content: "Ești un expert în detectarea limbilor. Răspunzi doar cu numele limbii în engleză.",
// 			},
// 			{
// 				Role:    "user",
// 				Content: prompt,
// 			},
// 		},
// 	}

// 	response, err := callOpenRouter(reqBody, apiKey)
// 	if err != nil {
// 		fmt.Printf("Error at detecting language: %v, using English as default\n", err)
// 		return "english", nil
// 	}

// 	language := strings.ToLower(strings.TrimSpace(response))

// 	fmt.Printf("Detected language: %s\n", language)

// 	return language, nil
// }

/*
NEEDS TESTS, MIGHT BE DEVELOPED IN FUTURE
COULD BE PROCESSED AS MULTIPLE SOLUTION
------------------------------------------

func processSummaryRequest(request SummaryRequest) (*SummaryResult, error) {
	startTime := time.Now()

	fmt.Printf("📄 Începe procesarea rezumatului pentru %d pagini...\n", request.TotalPages)

	language := request.Language
	if language == "" {
		language = "english"
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

	// 3. Calculează nivelul de rezumat selectat (vor lucra cu chunk-uri)
	selectedLevel := calculateSummaryLevels(request.TotalPages, request.DesiredLevel)

	// 4. Generează rezumat pentru nivelul selectat (lucrează cu chunk-uri)
	fmt.Printf("📄 Generez rezumat pentru nivelul %d (chunk-uri)...\n", selectedLevel.Level)
	summary, err := generateLevelSummary(request.Text, request.TotalPages, selectedLevel, language)
	if err != nil {
		fmt.Printf("⚠️ Eroare la generarea rezumatului la nivelul %d: %v\n", selectedLevel.Level, err)
	} else {
		selectedLevel.Summary = summary
	}

	result.Levels = []SummaryLevel{selectedLevel}
	result.ProcessingTime = time.Since(startTime).String()

	fmt.Printf("✅ Rezumat generat cu succes în %s\n", result.ProcessingTime)
	return result, nil
}
*/
