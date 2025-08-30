package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
)

const QdrantURL = "https://qdrant-production-449a.up.railway.app"
const OpenAIAPIURL = "https://api.openai.com/v1/embeddings"

// OpenAI Embedding Request/Response structures
type OpenAIEmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
}

type OpenAIEmbeddingResponse struct {
	Data []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Qdrant Collection and Vector Configuration
type QdrantCollection struct {
	Vectors VectorConfig `json:"vectors"` // Simple vector config, not named vectors
}

type VectorConfig struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

// Vector Point for Qdrant with simple vectors
type QdrantPoint struct {
	ID      string      `json:"id"`
	Vector  []float32   `json:"vector"` // Simple vector array
	Payload interface{} `json:"payload"`
}

// Page payload structure
type QdrantPage struct {
	Username string `json:"username"`
	Text     string `json:"text"`
	PageNum  int    `json:"page_num"`
	DocName  string `json:"doc_name,omitempty"`
}

// Search request structure
type SearchRequest struct {
	// Simple vector array for simple vector collections
	Vector      []float32   `json:"vector"`
	Filter      interface{} `json:"filter,omitempty"`
	Limit       int         `json:"limit"`
	WithPayload bool        `json:"with_payload,omitempty"`
}

// Search response structure
type SearchResponse struct {
	Result []SearchResult `json:"result"`
}

// SearchResult uses string ID (we store UUID strings) and typed payload
type SearchResult struct {
	ID      string     `json:"id"`
	Score   float32    `json:"score"`
	Payload QdrantPage `json:"payload"`
}

// Store pages in Qdrant with OpenAI embeddings
func storePagesInQdrant(username string, pages []string, docName string) error {
	var allPages []string
	var pagePayload []QdrantPage

	// Create pages with 20% overlap for better context preservation
	pagesWithOverlap := createPagesWithOverlap(pages, 0.2) // 20% overlap

	// Collect all pages and their metadata
	for pageNum, page := range pagesWithOverlap {
		if strings.TrimSpace(page) == "" || len(page) < 20 {
			continue // Skip empty pages
		}

		allPages = append(allPages, page)
		pagePayload = append(pagePayload, QdrantPage{
			Username: username,
			Text:     page,
			PageNum:  pageNum + 1,
			DocName:  docName,
		})
	}

	if len(allPages) == 0 {
		return fmt.Errorf("no pages found to store")
	}

	// Get embeddings from OpenAI in batches
	embeddings, err := getOpenAIEmbeddings(allPages)
	if err != nil {
		return fmt.Errorf("failed to get OpenAI embeddings: %v", err)
	}

	if len(embeddings) != len(allPages) {
		return fmt.Errorf("mismatch between pages (%d) and embeddings (%d)", len(allPages), len(embeddings))
	}

	// Create Qdrant points with UUID IDs and named vectors
	var points []QdrantPoint
	for i, embedding := range embeddings {
		// Generate UUID for unique ID
		pointID := uuid.New().String()

		point := QdrantPoint{
			ID:      pointID,
			Vector:  embedding, // Simple vector array
			Payload: pagePayload[i],
		}
		points = append(points, point)
	}

	fmt.Printf("📤 Uploading %d pages to Qdrant...\n", len(points))

	// Upload all points in a single batch request for better performance
	batchPayload := map[string]interface{}{
		"points": points,
	}

	payload, err := json.Marshal(batchPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal batch payload: %v", err)
	}

	// Use wait=true parameter to ensure operation completes
	url := fmt.Sprintf("%s/collections/pages/points?wait=true", QdrantURL)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create batch request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload batch: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("batch upload failed: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	fmt.Printf("✅ Successfully uploaded all %d pages with OpenAI embeddings for user '%s' in Qdrant\n", len(points), username)
	return nil
}

// Search pages by username and similarity using OpenAI embeddings
func searchPages(username, query, docName string, limit int) ([]SearchResult, error) {
	// Generate embedding for search query using OpenAI
	queryEmbeddings, err := getOpenAIEmbeddings([]string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to get query embedding: %v", err)
	}

	if len(queryEmbeddings) == 0 {
		return nil, fmt.Errorf("no embedding generated for query")
	}

	queryVector := queryEmbeddings[0]

	// Increase limit for hybrid search (get more results to filter)
	searchLimit := limit * 3
	if searchLimit < 10 {
		searchLimit = 10
	}

	// Create filter conditions - always include username
	filterConditions := []map[string]interface{}{
		{
			"key": "username",
			"match": map[string]string{
				"value": username,
			},
		},
	}

	// Add doc_name filter if specified
	if docName != "" {
		filterConditions = append(filterConditions, map[string]interface{}{
			"key": "doc_name",
			"match": map[string]string{
				"value": docName,
			},
		})
	}

	// Create search request with username filter (simple vector collection)
	searchReq := SearchRequest{
		Vector:      queryVector, // Direct vector array
		WithPayload: true,
		Filter: map[string]interface{}{
			"must": filterConditions,
		},
		Limit: searchLimit,
	}

	payload, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %v", err)
	}

	// Log the search request for debugging
	fmt.Printf("🔍 Search Request: %s\n", string(payload))

	// Use simple search endpoint for simple vector collections
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/pages/points/search", QdrantURL), bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed toa execute search: %v", err)
	}
	defer resp.Body.Close()

	// Read response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Log the full response for debugging
	fmt.Printf("🔍 Search Response: Status %d, Body: %s\n", resp.StatusCode, string(bodyBytes))

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search failed: status %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var searchResp SearchResponse
	if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %v", err)
	}

	// Hybrid search: Filter results by text similarity for more precise matches
	filteredResults := filterByTextSimilarity(searchResp.Result, query, limit)

	return filteredResults, nil
}

