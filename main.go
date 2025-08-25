package main

import (
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
	Error    string   `json:"error,omitempty"`
}

func main() {
	app := fiber.New(fiber.Config{
		BodyLimit: 100 << 20, // 100 MB
	})

	app.Use(logger.New())
	app.Use(cors.New())

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Document Text Extraction Server",
			"endpoints": []string{
				"POST /extract - Extract text from PDF/ODT (returns JSON with pages)",
				"POST /extract/text - Extract text from PDF/ODT (returns plain text)",
				"GET /health - Health check",
			},
		})
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "document-extractor"})
	})

	app.Post("/extract", handleExtractJSON)

	app.Listen(":3000")
}
