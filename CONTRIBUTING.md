# Contributing to ccpm

Thanks for your interest in contributing to ccpm. This document covers the basics you need to get started.

## Getting started

1. Fork the repo and clone your fork
2. Install Go 1.22+ and Node.js 20+
3. Build the CLI:

```bash
cd cli
go build -o ccpm .
./ccpm --version
```

4. Run the docs site locally:

```bash
cd docs
npm install
npm run dev
```

## Project structure

```
cli/           Go CLI source code
  cmd/         Cobra command definitions
  internal/    Internal packages (config, credentials, vault, etc.)
docs/          Next.js documentation website
npm/           npm wrapper package (downloads the Go binary on install)
scripts/       Installation scripts
```

## Before making changes

Two documents are load-bearing for non-trivial work:

- **[AGENTS.md](AGENTS.md)** — architectural briefing: core mental model, directory layout, merge precedence, invariants that must hold. Read this before changing anything that crosses packages or touches the merge stack.

## Making changes

### CLI (Go)
