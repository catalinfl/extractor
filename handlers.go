package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// New handler: Extract and store in Qdrant
func handleExtractAndStore(c *fiber.Ctx) error {
	username := c.FormValue("username", "anon1")

	grade := c.FormValue("grade", "1")
	paragraphGrade := 1
	if grade != "1" {
		if g, err := strconv.Atoi(grade); err == nil && g >= 2 && g <= 10 {
			paragraphGrade = g
		}
	}

	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ExtractResponse{
			Success: false,
			Error:   err.Error(),
		})
	}

	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ExtractResponse{
			Success: false,
			Error:   "Failed to extract text: " + err.Error(),
		})
	}

	// Split pages into paragraphs if grade > 1
	var finalContent []string
	if paragraphGrade > 1 && fileType == "pdf" {
		finalContent = splitPagesIntoParagraphs(pages, paragraphGrade)
	} else {
		finalContent = pages
	}

	// Store in Qdrant using the actual filename
	storedInQdrant := false
	if err := storePagesInQdrant(username, finalContent, filename); err != nil {
		fmt.Printf("⚠️ Failed to store in Qdrant: %v\n", err)
	} else {
		storedInQdrant = true
	}

	return c.JSON(ExtractResponse{
		Success:        true,
		FileType:       fileType,
		Filename:       filename,
		NumPages:       len(finalContent),
		Pages:          finalContent,
		StoredInQdrant: storedInQdrant,
	})
}

type SearchPageInQdrant struct {
	Username string `json:"username"`
	Query    string `json:"query"`
	DocName  string `json:"doc_name,omitempty"` // Optional: filter by document name
	Limit    int    `json:"limit,omitempty"`
}

// New handler: Search pages by username and similarity
func handleSearchPages(c *fiber.Ctx) error {
	var req SearchPageInQdrant

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ParagraphSearchResponse{
			Success: false,
			Error:   "Invalid request format: " + err.Error(),
		})
	}

	if req.Username == "" {
		req.Username = "anon1" // Default username
	}
	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ParagraphSearchResponse{
			Success: false,
			Error:   "Query is required",
		})
	}

	if req.Limit <= 0 {
		req.Limit = 5 // Default limit
	}

	results, err := searchPagesHybrid(req.Username, req.Query, req.DocName, req.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ParagraphSearchResponse{
			Success: false,
			Error:   "Search failed: " + err.Error(),
		})
	}

	return c.JSON(ParagraphSearchResponse{
		Success:    true,
		Results:    results,
		Query:      req.Query,
		Username:   req.Username,
		TotalFound: len(results),
	})
}

