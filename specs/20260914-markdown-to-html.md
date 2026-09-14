# Markdown to HTML

## Why

Issue #20 requests a CLI command to convert an input Markdown file to HTML.

## What

- Add `markdown-to-html <input.md>`.
- Write an HTML fragment to stdout or `--out`/`-O`.
- Support CommonMark and GFM tables/task lists.
- Omit raw HTML by default.

## How

Use the native-Go `github.com/yuin/goldmark` parser as the smallest maintained dependency. The command reads one input file, converts it, and writes the result without invoking an external runtime. Its default renderer omits raw HTML from untrusted Markdown input.

## 遭遇的困難

The repository had no Markdown parser and the implementation must avoid requiring Pandoc at runtime.

## 如何解決

Added goldmark v1.8.6 and kept its safe default renderer configuration. PDF conversion remains a separate Issue #18 workstream.

## 最後變動了什麼

- Added the `markdown-to-html` command and focused tests.
- Registered the command and documented it in README.
- Added the goldmark dependency.

## 驗證結果

Pending final build, test, lint, and security validation before PR.