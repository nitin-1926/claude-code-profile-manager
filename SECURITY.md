# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, report them privately using GitHub's
[private vulnerability reporting](https://github.com/nitin-1926/claude-code-profile-manager/security/advisories/new).
This opens a confidential advisory visible only to the maintainers.

If you cannot use that flow, email **gupta7nitin@gmail.com** with:

- A description of the vulnerability and its impact.
- Steps to reproduce (proof-of-concept if possible).
- The affected version (`ccpm --version`) and OS.

You can expect an initial acknowledgement within a few days. Please give us a
reasonable window to release a fix before any public disclosure.

## Supported versions

This project is pre-1.0 and moves quickly. Only the **latest released version**
receives security fixes. Please upgrade before reporting.

## Scope

ccpm manages Claude Code profiles and reads local transcripts on your machine.
Reports we're particularly interested in:

- Leakage of secrets/credentials across profiles or into shared/host assets.
- Path traversal or arbitrary file write via profile, asset, or MCP config.
- Command injection through profile names, asset names, or CLI arguments.