// Filter search results by text similarity to improve precision for name searches
func filterByTextSimilarity(results []SearchResult, query string, limit int) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// Convert query to lowercase for case-insensitive matching
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)

	// Score results based on exact matches and partial matches
	type scoredResult struct {
		result    SearchResult
		textScore float32
	}

	var scoredResults []scoredResult

	for _, result := range results {
		textLower := strings.ToLower(result.Payload.Text)
		textScore := float32(0)

		// Exact match gets highest score
		if strings.Contains(textLower, queryLower) {
			textScore = 1.0
		} else {
			// Partial word matches
			matchedWords := 0
			for _, word := range queryWords {
				if len(word) > 2 && strings.Contains(textLower, word) {
					matchedWords++
				}
			}
			if len(queryWords) > 0 {
				textScore = float32(matchedWords) / float32(len(queryWords))
			}
		}

		// Combine semantic score with text score
		combinedScore := result.Score*0.7 + textScore*0.3

		scoredResults = append(scoredResults, scoredResult{
			result: SearchResult{
				ID:      result.ID,
				Score:   combinedScore,
				Payload: result.Payload,
			},
			textScore: textScore,
		})
	}

	// Sort by combined score (descending)
	for i := 0; i < len(scoredResults)-1; i++ {
		for j := i + 1; j < len(scoredResults); j++ {
			if scoredResults[i].result.Score < scoredResults[j].result.Score {
				scoredResults[i], scoredResults[j] = scoredResults[j], scoredResults[i]
			}
		}
	}

	// Return top results up to limit
	var finalResults []SearchResult
	for i := 0; i < len(scoredResults) && i < limit; i++ {
		finalResults = append(finalResults, scoredResults[i].result)
	}

	return finalResults
}

// Get OpenAI embeddings for multiple texts
func getOpenAIEmbeddings(texts []string) ([][]float32, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	// OpenAI has a limit on batch size, process in chunks of 100
	const maxBatchSize = 100
	var allEmbeddings [][]float32

	for i := 0; i < len(texts); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		batchEmbeddings, err := getOpenAIEmbeddingsBatch(batch, apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get embeddings for batch %d-%d: %v", i, end-1, err)
		}

		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
}

// Get embeddings for a single batch from OpenAI
func getOpenAIEmbeddingsBatch(texts []string, apiKey string) ([][]float32, error) {
	reqBody := OpenAIEmbeddingRequest{
		Input:          texts,
		Model:          "text-embedding-3-small", // Fast, efficient, 1536 dimensions
		EncodingFormat: "float",
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %v", err)
	}

	req, err := http.NewRequest("POST", OpenAIAPIURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OpenAI API returned status %d", resp.StatusCode)
	}

	var embeddingResp OpenAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode embedding response: %v", err)
	}

	// Extract embeddings in the same order as input
	embeddings := make([][]float32, len(texts))
	for _, data := range embeddingResp.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
		}
	}

	fmt.Printf("🔮 Generated %d OpenAI embeddings (tokens: %d)\n", len(embeddings), embeddingResp.Usage.TotalTokens)
	return embeddings, nil
}

