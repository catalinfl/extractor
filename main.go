package main

import (
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type ExtractResponse struct {
	Success  bool     `json:"success"`
	FileType string   `json:"file_type"`
	NumPages int      `json:"num_pages,omitempty"`
	Pages    []string `json:"pages,omitempty"`
	Text     string   `json:"text,omitempty"`
	OCRText  []string `json:"ocr_text,omitempty"`
	Error    string   `json:"error,omitempty"`
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

	// Routes
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "document-extractor"})
	})

	app.Post("/extract", handleExtractJSON)
	app.Post("/extract/text", handleExtractText)
	app.Post("/extract/ocr", handleExtractOCR)
	
	// Async OCR endpoints for scalability
	app.Post("/extract/ocr/async", handleExtractOCRAsync)
	app.Get("/extract/ocr/status/:jobId", handleGetJobStatus)

	// Use PORT env var if present (Railway sets PORT)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	
	// Initialize job queue system
	initJobQueue()
	
	app.Listen(":" + port)
}

func handleExtractText(c *fiber.Ctx) error {
	fileData, fileType, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Error: " + err.Error())
	}

	pages, err := extractTextPages(fileData, fileType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to extract text: " + err.Error())
	}

	// Combine all pages
	allText := strings.Join(pages, "\n\n--- Page Break ---\n\n")
	return c.SendString(allText)
}