type LeaveResponse struct {
	Success      bool   `json:"success"`
	Username     string `json:"username"`
	DeletedCount int    `json:"deleted_count,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Handler: Delete all user data from Qdrant
func handleOnLeave(c *fiber.Ctx) error {
	username := c.Params("username")
	if username == "" {
		return c.Status(fiber.StatusBadRequest).JSON(LeaveResponse{
			Success: false,
			Error:   "Username is required",
		})
	}

	deletedCount, err := onLeave(username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(LeaveResponse{
			Success:  false,
			Username: username,
			Error:    "Failed to delete user data: " + err.Error(),
		})
	}

	return c.JSON(LeaveResponse{
		Success:      true,
		Username:     username,
		DeletedCount: deletedCount,
	})
}

func splitPagesIntoParagraphs(pages []string, grade int) []string {
	if grade < 2 || grade > 10 {
		return pages // Return original if invalid grade
	}

	var paragraphs []string

	for pageNum, pageText := range pages {
		cleanText := strings.TrimSpace(pageText)
		if len(cleanText) == 0 {
			continue // Skip empty pages
		}

		textLength := len(cleanText)
		paragraphLength := textLength / grade
		if paragraphLength < 100 {
			paragraphLength = 100 // Minimum paragraph length
		}

		for i := 0; i < grade; i++ {
			start := i * paragraphLength
			end := start + paragraphLength

			if i == grade-1 {
				end = textLength
			}

			if start >= textLength {
				break
			}
			if end > textLength {
				end = textLength
			}

			paragraphText := cleanText[start:end]

			if i < grade-1 && end < textLength {
				lastSpaceIndex := strings.LastIndex(paragraphText, " ")
				if lastSpaceIndex > paragraphLength-50 && lastSpaceIndex != -1 {
					paragraphText = paragraphText[:lastSpaceIndex]
					nextStart := start + lastSpaceIndex + 1
					paragraphLength = (textLength - nextStart) / (grade - i - 1)
				}
			}

			paragraphText = strings.TrimSpace(paragraphText)
			if len(paragraphText) > 0 {
				finalParagraph := fmt.Sprintf("[Page %d, Paragraph %d/%d]\n%s", pageNum+1, i+1, grade, paragraphText)
				paragraphs = append(paragraphs, finalParagraph)
			}
		}
	}

	return paragraphs
}

func handleAnswerQuestion(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Question string `json:"question"`
		DocName  string `json:"doc_name,omitempty"`
		Limit    int    `json:"limit,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid JSON format: " + err.Error(),
		})
	}

	if req.Username == "" || req.Question == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Username and question are required",
		})
	}

	if req.Limit == 0 {
		req.Limit = 5
	}

	keywordsResult, err := extractKeywords(req.Question)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to detect language: " + err.Error(),
		})
	}

	searchResults, err := searchPagesHybrid(req.Username, req.Question, req.DocName, req.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to search vector database: " + err.Error(),
		})
	}

	var contextText strings.Builder
	for i, result := range searchResults {
		contextText.WriteString(fmt.Sprintf("Document %d (Score: %.3f):\n%s\n\n",
			i+1, result.Score, result.Payload.Text))
	}

	if contextText.Len() == 0 {
		return c.JSON(fiber.Map{
			"success": true,
			"answer":  "Nu am găsit informații relevante pentru întrebarea ta în documentele încărcate.",
		})
	}

	answerResult, err := answerFromVectorDB(req.Question, keywordsResult.Language, contextText.String())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate answer: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"answer":         answerResult.Answer,
		"foundAnswer":    answerResult.FoundAnswer,
		"language":       keywordsResult.Language,
		"sources_found":  len(searchResults),
		"search_results": searchResults,
	})
}

func handleExtractKeywords(c *fiber.Ctx) error {
	var req struct {
		Query string `json:"query"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid JSON format: " + err.Error(),
		})
	}

	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Query is required",
		})
	}

	keywordsResult, err := extractKeywords(req.Query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract keywords: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"query":    keywordsResult.Query,
		"language": keywordsResult.Language,
	})
}

// Handler: Smart search - Extract keywords with AI, search in Qdrant, and return AI answer
func handleSmartSearch(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Query    string `json:"query"`
		DocName  string `json:"doc_name,omitempty"`
		Limit    int    `json:"limit,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid JSON format: " + err.Error(),
		})
	}

	if req.Username == "" || req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Username and query are required",
		})
	}

	// Set default limit if not provided
	if req.Limit == 0 {
		req.Limit = 5
	}

	// Step 1: Extract keywords using AI
	keywordsResult, err := extractKeywords(req.Query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract keywords: " + err.Error(),
		})
	}

	// Step 2: Use keywords for enhanced search in Qdrant
	// Combine original query with extracted keywords for better search
	enhancedQuery := req.Query
	if keywordsResult.Query != "" {
		enhancedQuery = req.Query + " " + keywordsResult.Query
	}

	searchResults, err := searchPagesHybrid(req.Username, enhancedQuery, req.DocName, req.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to search vector database: " + err.Error(),
		})
	}

	// Step 3: Convert search results to text for AI processing
	var contextText strings.Builder
	for i, result := range searchResults {
		contextText.WriteString(fmt.Sprintf("Document %d (Score: %.3f):\n%s\n\n",
			i+1, result.Score, result.Payload.Text))
	}

	if contextText.Len() == 0 {
		return c.JSON(fiber.Map{
			"success":            true,
			"answer":             "Nu am găsit informații relevante pentru întrebarea ta în documentele încărcate.",
			"keywords_extracted": keywordsResult.Query,
			"language_detected":  keywordsResult.Language,
			"sources_found":      0,
		})
	}

	answerResult, err := answerFromVectorDB(req.Query, keywordsResult.Language, contextText.String())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate answer: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":            true,
		"answer":             answerResult.Answer,
		"foundAnswer":        answerResult.FoundAnswer,
		"keywords_extracted": keywordsResult.Query,
		"language_detected":  keywordsResult.Language,
		"enhanced_query":     enhancedQuery,
		"sources_found":      len(searchResults),
		"search_results":     searchResults,
	})
}

