package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// OCRResponse represents the response structure for OCR extraction
type OCRResponse struct {
	Success   bool     `json:"success"`
	FileType  string   `json:"file_type"`
	NumPages  int      `json:"num_pages,omitempty"`
	Pages     []string `json:"pages,omitempty"`
	Text      string   `json:"text,omitempty"`
	Language  string   `json:"language"`
	Error     string   `json:"error,omitempty"`
	Timestamp string   `json:"timestamp"`
}

// handleExtractOCR performs OCR extraction on uploaded files
func handleExtractOCR(c *fiber.Ctx) error {
	startTime := time.Now()

	// Check Tesseract installation
	if err := checkTesseractInstallation(); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(OCRResponse{
			Success:   false,
			Error:     fmt.Sprintf("Tesseract not available: %v", err),
			Timestamp: startTime.Format(time.RFC3339),
		})
	}

	// Get file from request
	fileData, fileType, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(OCRResponse{
			Success:   false,
			Error:     "Invalid request: " + err.Error(),
			Timestamp: startTime.Format(time.RFC3339),
		})
	}

	// Get language parameter (default: eng)
	language := strings.ToLower(c.FormValue("lang", "eng"))
	if language == "" {
		language = "eng"
	}

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "ocr-extraction-*")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(OCRResponse{
			Success:   false,
			Error:     "Failed to create temporary directory: " + err.Error(),
			Timestamp: startTime.Format(time.RFC3339),
		})
	}
	defer os.RemoveAll(tmpDir)

	var pages []string
	var extractedText string

	switch fileType {
	case "pdf":
		pages, err = extractOCRFromPDF(fileData, tmpDir, language)
	case "png", "jpg", "jpeg", "tiff", "bmp":
		pages, err = extractOCRFromImage(fileData, tmpDir, language, fileType)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(OCRResponse{
			Success:   false,
			Error:     fmt.Sprintf("Unsupported file type for OCR: %s (supported: PDF, PNG, JPG, JPEG, TIFF, BMP)", fileType),
			Timestamp: startTime.Format(time.RFC3339),
		})
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(OCRResponse{
			Success:   false,
			FileType:  fileType,
			Language:  language,
			Error:     err.Error(),
			Timestamp: startTime.Format(time.RFC3339),
		})
	}

	// Combine all pages
	extractedText = strings.Join(pages, "\n\n--- Page Break ---\n\n")
	extractedText = strings.ReplaceAll(extractedText, "\r\n", "")
	extractedText = strings.ReplaceAll(extractedText, "\n", "")
	extractedText = strings.ReplaceAll(extractedText, "\r", "")

	return c.JSON(OCRResponse{
		Success:   true,
		FileType:  fileType,
		NumPages:  len(pages),
		Text:      extractedText,
		Language:  language,
		Timestamp: startTime.Format(time.RFC3339),
	})
}

// extractOCRFromPDF converts PDF pages to images and performs OCR
func extractOCRFromPDF(pdfData []byte, tmpDir, language string) ([]string, error) {
	// Check if pdftoppm is available (allow override with PDFTOPPM_CMD)
	pdftoppmCmd := getPdftoppmCmd()
	if _, err := exec.LookPath(pdftoppmCmd); err != nil {
		return nil, fmt.Errorf("%s not found (install poppler or set PDFTOPPM_CMD): %v", pdftoppmCmd, err)
	}

	// Write PDF to temporary file
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, pdfData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write PDF file: %v", err)
	}

	// Convert PDF pages to PNG images
	outputPrefix := filepath.Join(tmpDir, "page")
	cmd := exec.Command(pdftoppmCmd, "-png", "-r", "300", pdfPath, outputPrefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %v - %s", err, string(output))
	}

	// Find generated PNG files
	pattern := outputPrefix + "-*.png"
	imageFiles, err := filepath.Glob(pattern)
	if err != nil || len(imageFiles) == 0 {
		return nil, fmt.Errorf("no pages were converted from PDF")
	}

	// Perform OCR on each image
	pages := make([]string, 0, len(imageFiles))
	for _, imagePath := range imageFiles {
		text, err := performOCR(imagePath, language)
		if err != nil {
			// Continue with empty text for failed pages
			pages = append(pages, fmt.Sprintf("[OCR Error: %v]", err))
			continue
		}
		pages = append(pages, text)
	}

	return pages, nil
}

// getPdftoppmCmd returns the pdftoppm command name or an override from PDFTOPPM_CMD env var
func getPdftoppmCmd() string {
	if cmd := strings.TrimSpace(os.Getenv("PDFTOPPM_CMD")); cmd != "" {
		return cmd
	}
	return "pdftoppm"
}

// extractOCRFromImage performs OCR directly on image files
func extractOCRFromImage(imageData []byte, tmpDir, language, fileType string) ([]string, error) {
	// Write image to temporary file
	imagePath := filepath.Join(tmpDir, "image."+fileType)
	if err := os.WriteFile(imagePath, imageData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write image file: %v", err)
	}

	// Perform OCR
	text, err := performOCR(imagePath, language)
	if err != nil {
		return nil, err
	}

	return []string{text}, nil
}

// performOCR runs Tesseract OCR on a single image file
func performOCR(imagePath, language string) (string, error) {
	// Run tesseract command: tesseract image.png stdout -l language
	cmd := exec.Command(getTesseractCmd(), imagePath, "stdout", "-l", language)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's a language error
		errorMsg := string(output)
		if strings.Contains(errorMsg, "language") {
			return "", fmt.Errorf("unsupported language '%s': %v - install language pack or use 'eng'", language, err)
		}
		return "", fmt.Errorf("tesseract failed: %v - %s", err, errorMsg)
	}

	text := strings.TrimSpace(string(output))
	return text, nil
}

// checkTesseractInstallation verifies if Tesseract is installed and accessible
func checkTesseractInstallation() error {
	cmdName := getTesseractCmd()
	cmd := exec.Command(cmdName, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Provide helpful message mentioning env override
		return fmt.Errorf("tesseract not found or failed to run")
	}

	// Verify version output contains "tesseract"
	if !strings.Contains(strings.ToLower(string(output)), "tesseract") {
		return fmt.Errorf("tesseract command available but version check failed")
	}

	return nil
}

// getTesseractCmd returns the tesseract command name or an override from TESSERACT_CMD env var
func getTesseractCmd() string {
	if cmd := strings.TrimSpace(os.Getenv("TESSERACT_CMD")); cmd != "" {
		return cmd
	}
	return "tesseract"
}
