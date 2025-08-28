package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	JobID     string   `json:"job_id,omitempty"`
	Status    string   `json:"status,omitempty"` // "pending", "processing", "completed", "failed"
}

// Job Queue System for scalable OCR processing
type OCRJobRequest struct {
	ID       string
	FileData []byte
	FileType string
	Language string
	TmpDir   string
	Status   string
	Result   *OCRResponse
	Created  time.Time
	Started  *time.Time
	Finished *time.Time
	mu       sync.RWMutex
}

type OCRJobQueue struct {
	jobs     map[string]*OCRJobRequest
	pending  chan string
	workers  int
	mu       sync.RWMutex
}

var jobQueue *OCRJobQueue

// Traditional worker pool for internal page processing (limited threads)
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
var globalJobQueue *OCRJobQueue
var currentJobs int64
var maxConcurrentJobs int64 = 10 // Increased for better throughput

// generateJobID creates a unique job identifier
func generateJobID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// initJobQueue initializes the background job processing system
func initJobQueue() {
	// Use fewer workers for job queue (2-4) to allow more concurrent requests
	workers := 2
	if envWorkers := os.Getenv("QUEUE_WORKERS"); envWorkers != "" {
		if w, err := strconv.Atoi(envWorkers); err == nil && w > 0 {
			workers = w
		}
	}

	globalJobQueue = &OCRJobQueue{
		jobs:    make(map[string]*OCRJobRequest),
		pending: make(chan string, 50), // Large buffer for many requests
		workers: workers,
	}

	// Start background workers
	for i := 0; i < workers; i++ {
		go globalJobQueue.worker(i)
	}
}

// worker processes OCR jobs in background
func (q *OCRJobQueue) worker(id int) {
	for jobID := range q.pending {
		q.processJob(jobID)
	}
}

// processJob handles a single OCR job
func (q *OCRJobQueue) processJob(jobID string) {
	q.mu.RLock()
	job, exists := q.jobs[jobID]
	q.mu.RUnlock()
	
	if !exists {
		return
	}
	
	// Update job status
	job.mu.Lock()
	job.Status = "processing"
	now := time.Now()
	job.Started = &now
	job.mu.Unlock()
	
	// Process OCR (existing logic)
	result := q.performOCRJob(job)
	
	// Update job with result
	job.mu.Lock()
	job.Result = result
	job.Status = "completed"
	if result.Success == false {
		job.Status = "failed"
	}
	finished := time.Now()
	job.Finished = &finished
	job.mu.Unlock()
}

// performOCRJob executes the actual OCR processing
func (q *OCRJobQueue) performOCRJob(job *OCRJobRequest) *OCRResponse {
	startTime := time.Now()
	
	var pages []string
	var err error
	
	switch job.FileType {
	case "pdf":
		pages, err = extractOCRFromPDF(job.FileData, job.TmpDir, job.Language)
	case "png", "jpg", "jpeg", "tiff", "bmp":
		pages, err = extractOCRFromImage(job.FileData, job.TmpDir, job.Language, job.FileType)
	default:
		return &OCRResponse{
			Success:   false,
			Error:     fmt.Sprintf("Unsupported file type: %s", job.FileType),
			Timestamp: startTime.Format(time.RFC3339),
			JobID:     job.ID,
			Status:    "failed",
		}
	}
	
	if err != nil {
		return &OCRResponse{
			Success:   false,
			FileType:  job.FileType,
			Language:  job.Language,
			Error:     err.Error(),
			Timestamp: startTime.Format(time.RFC3339),
			JobID:     job.ID,
			Status:    "failed",
		}
	}
	
	// Combine pages
	extractedText := strings.Join(pages, "\n\n--- Page Break ---\n\n")
	extractedText = strings.ReplaceAll(extractedText, "\r\n", "")
	extractedText = strings.ReplaceAll(extractedText, "\n", "")
	extractedText = strings.ReplaceAll(extractedText, "\r", "")
	
	return &OCRResponse{
		Success:   true,
		FileType:  job.FileType,
		NumPages:  len(pages),
		Text:      extractedText,
		Language:  job.Language,
		Timestamp: startTime.Format(time.RFC3339),
		JobID:     job.ID,
		Status:    "completed",
	}
}

// submitJob adds a new OCR job to the queue
func (q *OCRJobQueue) submitJob(fileData []byte, fileType, language, tmpDir string) string {
	jobID := generateJobID()
	
	job := &OCRJobRequest{
		ID:       jobID,
		FileData: fileData,
		FileType: fileType,
		Language: language,
		TmpDir:   tmpDir,
		Status:   "pending",
		Created:  time.Now(),
	}
	
	q.mu.Lock()
	q.jobs[jobID] = job
	q.mu.Unlock()
	
	// Send to worker queue
	select {
	case q.pending <- jobID:
		return jobID
	default:
		// Queue full - clean up and return error
		q.mu.Lock()
		delete(q.jobs, jobID)
		q.mu.Unlock()
		return ""
	}
}

// getJobStatus retrieves job status and result
func (q *OCRJobQueue) getJobStatus(jobID string) *OCRResponse {
	q.mu.RLock()
	job, exists := q.jobs[jobID]
	q.mu.RUnlock()
	
	if !exists {
		return &OCRResponse{
			Success: false,
			Error:   "Job not found",
			JobID:   jobID,
			Status:  "not_found",
		}
	}
	
	job.mu.RLock()
	defer job.mu.RUnlock()
	
	if job.Result != nil {
		return job.Result
	}
	
	return &OCRResponse{
		Success: true,
		JobID:   jobID,
		Status:  job.Status,
	}
}

