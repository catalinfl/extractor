package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/ledongthuc/pdf"
)

// extractDOCText attempts a best-effort extraction from legacy MS Word .doc (CFBF/OLE) files
// It uses a heuristic: scan for long runs of printable UTF-8/UTF-16LE text inside the binary
// and returns the concatenated results as a single page. This is not perfect but works for
// many simple documents without depending on heavy external libraries.
func extractDOCText(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty DOC file")
	}

	const minRun = 6 // minimum printable chars to accept a run
	const mergeGap = 512

	var out strings.Builder
	i := 0
	lastEnd := -1
	for i < len(data)-1 {
		// Detect likely UTF-16LE run
		if i+1 < len(data) && data[i+1] == 0x00 && data[i] >= 0x20 && data[i] <= 0x7e {
			j := i
			var units []uint16
			for j+1 < len(data) {
				u := binary.LittleEndian.Uint16(data[j : j+2])
				if u >= 0x20 && u <= 0x7e {
					units = append(units, u)
					j += 2
				} else {
					break
				}
			}
			if len(units) >= minRun {
				run := string(utf16.Decode(units))
				if lastEnd >= 0 && i-lastEnd < mergeGap {
					out.WriteByte(' ')
				} else if out.Len() > 0 {
					out.WriteString("\n\n")
				}
				out.WriteString(run)
				lastEnd = j
			}
			i = j
			continue
		}

		// ASCII run
		if data[i] >= 0x20 && data[i] <= 0x7e {
			j := i
			for j < len(data) && data[j] >= 0x20 && data[j] <= 0x7e {
				j++
			}
			if j-i >= minRun {
				run := string(data[i:j])
				if lastEnd >= 0 && i-lastEnd < mergeGap {
					out.WriteByte(' ')
				} else if out.Len() > 0 {
					out.WriteString("\n\n")
				}
				out.WriteString(run)
				lastEnd = j
			}
			i = j
			continue
		}

		i++
	}

	text := strings.TrimSpace(out.String())
	if text == "" {
		return nil, fmt.Errorf("no readable text found in DOC file")
	}

	// Split into logical pages
	return splitTextIntoPages(text), nil
}

func extractDOCXText(data []byte) ([]string, error) {
	r := bytes.NewReader(data)
	zr, err := zip.NewReader(r, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("cannot open DOCX archive: %v", err)
	}

	// Find document.xml
	var documentXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("cannot open document.xml: %v", err)
			}
			documentXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("cannot read document.xml: %v", err)
			}
			break
		}
	}

	if len(documentXML) == 0 {
		return nil, fmt.Errorf("document.xml not found in DOCX file")
	}

	text := extractTextFromXML(string(documentXML))

	// Split into logical pages based on content length or page breaks
	return splitTextIntoPages(text), nil
}

func extractPDFText(data []byte) ([]string, error) {
	r := bytes.NewReader(data)

	reader, err := pdf.NewReader(r, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("cannot open PDF: %v", err)
	}

	totalPages := reader.NumPage()
	pages := make([]string, 0, totalPages)

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			pages = append(pages, "")
			continue
		}

		rows, err := page.GetTextByRow()
		if err != nil {
			// Fallback: try to extract text manually if GetTextByRow fails
			content := page.Content()
			var sb strings.Builder
			for _, text := range content.Text {
				sb.WriteString(text.S)
				sb.WriteByte(' ')
			}
			pages = append(pages, strings.TrimSpace(sb.String()))
			continue
		}

		var pageLines []string
		for _, row := range rows {
			var rowText strings.Builder
			for i, word := range row.Content {
				if i > 0 {
					rowText.WriteByte(' ')
				}
				rowText.WriteString(word.S)
			}
			if rowText.Len() > 0 {
				pageLines = append(pageLines, strings.TrimSpace(rowText.String()))
			}
		}

		pageText := strings.Join(pageLines, "\n")
		pageText = strings.TrimSpace(pageText)
		pages = append(pages, pageText)
	}

	return pages, nil
}