// Handler for generating PDF summary
// 1. HANDLER PENTRU REZUMAT PE CAPITOLE - PRIMEȘTE PDF CA FORMFILE
func handleChapterSummary(c *fiber.Ctx) error {
	// Extract PDF file from form
	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get PDF file: " + err.Error(),
		})
	}

	if fileType != "pdf" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Only PDF files are supported",
		})
	}

	// Extract text from PDF
	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract text from PDF: " + err.Error(),
		})
	}

	// Combine all pages into one text
	fullText := strings.Join(pages, "\n\n")
	totalPages := len(pages)

	fmt.Printf("📚 Generez rezumat pe capitole pentru %d pagini din %s...\n", totalPages, filename)

	// Optional: allow client to force the language via form field `language`
	language := c.FormValue("language", "english")

	// Generate chapter summaries
	chapters, err := generateChapterSummaries(fullText, language)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate chapter summary: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"type":           "chapter_summary",
		"filename":       filename,
		"original_pages": totalPages,
		"language":       language,
		"chapters":       chapters,
		"total_chapters": len(chapters),
	})
}

// 2. HANDLER PENTRU REZUMAT GENERAL - PRIMEȘTE PDF CA FORMFILE
func handleGeneralSummary(c *fiber.Ctx) error {
	// Get one_line parameter from form

	// Extract PDF file from form
	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get PDF file: " + err.Error(),
		})
	}

	if fileType != "pdf" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Only PDF files are supported",
		})
	}

	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract text from PDF: " + err.Error(),
		})
	}

	fullText := strings.Join(pages, "\n\n")
	totalPages := len(pages)

	fmt.Printf("🎯 Generez rezumat general pentru %d pagini din %s...\n", totalPages, filename)

	language := c.FormValue("language", "english")

	summary, err := generateGeneralSummary(fullText, language)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate general summary: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"type":           "general_summary",
		"filename":       filename,
		"original_pages": totalPages,
		"language":       language,
		"summary":        summary,
	})
}

func handleLevelSummary(c *fiber.Ctx) error {
	// Get level parameter from form
	levelStr := c.FormValue("level", "1")
	level, err := strconv.Atoi(levelStr)
	if err != nil || level < 1 || level > 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Level must be a number between 1 and 10",
		})
	}

	// Extract PDF file from form
	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get PDF file: " + err.Error(),
		})
	}

	if fileType != "pdf" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Only PDF files are supported",
		})
	}

	// Extract text from PDF
	startExtract := time.Now()
	pages, err := extractTextPages(fileData, fileType)
	extractDuration := time.Since(startExtract)
	fmt.Printf("⏱️ PDF extraction took: %v\n", extractDuration)
	fmt.Printf("📄 Extracted %d pages\n", len(pages))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract text from PDF: " + err.Error(),
		})
	}

	// Combine all pages into one text
	startCombine := time.Now()
	fullText := strings.Join(pages, "\n\n")
	totalPages := len(pages)
	combineDuration := time.Since(startCombine)
	fmt.Printf("⏱️ Text combination took: %v, total chars: %d\n", combineDuration, len(fullText))

	fmt.Printf("📊 Generez rezumat nivel %d pentru %d pagini din %s...\n", level, totalPages, filename)

	// Optional: allow client to force the language via form field `language`
	language := c.FormValue("language", "english")

	// Calculate configuration for selected level
	startConfig := time.Now()
	selectedLevel := calculateSummaryLevels(totalPages, level)
	configDuration := time.Since(startConfig)
	fmt.Printf("⏱️ Level calculation took: %v, chunks will be: %d pages per chunk\n", configDuration, selectedLevel.PagesPerChunk)

	// Generate summary for selected level only
	startSummary := time.Now()
	summary, err := generateLevelSummary(fullText, totalPages, selectedLevel, language)
	summaryDuration := time.Since(startSummary)
	fmt.Printf("⏱️ Summary generation took: %v\n", summaryDuration)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate level summary: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"type":           "level_summary",
		"filename":       filename,
		"original_pages": totalPages,
		"language":       language,
		"level":          selectedLevel,
		"summary":        summary,
	})
}