// Hybrid search combining keyword matching with semantic search
func searchPagesHybrid(username, query, docName string, limit int) ([]SearchResult, error) {
	fmt.Printf("🔍 Starting hybrid search for: '%s'\n", query)

	// First: Try exact text matching using scroll endpoint
	keywordResults, keywordErr := searchPagesKeyword(username, query, docName, limit)
	if keywordErr != nil {
		fmt.Printf("⚠️ Keyword search failed: %v\n", keywordErr)
	} else {
		fmt.Printf("✅ Keyword search found %d results\n", len(keywordResults))
	}

	// If keyword search found exact matches, prioritize them
	if len(keywordResults) > 0 {
		// Check if any result contains the exact query
		for _, result := range keywordResults {
			if strings.Contains(strings.ToLower(result.Payload.Text), strings.ToLower(query)) {
				fmt.Printf("🎯 Found exact match in keyword results\n")
				return keywordResults, nil
			}
		}
	}

	// Second: Semantic search for broader context
	semanticResults, semanticErr := searchPages(username, query, docName, limit)
	if semanticErr != nil {
		fmt.Printf("⚠️ Semantic search failed: %v\n", semanticErr)
		// If semantic fails but keyword worked, return keyword results
		if len(keywordResults) > 0 {
			return keywordResults, nil
		}
		return nil, semanticErr
	}

	fmt.Printf("✅ Semantic search found %d results\n", len(semanticResults))

	// Combine results, prioritizing exact matches
	combined := combineSearchResults(keywordResults, semanticResults, limit)

	fmt.Printf("🔗 Combined search returned %d total results\n", len(combined))
	return combined, nil
}

// Keyword-based search using text matching
func searchPagesKeyword(username, query, docName string, limit int) ([]SearchResult, error) {
	// Create filter conditions - always include username
	filterConditions := []map[string]interface{}{
		{
			"key": "username",
			"match": map[string]string{
				"value": username,
			},
		},
	}

	// Add doc_name filter if specified
	if docName != "" {
		filterConditions = append(filterConditions, map[string]interface{}{
			"key": "doc_name",
			"match": map[string]string{
				"value": docName,
			},
		})
	}

	// Use scroll endpoint with text filter for exact matching
	scrollReq := map[string]interface{}{
		"filter": map[string]interface{}{
			"must": filterConditions,
		},
		"limit":        limit * 10, // Get more results to search through
		"with_payload": true,
	}

	payload, err := json.Marshal(scrollReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scroll request: %v", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/pages/points/scroll", QdrantURL), bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create scroll request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute scroll: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read scroll response: %v", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("scroll failed: status %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var scrollResp struct {
		Result struct {
			Points []SearchResult `json:"points"`
		} `json:"result"`
	}

	if err := json.Unmarshal(bodyBytes, &scrollResp); err != nil {
		return nil, fmt.Errorf("failed to decode scroll response: %v", err)
	}

	// Filter results that contain the query text
	var filtered []SearchResult
	queryLower := strings.ToLower(query)

	for _, point := range scrollResp.Result.Points {
		textLower := strings.ToLower(point.Payload.Text)
		if strings.Contains(textLower, queryLower) {
			filtered = append(filtered, point)
			if len(filtered) >= limit {
				break
			}
		}
	}

	return filtered, nil
}

// Combine and deduplicate search results
func combineSearchResults(keywordResults, semanticResults []SearchResult, limit int) []SearchResult {
	seen := make(map[string]bool)
	var combined []SearchResult

	// First add keyword results (exact matches have priority)
	for _, result := range keywordResults {
		if !seen[result.ID] && len(combined) < limit {
			combined = append(combined, result)
			seen[result.ID] = true
		}
	}

	// Then add semantic results if there's still space
	for _, result := range semanticResults {
		if !seen[result.ID] && len(combined) < limit {
			combined = append(combined, result)
			seen[result.ID] = true
		}
	}

	return combined
}