// ODT Extractor => Split into pages
func extractODTText(data []byte) ([]string, error) {
	// ODT is a ZIP archive with content.xml containing the text
	r := bytes.NewReader(data)
	zr, err := zip.NewReader(r, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("cannot open ODT archive: %v", err)
	}

	// Find content.xml
	var contentXML []byte
	for _, f := range zr.File {
		if f.Name == "content.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("cannot open content.xml: %v", err)
			}
			contentXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("cannot read content.xml: %v", err)
			}
			break
		}
	}

	if len(contentXML) == 0 {
		return nil, fmt.Errorf("content.xml not found in ODT file")
	}

	// Simple XML text extraction (removes tags)
	text := extractTextFromXML(string(contentXML))

	// Split into logical pages
	return splitTextIntoPages(text), nil
}

func extractTextFromXML(xmlContent string) string {
	var result strings.Builder
	inTag := false

	for _, r := range xmlContent {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				result.WriteRune(r)
			}
		}
	}

	// Clean up whitespace
	text := result.String()
	text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	text = strings.ReplaceAll(text, "\t", " ")
	lines := strings.Split(text, "\n")

	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

// splitTextIntoPages splits a long text into logical pages
// Based on content length and natural breaks like double newlines
func splitTextIntoPages(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{""}
	}

	// First, try to split by explicit page breaks or form feeds
	if strings.Contains(text, "\f") {
		pages := strings.Split(text, "\f")
		var result []string
		for _, page := range pages {
			page = strings.TrimSpace(page)
			if page != "" {
				result = append(result, page)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	// Split by multiple newlines (paragraph breaks) as page separators
	paragraphs := strings.Split(text, "\n\n")

	// If we have many short paragraphs, group them into pages
	const maxCharsPerPage = 2000

	var pages []string
	var currentPage strings.Builder

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		// If adding this paragraph would make the page too long, start a new page
		if currentPage.Len() > 0 && currentPage.Len()+len(paragraph) > maxCharsPerPage {
			pages = append(pages, strings.TrimSpace(currentPage.String()))
			currentPage.Reset()
		}

		if currentPage.Len() > 0 {
			currentPage.WriteString("\n\n")
		}
		currentPage.WriteString(paragraph)
	}

	// Add the last page if it has content
	if currentPage.Len() > 0 {
		pages = append(pages, strings.TrimSpace(currentPage.String()))
	}

	// If we ended up with no pages or very few, try a different approach
	if len(pages) == 0 {
		return []string{text}
	}

	// If we have only one page but it's very long, split it by sentences
	if len(pages) == 1 && len(pages[0]) > maxCharsPerPage*2 {
		return splitByLength(pages[0], maxCharsPerPage)
	}

	return pages
}

// splitByLength splits text into chunks of approximately maxLength characters
// trying to break at sentence or paragraph boundaries
func splitByLength(text string, maxLength int) []string {
	if len(text) <= maxLength {
		return []string{text}
	}

	var pages []string
	remaining := text

	for len(remaining) > maxLength {
		// Find a good break point near maxLength
		breakPoint := maxLength

		// Look for paragraph break first
		if idx := strings.LastIndex(remaining[:breakPoint], "\n\n"); idx > maxLength/2 {
			breakPoint = idx
		} else if idx := strings.LastIndex(remaining[:breakPoint], ". "); idx > maxLength/2 {
			// Look for sentence break
			breakPoint = idx + 1
		} else if idx := strings.LastIndex(remaining[:breakPoint], " "); idx > maxLength/2 {
			// Look for word break
			breakPoint = idx
		}

		pages = append(pages, strings.TrimSpace(remaining[:breakPoint]))
		remaining = strings.TrimSpace(remaining[breakPoint:])
	}

	// Add the remaining text as the last page
	if len(remaining) > 0 {
		pages = append(pages, remaining)
	}

	return pages
}