// HANDLER PENTRU DESCĂRCARE PDF CAPITOLE - PRIMEȘTE PDF CA FORMFILE
func handleDownloadChapterSummaryPDF(c *fiber.Ctx) error {
	// Extract PDF file from form
	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get PDF file: " + err.Error(),
		})
	}

	if fileType != "pdf" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Only PDF files are supported",
		})
	}

	// Extract text from PDF
	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract text from PDF: " + err.Error(),
		})
	}

	// Combine all pages into one text
	fullText := strings.Join(pages, "\n\n")
	totalPages := len(pages)

	language := c.FormValue("language", "english")

	// Generate chapters
	chapters, err := generateChapterSummaries(fullText, language)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate chapter summary: " + err.Error(),
		})
	}

	// Create PDF for chapters
	pdfFilename := fmt.Sprintf("tmp/chapters_%d.pdf", time.Now().Unix())
	err = generateChaptersPDF(chapters, totalPages, pdfFilename)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate PDF: " + err.Error(),
		})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"capitole_%s_%d.pdf\"", strings.TrimSuffix(filename, ".pdf"), time.Now().Unix()))

	return c.SendFile(pdfFilename)
}

func handleDownloadGeneralSummaryPDF(c *fiber.Ctx) error {
	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get PDF file: " + err.Error(),
		})
	}

	if fileType != "pdf" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Only PDF files are supported",
		})
	}

	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract text from PDF: " + err.Error(),
		})
	}

	fullText := strings.Join(pages, "\n\n")
	totalPages := len(pages)

	language := c.FormValue("language", "english")

	// Generate general summary
	summary, err := generateGeneralSummary(fullText, language)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate general summary: " + err.Error(),
		})
	}

	// Create PDF for general summary
	pdfFilename := fmt.Sprintf("tmp/general_%d.pdf", time.Now().Unix())
	err = generateGeneralSummaryPDF(summary, totalPages, pdfFilename)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate PDF: " + err.Error(),
		})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"general_%s_%d.pdf\"", strings.TrimSuffix(filename, ".pdf"), time.Now().Unix()))

	return c.SendFile(pdfFilename)
}

// HANDLER PENTRU DESCĂRCARE PDF NIVEL - PRIMEȘTE PDF CA FORMFILE
func handleDownloadLevelSummaryPDF(c *fiber.Ctx) error {
	// Get level parameter from form
	levelStr := c.FormValue("level", "1")
	level, err := strconv.Atoi(levelStr)
	if err != nil || level < 1 || level > 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Level must be a number between 1 and 10",
		})
	}

	// Extract PDF file from form
	fileData, fileType, filename, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get PDF file: " + err.Error(),
		})
	}

	if fileType != "pdf" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Only PDF files are supported",
		})
	}

	// Extract text from PDF
	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to extract text from PDF: " + err.Error(),
		})
	}

	// Combine all pages into one text
	fullText := strings.Join(pages, "\n\n")
	totalPages := len(pages)

	language := c.FormValue("language", "english")

	// Calculate and generate level
	selectedLevel := calculateSummaryLevels(totalPages, level)

	summary, err := generateLevelSummary(fullText, totalPages, selectedLevel, language)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate level summary: " + err.Error(),
		})
	}

	selectedLevel.Summary = summary

	// Create PDF for level
	pdfFilename := fmt.Sprintf("tmp/level_%d_%d.pdf", level, time.Now().Unix())
	err = generateLevelSummaryPDF(selectedLevel, totalPages, pdfFilename)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate PDF: " + err.Error(),
		})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"nivel_%d_%s_%d.pdf\"", level, strings.TrimSuffix(filename, ".pdf"), time.Now().Unix()))

	return c.SendFile(pdfFilename)
}
