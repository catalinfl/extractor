package main

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
)

type ExtractResponse struct {
	Success        bool     `json:"success"`
	FileType       string   `json:"file_type"`
	Filename       string   `json:"filename,omitempty"`
	NumPages       int      `json:"num_pages,omitempty"`
	Pages          []string `json:"pages,omitempty"`
	Text           string   `json:"text,omitempty"`
	Error          string   `json:"error,omitempty"`
	StoredInQdrant bool     `json:"stored_in_qdrant,omitempty"`
}

type ParagraphSearchResponse struct {
	Success    bool           `json:"success"`
	Results    []SearchResult `json:"results,omitempty"`
	Query      string         `json:"query"`
	Username   string         `json:"username"`
	TotalFound int            `json:"total_found"`
	Error      string         `json:"error,omitempty"`
}

func main() {

	app := fiber.New(fiber.Config{
		BodyLimit:         15 << 20,         // 15 MB
		ReadTimeout:       10 * time.Minute, // Railway timeout protection
		WriteTimeout:      10 * time.Minute, // Railway timeout protection
		IdleTimeout:       2 * time.Minute,  // Faster connection cleanup
		DisableKeepalive:  false,            // Keep connections alive
		ReadBufferSize:    8192,             // Optimized buffer
		WriteBufferSize:   8192,
		ProxyHeader:       "X-Forwarded-For",
		ServerHeader:      "PDF-Extractor-Railway",
		ReduceMemoryUsage: true, // Railway memory optimization
	})

	// Middleware
	app.Use(recover.New()) // Prevent panics from killing connections
	app.Use(logger.New())
	app.Use(cors.New())

	godotenv.Load()

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "document-extractor"})
	})

	// PDF ROUTES
	// Extract from PDF, returns JSON
	app.Post("/extract", handleExtractJSON)

	// QDRANT ROUTES
	// Extract from PDF -> Put pages in Qdrant
	app.Post("/extract/store", handleExtractAndStore)
	// Search by username and query
	app.Post("/search", handleSearchPages)
	// Delete all user data from Qdrant
	app.Delete("/leave/:username", handleOnLeave)

	// OPENROUTER ROUTES
	// Answer questions based on vector search results
	app.Post("/answer", handleAnswerQuestion)
	// Extract keywords from query for better search
	app.Post("/extract-keywords", handleExtractKeywords)
	// Smart search: Extract keywords + Search + AI answer in one request
	app.Post("/smart-search", handleSmartSearch)

	// SUMMARY ROUTES - 3 TIPURI SEPARATE
	// 1. Rezumat pe capitole (primește tot PDF-ul)
	app.Post("/summary/chapters", handleChapterSummary)
	app.Post("/summary/chapters/download", handleDownloadChapterSummaryPDF)

	// 2. Rezumat general (o linie sau o pagină)
	app.Post("/summary/general", handleGeneralSummary)
	app.Post("/summary/general/download", handleDownloadGeneralSummaryPDF)

	// 3. Rezumat pe nivele (user alege nivelul 1-10)
	app.Post("/summary/level", handleLevelSummary)
	app.Post("/summary/level/download", handleDownloadLevelSummaryPDF)

	// Use PORT env var if present (Railway sets PORT)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	app.Listen(":" + port)
}
