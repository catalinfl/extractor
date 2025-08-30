package main

import (
	"fmt"
	"strconv"
	"strings"

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

// Split pages into paragraphs based on the specified grade (2-10)
func splitPagesIntoParagraphs(pages []string, grade int) []string {
	if grade < 2 || grade > 10 {
		return pages // Return original if invalid grade
	}

	var paragraphs []string

	for pageNum, pageText := range pages {
		// Clean up the page text
		cleanText := strings.TrimSpace(pageText)
		if len(cleanText) == 0 {
			continue // Skip empty pages
		}

		// Calculate paragraph length
		textLength := len(cleanText)
		paragraphLength := textLength / grade
		if paragraphLength < 100 {
			paragraphLength = 100 // Minimum paragraph length
		}

		// Split the page into paragraphs
		for i := 0; i < grade; i++ {
			start := i * paragraphLength
			end := start + paragraphLength

			// Adjust for the last paragraph to include remaining text
			if i == grade-1 {
				end = textLength
			}

			// Make sure we don't go out of bounds
			if start >= textLength {
				break
			}
			if end > textLength {
				end = textLength
			}

			// Extract paragraph text
			paragraphText := cleanText[start:end]

			// Try to break at word boundaries for better readability
			if i < grade-1 && end < textLength {
				// Find last space within next 50 characters to avoid breaking words
				lastSpaceIndex := strings.LastIndex(paragraphText, " ")
				if lastSpaceIndex > paragraphLength-50 && lastSpaceIndex != -1 {
					paragraphText = paragraphText[:lastSpaceIndex]
					// Adjust the start position for next paragraph
					nextStart := start + lastSpaceIndex + 1
					paragraphLength = (textLength - nextStart) / (grade - i - 1)
				}
			}

			// Add paragraph with metadata
			paragraphText = strings.TrimSpace(paragraphText)
			if len(paragraphText) > 0 {
				// Add page and paragraph info at the beginning
				finalParagraph := fmt.Sprintf("[Page %d, Paragraph %d/%d]\n%s", pageNum+1, i+1, grade, paragraphText)
				paragraphs = append(paragraphs, finalParagraph)
			}
		}
	}

	return paragraphs
}

// Handler: Answer question using vector search + OpenRouter AI
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

	// Set default limit if not provided
	if req.Limit == 0 {
		req.Limit = 5
	}

	// Extract keywords to detect language
	keywordsResult, err := extractKeywords(req.Question)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to detect language: " + err.Error(),
		})
	}

	// First, search in vector database
	searchResults, err := searchPagesHybrid(req.Username, req.Question, req.DocName, req.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to search vector database: " + err.Error(),
		})
	}

	// Convert search results to text for AI processing
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

	// Generate answer using OpenRouter AI with detected language
	answer, err := answerFromVectorDB(req.Question, keywordsResult.Language, contextText.String())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate answer: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"answer":         answer,
		"language":       keywordsResult.Language,
		"sources_found":  len(searchResults),
		"search_results": searchResults,
	})
}

// Handler: Extract keywords from query for better search
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

	// Step 4: Generate answer using OpenRouter AI
	answer, err := answerFromVectorDB(req.Query, keywordsResult.Language, contextText.String())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate answer: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":            true,
		"answer":             answer,
		"keywords_extracted": keywordsResult.Query,
		"language_detected":  keywordsResult.Language,
		"enhanced_query":     enhancedQuery,
		"sources_found":      len(searchResults),
		"search_results":     searchResults,
	})
}
