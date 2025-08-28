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
		BodyLimit:    100 << 20,        // 100 MB
		ReadTimeout:  15 * time.Minute, // Allow long OCR requests
		WriteTimeout: 15 * time.Minute,
		IdleTimeout:  5 * time.Minute,
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

	// Use PORT env var if present (Railway sets PORT)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}
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
