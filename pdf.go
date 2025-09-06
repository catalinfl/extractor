package main

import (
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// TO DO FIX RUSSIAN - CHINESE - ROMANIAN CHARACTERS

// generateChaptersPDF creează PDF pentru rezumatul pe capitole
func generateChaptersPDF(chapters []ChapterInfo, totalPages int, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Title
	pdf.Cell(0, 10, "Chapter Summary")
	pdf.Ln(15)

	// General Information
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pages: %d", totalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Chapters detected: %d", len(chapters)))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generated at: %s", time.Now().Format("02/01/2006 15:04")))
	pdf.Ln(15)

	for _, chapter := range chapters {
		pdf.SetFont("Arial", "B", 14)
		pdf.Cell(0, 10, fmt.Sprintf("Chapter %d: %s", chapter.Number, chapter.Title))
		pdf.Ln(8)
		pdf.SetFont("Arial", "I", 10)
		pdf.Cell(0, 6, chapter.Pages)
		pdf.Ln(8)
		pdf.SetFont("Arial", "", 11)
		pdf.MultiCell(0, 6, chapter.Summary, "", "", false)
		pdf.Ln(10)
	}

	return pdf.OutputFileAndClose(filename)
}

// generateGeneralSummaryPDF creează PDF pentru rezumatul general
func generateGeneralSummaryPDF(summary string, totalPages int, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Titlu
	title := "General Summary"

	pdf.Cell(0, 10, title)
	pdf.Ln(15)

	// Informații
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pages: %d", totalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generated at: %s", time.Now().Format("02/01/2006 15:04")))
	pdf.Ln(15)

	// Rezumat
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Summary:")
	pdf.Ln(10)

	return pdf.OutputFileAndClose(filename)
}

// generateLevelSummaryPDF creează PDF pentru rezumatul pe nivel
func generateLevelSummaryPDF(level SummaryLevel, totalPages int, filename string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	pdf.Cell(0, 10, fmt.Sprintf("Summary Level %d", level.Level))
	pdf.Ln(15)

	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Pages: %d", totalPages))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Level: %d", level.Level))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Pages per chunk: %d", level.PagesPerChunk))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Description: %s", level.Description))
	pdf.Ln(6)
	pdf.Cell(0, 8, fmt.Sprintf("Generated at: %s", time.Now().Format("02/01/2006 15:04")))
	pdf.Ln(15)

	// Summary
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, "Summary:")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 11)
	pdf.MultiCell(0, 6, level.Summary, "", "", false)

	return pdf.OutputFileAndClose(filename)
}
