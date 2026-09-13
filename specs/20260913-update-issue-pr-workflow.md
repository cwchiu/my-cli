# Update Issue-to-PR Workflow

## Why

The password-gen work exposed a workflow gap: the implementation PR was merged only after a separate confirmation step, but the repository rules did not require every change to originate from a GitHub Issue or require the PR to close that Issue automatically.

## What

- Make GitHub Issue the required source of truth for every change.
- Require PR descriptions to use an auto-closing keyword such as `Closes #123`.
- Require PR review and CI approval before merge.
- Require post-merge worktree cleanup and local `main` fast-forward synchronization.

## How

Updated `AGENTS.md` section 10.1 with the complete Issue -> worktree -> validation -> linked PR -> reviewed merge -> cleanup -> local `main` verification flow.

## 遭遇的困難

The existing workflow described local merge commands, while the actual project process merges through GitHub pull requests.

## 如何解決

Replaced the local-merge step with explicit GitHub PR creation, review, merge confirmation, worktree removal, and `git pull --ff-only origin main` instructions.

## 最後變動了什麼

- `AGENTS.md`: enforced Issue-first development, PR auto-closing linkage, review-gated merge, worktree cleanup, and main synchronization.
- `specs/20260913-update-issue-pr-workflow.md`: recorded this workflow correction.
- Commit: `e1b8ed5`.

## 驗證結果

- Documentation change; no Go source behavior changed.
- `git diff --check`: passed.
- `git status --short --branch`: `main` is synchronized with `origin/main`; unrelated untracked files remain untouched.