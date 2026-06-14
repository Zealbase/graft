# Contributing to graft

Thank you for your interest in contributing.

## Prerequisites

- Go 1.25+
- A git repo to test against

## Build and test

```sh
# Build
go build ./...

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/providers/claudecode/...
```

## Architecture

The codebase is organized around two axes:

- **Provider packages** — `internal/providers/<name>/` — each owns `Parse`, `Serialize`, and `Schema` for one provider. To add a provider, add a new package here and register it in the gateway.
- **Sync engine** — `internal/core/` — the branch-per-file merge engine that propagates changes across providers.

The CLI (`internal/cli/`) routes through `EntryGate` (`internal/gateway/`) only; it never calls provider or engine packages directly.

## Pull request conventions

- Keep PRs focused — one logical change per PR.
- Add or update tests for any changed behavior.
- Run `go test ./...` and fix failures before submitting.
- Reference a relevant issue in the PR description if one exists.
- PR titles use the form `<type>(<scope>): <short description>` — e.g. `feat(cursor): add cursor provider`.

## Reporting issues

Open a GitHub issue with steps to reproduce and, where possible, a minimal reproduction.
