package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// Worker pool for OCR processing
type OCRWorkerPool struct {
	workers  int
	jobQueue chan OCRJob
}

type OCRJob struct {
	imagePath string
	language  string
	result    chan OCRResult
}

type OCRResult struct {
	text string
	err  error
}

var ocrPool *OCRWorkerPool

// Initialize OCR worker pool
func initOCRPool() {
	workers := runtime.NumCPU()
	if w := os.Getenv("OCR_WORKERS"); w != "" {
		if v, err := strconv.Atoi(w); err == nil && v > 0 {
			workers = v
		}
	}

	ocrPool = &OCRWorkerPool{
		workers:  workers,
		jobQueue: make(chan OCRJob, workers*6), // Large buffer for high throughput 8 vCPU
	}

	// Start worker goroutines
	for i := 0; i < workers; i++ {
		go ocrPool.worker()
	}
}

func (p *OCRWorkerPool) worker() {
	for job := range p.jobQueue {
		text, err := performOCRDirect(job.imagePath, job.language)
		job.result <- OCRResult{text: text, err: err}
	}
}

func (p *OCRWorkerPool) processOCR(imagePath, language string) (string, error) {
	result := make(chan OCRResult, 1)
	job := OCRJob{
		imagePath: imagePath,
		language:  language,
		result:    result,
	}

	select {
	case p.jobQueue <- job:
		res := <-result
		return res.text, res.err
	default:
		// Fallback if pool is full
		return performOCRDirect(imagePath, language)
	}
}

// handleExtractOCR performs OCR extraction on uploaded files
func handleExtractOCR(c *fiber.Ctx) error {
	startTime := time.Now()

	// Initialize pool if not done
	if ocrPool == nil {
		initOCRPool()
	}

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

// extractOCRFromPDF converts PDF pages to images and performs OCR with parallel processing
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

	// Convert PDF pages to PNG images (DPI configurable via env)
	outputPrefix := filepath.Join(tmpDir, "page")

	// Use PDF_DPI env var or default to 75 for Railway performance
	dpi := "100"
	if envDPI := os.Getenv("PDF_DPI"); envDPI != "" {
		dpi = envDPI
	}

	// Optimized pdftoppm with parallel processing hints
	cmd := exec.Command(pdftoppmCmd, "-png", "-r", dpi, "-cropbox", "-aa", "no", pdfPath, outputPrefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %v - %s", err, string(output))
	}

	// Find generated PNG files
	pattern := outputPrefix + "-*.png"
	imageFiles, err := filepath.Glob(pattern)
	if err != nil || len(imageFiles) == 0 {
		return nil, fmt.Errorf("no pages were converted from PDF")
	}

	// Ultra-fast parallel OCR processing with optimized batching
	type pageResult struct {
		index int
		text  string
		err   error
	}

	resultChan := make(chan pageResult, len(imageFiles))

	// Calculate optimal batch size based on worker count and page count
	batchSize := len(imageFiles) / ocrPool.workers
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 4 {
		batchSize = 4 // Max 4 pages per goroutine for memory efficiency
	}

	var wg sync.WaitGroup

	// Process pages in optimized batches
	for i := 0; i < len(imageFiles); i += batchSize {
		end := i + batchSize
		if end > len(imageFiles) {
			end = len(imageFiles)
		}

		wg.Add(1)
		go func(start, stop int) {
			defer wg.Done()
			for idx := start; idx < stop; idx++ {
				text, err := ocrPool.processOCR(imageFiles[idx], language)
				if err != nil {
					text = fmt.Sprintf("[OCR Error: %v]", err)
				}
				resultChan <- pageResult{index: idx, text: text, err: err}
			}
		}(i, end)
	}

	wg.Wait()
	close(resultChan)

	// Collect results in order
	pages := make([]string, len(imageFiles))
	for result := range resultChan {
		pages[result.index] = result.text
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

	// Perform OCR using worker pool
	text, err := ocrPool.processOCR(imagePath, language)
	if err != nil {
		return nil, err
	}

	return []string{text}, nil
}

// performOCR runs Tesseract OCR on a single image file (legacy function, keep for compatibility)
// func performOCR(imagePath, language string) (string, error) {
// 	return performOCRDirect(imagePath, language)
// }

// performOCRDirect runs Tesseract OCR directly (used by worker pool)
func performOCRDirect(imagePath, language string) (string, error) {
	// Tesseract optimized for Railway 8 vCPU maximum performance:
	// --psm 3 = fully automatic page segmentation (reliable and fast)
	// --oem 1 = LSTM only (faster than combined)
	// Disable dictionaries for speed but keep accuracy
	cmd := exec.Command(getTesseractCmd(), imagePath, "stdout", "-l", language,
		"--psm", "3", "--oem", "1",
		"-c", "tessedit_do_invert=0",
		"-c", "load_system_dawg=0",
		"-c", "load_freq_dawg=0",
		"-c", "load_unambig_dawg=0",
		"-c", "textord_heavy_nr=1")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to even simpler mode
		cmd = exec.Command(getTesseractCmd(), imagePath, "stdout", "-l", language, "--psm", "6", "--oem", "1")
		output, err = cmd.CombinedOutput()
		if err != nil {
			// Final fallback - basic mode
			cmd = exec.Command(getTesseractCmd(), imagePath, "stdout", "-l", language)
			output, err = cmd.CombinedOutput()
			if err != nil {
				errorMsg := string(output)
				if strings.Contains(errorMsg, "language") {
					return "", fmt.Errorf("unsupported language '%s': %v - install language pack or use 'eng'", language, err)
				}
				return "", fmt.Errorf("tesseract failed: %v - %s", err, errorMsg)
			}
		}
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
