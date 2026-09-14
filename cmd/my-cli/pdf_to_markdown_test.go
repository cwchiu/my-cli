package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPDFToMarkdownMissingInput(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer

	err := runPDFToMarkdown(&output, pdfToMarkdownConfig{input: filepath.Join(t.TempDir(), "missing.pdf")})

	require.ErrorContains(t, err, "open PDF file")
}

func TestRunPDFToMarkdownWritesOutputFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	input := filepath.Join(directory, "invalid.pdf")
	output := filepath.Join(directory, "output.md")

	require.NoError(t, os.WriteFile(input, []byte("not a PDF"), 0o600))

	var stdout bytes.Buffer

	err := runPDFToMarkdown(&stdout, pdfToMarkdownConfig{input: input, outFile: output})

	require.ErrorContains(t, err, "open PDF file")
	assert.Empty(t, stdout.String())
}
