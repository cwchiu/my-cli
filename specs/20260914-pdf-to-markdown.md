# PDF to Markdown

## Why

Issue #18 requests a command that converts PDF content to Markdown while keeping the output as close to the source as practical.

## What

- Add `pdf-to-markdown <input.pdf>`.
- Extract text in page and row order.
- Write Markdown to stdout or `--out`/`-O`.
- Document the limits: OCR, images, tables, and exact multi-column layout are not reconstructed.

## How

Use the native-Go `github.com/ledongthuc/pdf` package for PDF text and row extraction. The command owns the Markdown rendering and does not invoke an external converter.

## 遭遇的困難

PDF stores positioned drawing instructions rather than Markdown semantics. A general converter cannot reliably infer headings, tables, images, or multi-column reading order from arbitrary PDFs.

## 如何解決

Preserve the reliable information available from the parser: page order, row order, horizontal text order, and line breaks. Keep the scope explicit instead of producing misleading Markdown structure.

## 最後變動了什麼

- Added the `pdf-to-markdown` command and focused error-path tests.
- Registered the command and documented its behavior and limits in README.
- Added the minimal native-Go PDF dependency.

## 驗證結果

Pending final build, test, lint, and security validation before PR.