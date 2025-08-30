package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func handleExtractJSON(c *fiber.Ctx) error {
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

	return c.JSON(ExtractResponse{
		Success:  true,
		FileType: fileType,
		Filename: filename,
		NumPages: len(pages),
		Pages:    pages,
	})
}

func getFileFromRequest(c *fiber.Ctx) ([]byte, string, string, error) {
	// Try multipart form file first
	fh, err := c.FormFile("file")
	if err == nil && fh != nil {
		data, fileType, readErr := readMultipartFile(fh)
		return data, fileType, fh.Filename, readErr
	}

	data := c.Body()
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("no file provided (use multipart field 'file' or send raw file body)")
	}

	// Try to get filename from headers
	filename := "uploaded_file" // default fallback
	if contentDisposition := c.Get("Content-Disposition"); contentDisposition != "" {
		// Parse Content-Disposition header for filename
		if idx := strings.Index(contentDisposition, "filename="); idx != -1 {
			filename = strings.Trim(contentDisposition[idx+9:], "\"")
		}
	} else if xFilename := c.Get("X-Filename"); xFilename != "" {
		// Check for custom X-Filename header
		filename = xFilename
	} else if originalName := c.Get("X-Original-Name"); originalName != "" {
		// Check for X-Original-Name header
		filename = originalName
	}

	// Detect file type from content
	fileType := detectFileType(data)
	return data, fileType, filename, nil
}

func readMultipartFile(fh *multipart.FileHeader) ([]byte, string, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, "", fmt.Errorf("cannot open uploaded file: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read uploaded file: %v", err)
	}

	// Detect file type from filename and content
	fileType := detectFileTypeFromName(fh.Filename)
	if fileType == "unknown" {
		fileType = detectFileType(data)
	}

	return data, fileType, nil
}

func detectFileTypeFromName(filename string) string {
	filename = strings.ToLower(filename)
	if strings.HasSuffix(filename, ".pdf") {
		return "pdf"
	}
	if strings.HasSuffix(filename, ".odt") {
		return "odt"
	}
	if strings.HasSuffix(filename, ".doc") {
		return "doc"
	}
	if strings.HasSuffix(filename, ".docx") {
		return "docx"
	}
	return "unknown"
}

func detectFileType(data []byte) string {
	if len(data) < 4 {
		return "unknown"
	}

	// Legacy MS Word .doc (OLE Compound File Binary Format) starts with D0 CF 11 E0
	if len(data) >= 8 && bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		return "doc"
	}

	if bytes.HasPrefix(data, []byte("%PDF")) {
		return "pdf"
	}

	// Both DOCX and ODT are ZIP files starting with "PK"
	if bytes.HasPrefix(data, []byte("PK")) {
		// Try to distinguish between DOCX and ODT by checking ZIP contents
		r := bytes.NewReader(data)
		zr, err := zip.NewReader(r, int64(len(data)))
		if err != nil {
			return "unknown"
		}

		// Check for DOCX structure (word/document.xml)
		for _, f := range zr.File {
			if f.Name == "word/document.xml" || f.Name == "[Content_Types].xml" {
				return "docx"
			}
			if f.Name == "content.xml" || f.Name == "META-INF/manifest.xml" {
				return "odt"
			}
		}

		// Default to docx for unknown ZIP files
		return "docx"
	}

	return "unknown"
}

func extractTextPages(data []byte, fileType string) ([]string, error) {
	switch fileType {
	case "pdf":
		return extractPDFText(data)
	case "odt":
		return extractODTText(data)
	case "doc":
		return extractDOCText(data)
	case "docx":
		return extractDOCXText(data)
	default:
		return nil, fmt.Errorf("unsupported file type: %s (supported: pdf, odt, doc, docx)", fileType)
	}
}
