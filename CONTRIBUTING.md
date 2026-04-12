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

## Making changes

### CLI (Go)

- The CLI lives in `cli/`. All Go code is there.
- Run tests with `cd cli && go test ./...`
- Follow existing code style. No external linters are enforced, but keep it clean.
- If you add a new command, create a new file in `cli/cmd/` following the existing pattern.

### Docs site (Next.js)

- The docs site lives in `docs/`. It uses Next.js 16, React 19, and Tailwind v4.
- Run `npm run dev` to start the dev server.
- Run `npm run lint` and `npx tsc --noEmit` before submitting.
- Components live in `docs/app/components/`.

### npm wrapper

- The npm package in `npm/` is a thin wrapper that downloads the correct Go binary on `npm install`.
- If you change `install.js`, test locally with `npm pack && npm install -g ./ngcodes-ccpm-*.tgz`.

## Pull requests

1. Create a feature branch from `main` (`git checkout -b feature/your-feature`)
2. Make your changes
3. Run tests and linting
4. Write a clear commit message describing what you changed and why
5. Open a PR against `main`

Keep PRs focused. One feature or fix per PR is ideal. If you are planning a larger change, open an issue first to discuss the approach.

## Reporting bugs

Open a [GitHub issue](https://github.com/nitin-1926/claude-code-profile-manager/issues) with:

- What you expected to happen
- What actually happened
- Steps to reproduce
- Your OS and ccpm version (`ccpm --version`)

## Code of conduct

Be respectful. This is a small project and contributions of all sizes are welcome. No contribution is too small.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
