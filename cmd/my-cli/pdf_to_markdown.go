package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/spf13/cobra"
)

type pdfToMarkdownConfig struct {
	input   string
	outFile string
}

// newPDFToMarkdownCmd returns the command that extracts PDF text as Markdown.
func newPDFToMarkdownCmd() *cobra.Command {
	return newFileConversionCmd(
		"pdf-to-markdown <input.pdf>",
		"Extract PDF text as Markdown",
		`Extract text from a PDF and write it as Markdown.

The converter preserves page and row order and writes the result to stdout by
default. Use --out to write it to a file instead. Text layout is preserved as
paragraph lines where possible; scanned PDFs, images, tables, and exact
multi-column layout require OCR or a dedicated layout workflow.

Examples:
  my-cli pdf-to-markdown report.pdf
  my-cli pdf-to-markdown report.pdf --out report.md`,
		"write the Markdown to this file instead of stdout",
		[]string{"md", "markdown"},
		func(stdout io.Writer, input, outFile string) error {
			return runPDFToMarkdown(stdout, pdfToMarkdownConfig{input: input, outFile: outFile})
		},
	)
}

func runPDFToMarkdown(stdout io.Writer, cfg pdfToMarkdownConfig) error {
	pdfFile, reader, err := pdf.Open(cfg.input)
	if err != nil {
		return fmt.Errorf("open PDF file: %w", err)
	}
	defer func() { _ = pdfFile.Close() }()

	markdown, err := extractPDFMarkdown(reader)
	if err != nil {
		return fmt.Errorf("extract PDF text: %w", err)
	}

	if cfg.outFile == "" {
		if _, err := io.WriteString(stdout, markdown); err != nil {
			return fmt.Errorf("write Markdown: %w", err)
		}

		return nil
	}

	if err := os.WriteFile(cfg.outFile, []byte(markdown), 0o600); err != nil {
		return fmt.Errorf("write Markdown file: %w", err)
	}

	return nil
}

func extractPDFMarkdown(reader *pdf.Reader) (string, error) {
	var output strings.Builder

	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		rows, err := reader.Page(pageNumber).GetTextByRow()
		if err != nil {
			return "", fmt.Errorf("read page %d: %w", pageNumber, err)
		}

		if pageNumber > 1 && output.Len() > 0 {
			output.WriteString("\n")
		}

		for _, row := range rows {
			line := strings.TrimSpace(textRow(row.Content))
			if line == "" {
				continue
			}

			output.WriteString(line)
			output.WriteString("\n")
		}
	}

	return output.String(), nil
}

func textRow(texts pdf.TextHorizontal) string {
	var line strings.Builder
	for _, text := range texts {
		line.WriteString(text.S)
	}

	return line.String()
}
