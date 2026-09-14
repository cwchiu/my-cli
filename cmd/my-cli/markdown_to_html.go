package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type markdownToHTMLConfig struct {
	input   string
	outFile string
}

// newMarkdownToHTMLCmd returns the command that converts Markdown to HTML.
func newMarkdownToHTMLCmd() *cobra.Command {
	var raw markdownToHTMLConfig

	cmd := &cobra.Command{
		Use:   "markdown-to-html <input.md>",
		Short: "Convert a Markdown file to HTML",
		Long: `Convert a Markdown file to HTML using CommonMark and GFM extensions.

The generated HTML fragment is written to stdout by default. Use --out to
write it to a file instead. Raw HTML in the Markdown source is omitted by
default for safer output when the source is not trusted.

Examples:
  my-cli markdown-to-html README.md
  my-cli markdown-to-html README.md --out README.html`,
		Args: wrapUsage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw.input = args[0]

			return runMarkdownToHTML(cmd.OutOrStdout(), raw)
		},
	}

	cmd.Flags().StringVarP(&raw.outFile, "out", "O", "",
		"write the HTML to this file instead of stdout")
	_ = cmd.MarkFlagFilename("out", "html", "htm")

	return cmd
}

func runMarkdownToHTML(stdout io.Writer, cfg markdownToHTMLConfig) error {
	source, err := os.ReadFile(cfg.input)
	if err != nil {
		return fmt.Errorf("read Markdown file: %w", err)
	}

	var html bytes.Buffer

	markdown := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := markdown.Convert(source, &html); err != nil {
		return fmt.Errorf("convert Markdown: %w", err)
	}

	if cfg.outFile == "" {
		if _, err := stdout.Write(html.Bytes()); err != nil {
			return fmt.Errorf("write HTML: %w", err)
		}

		return nil
	}

	if err := os.WriteFile(cfg.outFile, html.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write HTML file: %w", err)
	}

	return nil
}
