# ccpm

**Claude Code Profile Manager** — run multiple Claude Code accounts simultaneously with full isolation.

[![CI](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/nitin-1926/ccpm)](https://goreportcard.com/report/github.com/nitin-1926/ccpm)

---

## Privacy & Security

**ccpm is 100% local.** Your data never leaves your machine.

- No telemetry, analytics, or tracking of any kind
- No network calls — ccpm never contacts any server
- No data collection — we don't know you exist
- Credentials stored in your **OS keychain** (macOS Keychain, Linux Secret Service, Windows Credential Manager)
- Vault backups use **AES-256-GCM encryption** with a master key in your keychain
- Config files live in `~/.ccpm/` on your filesystem — nowhere else
- Fully open source — [audit the code yourself](https://github.com/nitin-1926/claude-code-profile-manager)

---

## The Problem

You have multiple Claude Code accounts — personal, work, side projects — and switching between them is painful:

- Manual logout/login cycles break your flow
- No way to run two accounts in parallel
- Settings, MCP servers, and memory bleed across accounts
- VS Code extension hardcoded to one account

**ccpm** fixes this. One command per terminal, full isolation, zero conflicts.

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh

# Create profiles
ccpm add personal     # Choose OAuth or API key — first profile auto-sets as default
ccpm add work         # Each profile is fully isolated

# Run in parallel — one per terminal
ccpm run personal     # Terminal 1
ccpm run work         # Terminal 2
```

That's it. Each terminal runs a completely isolated Claude Code instance with its own credentials, settings, and memory.

## How It Works

ccpm is built on one official Claude Code mechanism:

> Setting `CLAUDE_CONFIG_DIR` to a directory path causes Claude Code to read all credentials, settings, MCP config, memory, and project data from that directory instead of `~/.claude`.

ccpm manages isolated directories under `~/.ccpm/profiles/<name>/` and launches Claude with the correct environment. Each profile gets its own keychain entry on macOS, its own credentials file on Linux/Windows, and its own config.

```
~/.ccpm/
├── config.json              # Global ccpm config
├── profiles/
│   ├── personal/            # CLAUDE_CONFIG_DIR for "personal"
│   │   ├── .claude.json     # Account data, settings
│   │   ├── settings.json    # MCP servers, preferences
│   │   └── sessions/        # Chat history
│   └── work/                # Fully isolated from "personal"
└── vault/
    ├── personal.enc         # Encrypted credential backup
    └── work.enc
```

## Installation

### curl (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh
```

### Go

```bash
go install github.com/nitin-1926/ccpm@latest
```

### npm

```bash
npm install -g ccpm
```

### From source

```bash
git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/cli
make build          # Binary at ./bin/ccpm
go install .        # Install to $GOPATH/bin
```

## Shell Integration (Optional)

For `ccpm use` (setting a profile for your whole shell session), add this to your `~/.zshrc` or `~/.bashrc`:

```bash
eval "$(ccpm shell-init)"
```

Then reload: `source ~/.zshrc`

> **Note:** `ccpm run` works without any shell setup. Shell integration is only needed for `ccpm use`.

## Commands

### Profile Management

```bash
ccpm add <name>         # Create profile (OAuth or API key)
ccpm list               # List all profiles with auth status
ccpm remove <name>      # Delete a profile
ccpm status             # Full system overview
```

### Running Claude

```bash
ccpm run <name>         # Launch Claude with this profile (recommended)
ccpm use <name>         # Set profile for current shell session
```

### Authentication

```bash
ccpm auth status        # Check auth health across all profiles
ccpm auth refresh <n>   # Re-authenticate a profile
ccpm auth backup <n>    # Encrypted credential backup to vault
ccpm auth restore <n>   # Restore credentials from vault backup
```

### IDE / VS Code

```bash
ccpm set-default <name>   # Set profile for VS Code extension
ccpm unset-default        # Clear default
```

### Uninstall

```bash
ccpm uninstall          # Remove all ccpm data, profiles, and keychain entries
```

## Auth Methods

ccpm supports both authentication methods:

### OAuth (Browser Login)

```bash
$ ccpm add personal
Choose authentication method:
  1) OAuth (browser login via claude /login)
  2) API Key
Enter choice [1/2]: 1

Claude Code will launch. Run /login to authenticate.
# ...browser auth flow...

✓ Profile "personal" authenticated via OAuth
✓ Set as default profile (first profile)
```

### API Key

```bash
$ ccpm add work
Choose authentication method:
  1) OAuth (browser login via claude /login)
  2) API Key
Enter choice [1/2]: 2

Enter your Anthropic API key: ****
✓ Profile "work" authenticated via API key
```

API keys are stored in your OS keychain (macOS Keychain / Linux Secret Service / Windows Credential Manager) — never in plaintext files.

## Parallel Sessions

Run different accounts simultaneously in different terminals:

```
┌─────────────────────┐  ┌─────────────────────┐
│ Terminal 1           │  │ Terminal 2           │
│                      │  │                      │
│ $ ccpm run personal  │  │ $ ccpm run work      │
│                      │  │                      │
│ Claude Code          │  │ Claude Code          │
│ (personal@gmail.com) │  │ (work@company.com)   │
│                      │  │                      │
│ Own settings         │  │ Own settings         │
│ Own MCP servers      │  │ Own MCP servers      │
│ Own memory           │  │ Own memory           │
└─────────────────────┘  └─────────────────────┘
```

## Platform Support

| Feature | macOS | Linux | Windows |
|---------|-------|-------|---------|
| OAuth auth | Keychain (per-profile isolation) | `.credentials.json` in profile dir | `.credentials.json` in profile dir |
| API key auth | Keychain | Secret Service (D-Bus) | Credential Manager |
| Parallel sessions | Yes | Yes | Yes |
| Vault backup | Yes | Yes | Yes |
| Shell hook | zsh, bash, fish | zsh, bash, fish | PowerShell |

## Known Limitations

We believe in being honest about constraints:

| # | Limitation | Severity | Workaround |
|---|---|---|---|
| L1 | **VS Code extension ignores `CLAUDE_CONFIG_DIR`** — reads from `~/.claude` always | High | Use `ccpm set-default <profile>` to set the VS Code account |
| L2 | **`CLAUDE_CONFIG_DIR` path with `~/`** — Claude has a bug resolving `~/` paths on Linux | Medium | ccpm always uses absolute paths (handled automatically) |
| L3 | **Same-account parallel sessions** — running one profile in two terminals hits Anthropic's refresh token race | Medium | Use different profiles in different terminals |
| L4 | **Headless Linux** — `go-keyring` requires D-Bus + secret service | Low | API key profiles need a running secret service |

## Project Structure

```
claude-code-profile-manager/
├── cli/              # Go source code (ccpm binary)
│   ├── cmd/          # CLI commands
│   ├── internal/     # Core packages (config, profile, vault, etc.)
│   └── Makefile
├── docs/             # Documentation website (Next.js, deploy to Vercel)
├── npm/              # npm wrapper package
├── scripts/          # Install script
└── .github/          # CI/CD workflows
```

## Roadmap

- [x] Profile management (add, list, remove, run)
- [x] OAuth + API key authentication
- [x] Encrypted vault backup/restore
- [x] Shell integration (use, shell-init)
- [x] VS Code default profile (set-default)
- [x] Uninstall command
- [ ] Per-profile MCP server management
- [ ] Shared vs isolated config (skills, commands, CLAUDE.md)
- [ ] Token optimization presets
- [ ] Diagnostics (`ccpm doctor`)
- [ ] GUI companion app
- [ ] Multi-tool support (Codex CLI, Cursor CLI, Gemini CLI)

## Contributing

Contributions welcome! Please open an issue first to discuss what you'd like to change.

```bash
# Development setup
git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/cli
go mod tidy
make build          # Build binary to ./bin/ccpm
go install .        # Install to $GOPATH/bin for testing
make test           # Run tests
```

### Testing the npm package locally

```bash
cd npm
npm pack                          # Creates ccpm-0.1.0.tgz
npm install -g ccpm-0.1.0.tgz    # Install from local tarball
ccpm --version                    # Verify
npm uninstall -g ccpm             # Clean up
```

## License

[MIT](LICENSE)
