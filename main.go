package main

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
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
		BodyLimit: 100 << 20, // 100 MB
	})

	app.Use(logger.New())
	app.Use(cors.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "document-extractor"})
	})

	app.Post("/extract", handleExtractJSON)
	app.Post("/extract/text", handleExtractText)
	app.Post("/extract/ocr", handleExtractOCR)

	app.Listen(":3000")
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