// handleExtractOCRAsync submits OCR job and returns job ID immediately
func handleExtractOCRAsync(c *fiber.Ctx) error {
	// Initialize systems
	if globalJobQueue == nil {
		initJobQueue()
	}
	if ocrPool == nil {
		initOCRPool()
	}
	
	// Get file from request
	fileData, fileType, err := getFileFromRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(OCRResponse{
			Success: false,
			Error:   err.Error(),
		})
	}
	
	// Get language parameter
	language := c.FormValue("language")
	if language == "" {
		language = "eng"
	}
	
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "ocr_*")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(OCRResponse{
			Success: false,
			Error:   "Failed to create temporary directory",
		})
	}
	
	// Submit job to queue
	jobID := globalJobQueue.submitJob(fileData, fileType, language, tmpDir)
	if jobID == "" {
		return c.Status(fiber.StatusTooManyRequests).JSON(OCRResponse{
			Success: false,
			Error:   "Queue is full - please try again later",
		})
	}
	
	return c.JSON(OCRResponse{
		Success:   true,
		JobID:     jobID,
		Status:    "pending",
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// handleGetJobStatus returns current job status and result
func handleGetJobStatus(c *fiber.Ctx) error {
	jobID := c.Params("jobId")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(OCRResponse{
			Success: false,
			Error:   "Job ID required",
		})
	}
	
	if globalJobQueue == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(OCRResponse{
			Success: false,
			Error:   "Job queue not initialized",
		})
	}
	
	result := globalJobQueue.getJobStatus(jobID)
	return c.JSON(result)
}

// CPU load monitoring
var cpuLoadHigh bool
var lastCPUCheck time.Time

// Circuit breaker state
type CircuitState int32

const (
	Closed CircuitState = iota
	Open
	HalfOpen
)

var circuitState int32 = int32(Closed)
var failures int64
var lastFailureTime time.Time

// checkSystemLoad monitors CPU and memory to prevent Railway throttling
func checkSystemLoad() bool {
	now := time.Now()
	if now.Sub(lastCPUCheck) < 2*time.Second {
		return !cpuLoadHigh
	}

	lastCPUCheck = now

	// Check concurrent jobs
	current := atomic.LoadInt64(&currentJobs)
	if current >= maxConcurrentJobs {
		cpuLoadHigh = true
		return false
	}

	// Check memory pressure (simplified)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Alloc > 6*1024*1024*1024 { // 6GB threshold for 8GB system
		cpuLoadHigh = true
		return false
	}

	cpuLoadHigh = false
	return true
}

// isCircuitOpen checks if circuit breaker should block requests
func isCircuitOpen() bool {
	state := CircuitState(atomic.LoadInt32(&circuitState))

	switch state {
	case Open:
		// Try to close circuit after 10 seconds
		if time.Since(lastFailureTime) > 10*time.Second {
			atomic.CompareAndSwapInt32(&circuitState, int32(Open), int32(HalfOpen))
			return false
		}
		return true
	case HalfOpen:
		return false
	default:
		return false
	}
}

// recordFailure tracks failures for circuit breaker
func recordFailure() {
	atomic.AddInt64(&failures, 1)
	lastFailureTime = time.Now()

	// Open circuit after 3 failures
	if atomic.LoadInt64(&failures) >= 3 {
		atomic.StoreInt32(&circuitState, int32(Open))
		atomic.StoreInt64(&failures, 0)
	}
}

// recordSuccess resets circuit breaker
func recordSuccess() {
	atomic.StoreInt32(&circuitState, int32(Closed))
	atomic.StoreInt64(&failures, 0)
}

// Initialize OCR worker pool (for internal page processing - LIMITED threads)
func initOCRPool() {
	// Use fewer workers to leave CPU for multiple concurrent requests
	workers := 2 // Conservative for scalability
	
	if w := os.Getenv("OCR_WORKERS"); w != "" {
		if v, err := strconv.Atoi(w); err == nil && v > 0 {
			workers = v
		}
	}
	
	// Don't exceed 4 workers to avoid CPU saturation
	if workers > 4 {
		workers = 4
	}

	ocrPool = &OCRWorkerPool{
		workers:  workers,
		jobQueue: make(chan OCRJob, workers*2), // Smaller buffer for limited workers
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

	// Circuit breaker check
	if isCircuitOpen() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(OCRResponse{
			Success:   false,
			Error:     "Service temporarily unavailable - system recovering",
			Timestamp: startTime.Format(time.RFC3339),
		})
	}

	// System load check - Railway throttling protection
	if !checkSystemLoad() {
		return c.Status(fiber.StatusTooManyRequests).JSON(OCRResponse{
			Success:   false,
			Error:     "System under high load - please retry in a few seconds",
			Timestamp: startTime.Format(time.RFC3339),
		})
	}

	// Track concurrent jobs
	atomic.AddInt64(&currentJobs, 1)
	defer atomic.AddInt64(&currentJobs, -1)

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
		// Record failure for circuit breaker
		recordFailure()
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

	// Success - record it for circuit breaker
	recordSuccess()

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

	// Optimized pdftoppm with parallel processing hints
	cmd := exec.Command(pdftoppmCmd, "-png", "-r", "100", "-cropbox", "-aa", "no", pdfPath, outputPrefix)
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
