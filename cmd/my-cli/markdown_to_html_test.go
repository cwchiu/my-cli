package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMarkdownToHTML(t *testing.T) {
	t.Parallel()

	input := filepath.Join(t.TempDir(), "input.md")
	require.NoError(t, os.WriteFile(input, []byte("# Title\n\n- [x] done\n\n| A | B |\n| - | - |\n| 1 | 2 |\n"), 0o600))

	var output bytes.Buffer
	require.NoError(t, runMarkdownToHTML(&output, markdownToHTMLConfig{input: input}))

	assert.Contains(t, output.String(), "<h1>Title</h1>")
	assert.Contains(t, output.String(), "<input checked=\"\" disabled=\"\" type=\"checkbox\">")
	assert.Contains(t, output.String(), "<table>")
}

func TestRunMarkdownToHTMLEscapesRawHTML(t *testing.T) {
	t.Parallel()

	input := filepath.Join(t.TempDir(), "input.md")
	require.NoError(t, os.WriteFile(input, []byte("<script>alert(1)</script>\n"), 0o600))

	var output bytes.Buffer
	require.NoError(t, runMarkdownToHTML(&output, markdownToHTMLConfig{input: input}))

	assert.NotContains(t, output.String(), "<script>")
	assert.Contains(t, output.String(), "<!-- raw HTML omitted -->")
}

func TestRunMarkdownToHTMLFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	input := filepath.Join(directory, "input.md")
	output := filepath.Join(directory, "output.html")

	require.NoError(t, os.WriteFile(input, []byte("plain text\n"), 0o600))

	var stdout bytes.Buffer
	require.NoError(t, runMarkdownToHTML(&stdout, markdownToHTMLConfig{input: input, outFile: output}))

	content, err := os.ReadFile(output) // #nosec G304 -- output is created inside t.TempDir.
	require.NoError(t, err)
	assert.Equal(t, "<p>plain text</p>\n", string(content))
	assert.Empty(t, stdout.String())
}

func TestRunMarkdownToHTMLMissingInput(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer

	err := runMarkdownToHTML(&output, markdownToHTMLConfig{input: filepath.Join(t.TempDir(), "missing.md")})

	require.Error(t, err)
	assert.ErrorContains(t, err, "read Markdown file")
}