// Delete all data for a specific user from Qdrant using filter-based deletion
func onLeave(username string) (int, error) {
	if username == "" {
		return 0, fmt.Errorf("username cannot be empty")
	}

	fmt.Printf("🗑️ Starting cleanup for user: '%s'\n", username)

	// Use filter-based deletion to delete all points for this user in one request
	deleteReq := map[string]interface{}{
		"filter": map[string]interface{}{
			"must": []map[string]interface{}{
				{
					"key": "username",
					"match": map[string]string{
						"value": username,
					},
				},
			},
		},
		"wait": true, // Wait for operation to complete
	}

	payload, err := json.Marshal(deleteReq)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal delete request: %v", err)
	}

	url := fmt.Sprintf("%s/collections/pages/points/delete", QdrantURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return 0, fmt.Errorf("failed to create delete request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute delete request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read delete response: %v", err)
	}

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("delete failed: status %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response to get the number of deleted points
	var deleteResp struct {
		Result struct {
			Operation struct {
				Deleted int `json:"deleted"`
			} `json:"operation"`
		} `json:"result"`
	}

	if err := json.Unmarshal(bodyBytes, &deleteResp); err != nil {
		fmt.Printf("⚠️ Could not parse delete response, but operation succeeded\n")
		fmt.Printf("✅ Successfully deleted all data for user '%s'\n", username)
		return -1, nil // Return -1 to indicate success but unknown count
	}

	deletedCount := deleteResp.Result.Operation.Deleted
	if deletedCount == 0 {
		fmt.Printf("ℹ️ No data found for user '%s'\n", username)
	} else {
		fmt.Printf("✅ Successfully deleted %d points for user '%s'\n", deletedCount, username)
	}

	return deletedCount, nil
}

// createPagesWithOverlap creates pages with specified overlap percentage
// overlap should be between 0.0 and 1.0 (e.g., 0.2 for 20% overlap)
func createPagesWithOverlap(pages []string, overlap float64) []string {
	// First, remove empty pages
	var cleanPages []string
	for _, page := range pages {
		trimmed := strings.TrimSpace(page)
		if trimmed != "" {
			cleanPages = append(cleanPages, trimmed)
		}
	}

	if len(cleanPages) <= 1 {
		return cleanPages // No overlap needed for single page
	}

	var result []string

	for i, currentPage := range cleanPages {
		var pageWithOverlap strings.Builder

		// Add overlap from PREVIOUS page (prefix) - last 20% or 200 chars
		if i > 0 {
			prevPage := cleanPages[i-1]

			// Calculate overlap size - either 20% or 200 chars, whichever is smaller
			overlapSize := int(float64(len(prevPage)) * overlap)
			if overlapSize > 200 {
				overlapSize = 200
			}
			if overlapSize < 50 {
				overlapSize = 50 // minimum meaningful overlap
			}

			if len(prevPage) >= overlapSize {
				// Take the last X characters of previous page
				startPos := len(prevPage) - overlapSize
				overlapText := prevPage[startPos:]

				// Find a good word boundary to start from
				if idx := strings.Index(overlapText, " "); idx != -1 && idx < 50 {
					overlapText = overlapText[idx+1:]
				}

				pageWithOverlap.WriteString(strings.TrimSpace(overlapText))
				pageWithOverlap.WriteString(" ")
			}
		}

		// Add the current page content
		pageWithOverlap.WriteString(currentPage)

		// Add overlap from NEXT page (suffix) - first 200 chars
		if i < len(cleanPages)-1 {
			nextPage := cleanPages[i+1]
			overlapSize := 200

			if len(nextPage) > overlapSize {
				overlapText := nextPage[:overlapSize]

				// Find a good word boundary to end at
				if idx := strings.LastIndex(overlapText, " "); idx > 150 {
					overlapText = overlapText[:idx]
				}

				pageWithOverlap.WriteString(" ")
				pageWithOverlap.WriteString(strings.TrimSpace(overlapText))
			} else if len(nextPage) > 20 {
				// If next page is shorter than 200 chars, add the whole thing
				pageWithOverlap.WriteString(" ")
				pageWithOverlap.WriteString(strings.TrimSpace(nextPage))
			}
		}

		result = append(result, strings.TrimSpace(pageWithOverlap.String()))
	}

	return result
}
