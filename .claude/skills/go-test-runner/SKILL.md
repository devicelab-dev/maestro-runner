---
name: go-test-runner
description: >
  Runs Go tests, race checks, coverage checks, benchmarks, and quality checks
  for the maestro-runner repository. Use this skill whenever the user asks to
  run or debug Go tests, `go test`, `make test`, `make check`, CI-style Go
  verification at repo root, or asks why a Go test is failing, how to fix a
  test failure, how to run go vet or staticcheck, or how to check coverage.
  DO NOT use for Python client tests or TypeScript client tests; use the
  dedicated client skills for those.
allowed-tools: "Bash(go:*) Bash(make:*) Bash(grep:*) Bash(awk:*) Bash(sed:*) Bash(cat:*) Bash(ls:*) Bash(find:*) Bash(head:*) Bash(tail:*) Bash(tee:*)"
metadata:
  author: maestro-runner
  version: 1.0.0
  category: testing
  tags: [go, gotest, race, coverage, benchmark, ci]
---

# Go Test Runner

Runs Go test and quality workflows for this repo from the root directory.

## Do NOT use this skill for

- Python client tests in `client/python/` -> use `python-test-runner`
- TypeScript client tests in `client/typescript/` -> use `typescript-test-runner`
- Non-test tasks unrelated to Go validation

## Prerequisites

- Go installed (`go version`)
- Commands run from repository root
- For race/coverage and full checks: allow longer runtime

## Quick Start

```sh
# From repo root
make test
```

Equivalent direct command:

```sh
go test -v ./...
```

## Core Workflows

### 1) Run all Go tests

```sh
make test
# or
go test -v ./...
```

### 2) Run race detector

```sh
make test-race
# or
go test -v -race ./...
```

### 3) Generate coverage report

```sh
make test-coverage
# Produces coverage.out and coverage.html
```

### 4) Enforce coverage threshold (CI-style)

```sh
make test-coverage-check
# Fails if total coverage is below 80%
```

### 5) Run benchmarks

```sh
make bench
# or
go test -v -bench=. -benchmem ./...
```

### 6) Run fuzz tests

```sh
make test-fuzz
# or
go test -v -fuzz=. -fuzztime=30s ./...
```

## Full Validation

### Local full check

```sh
make check
```

Runs formatting, static analysis/security checks, and race tests.

### CI-equivalent check

```sh
make ci
```

Runs full quality checks plus coverage threshold enforcement.

## Package-Specific / Focused Runs

```sh
# Single package
go test -v ./pkg/cli

# Single test by name pattern in a package
go test -v ./pkg/cli -run TestUnified

# Re-run failed tests quickly (Go test cache aware)
go test -v ./...
```

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `go: command not found` | Install Go and ensure it is on PATH |
| Very slow first run | Run `go mod download` (or `make deps`) to prefetch modules |
| Race test timeout/flakes | Re-run package-level tests first to isolate: `go test -v -race ./pkg/...` |
| Coverage check fails | Inspect `coverage.out` with `go tool cover -func=coverage.out` and add tests |
| Linter tool missing in `make check` | Install dev tools via `make dev-setup` |

## Useful Supporting Targets

```sh
make deps       # go mod download + go mod tidy
make dev-setup  # installs static analysis tools used by make check/ci
```
