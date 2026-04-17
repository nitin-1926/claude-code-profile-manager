# ccpm

**Run multiple Claude Code accounts in parallel. Fully isolated. One command.**

[![CI](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@ngcodes/ccpm)](https://www.npmjs.com/package/@ngcodes/ccpm)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

ccpm (Claude Code Profile Manager) lets you create isolated profiles for Claude Code, each with its own credentials, settings, MCP servers, and memory. Open two terminals, run two different accounts at the same time.

## Why

Claude Code reads config from a single directory (`~/.claude`). If you have a personal account and a work account, you cannot use both at the same time. Switching means logging out and back in, or manually swapping config files.

ccpm fixes this. Each profile gets its own config directory. When you run `ccpm run <profile>`, it sets `CLAUDE_CONFIG_DIR` to the right directory and launches Claude Code. Two terminals, two profiles, zero conflicts.

## Install

```bash
# npm
npm i -g @ngcodes/ccpm

# curl (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh

# go
go install github.com/nitin-1926/ccpm@latest
```

## Quick start

```bash
# Create profiles
ccpm add personal    # authenticate via OAuth or API key
ccpm add work        # same, with a different account

# Run them in parallel
ccpm run personal    # in terminal 1
ccpm run work        # in terminal 2

# Check status
ccpm list
```

Output:

```
NAME       AUTH      STATUS
personal   oauth     ✓ nitin@gmail.com
work       api_key   ✓ sk-ant-...7f2k   ★
```

## Key features

- **Parallel sessions**: run different Claude Code accounts in different terminals simultaneously
- **Full isolation**: each profile has its own credentials, settings, MCP servers, projects, and memory
- **OAuth + API key**: supports both authentication methods per profile
- **Skills, MCP, and settings management**: install globally or per-profile with `--global` / `--profile`
- **Encrypted vault**: AES-256-GCM encrypted credential backups with master key in your OS keychain
- **IDE support**: set the default profile for VS Code with `ccpm set-default`
- **Shell integration**: `ccpm use` sets the profile for your entire shell session
- **Cross-platform**: macOS Keychain, Linux Secret Service, Windows Credential Manager

## Commands

| Command                                   | Description                                                               |
| ----------------------------------------- | ------------------------------------------------------------------------- |
| `ccpm add <name>`                         | Create a new profile (OAuth or API key) with an interactive import wizard |
| `ccpm run <name>`                         | Launch Claude Code with a profile                                         |
| `ccpm use <name>`                         | Set profile for the current shell session                                 |
| `ccpm list`                               | List all profiles and their status                                        |
| `ccpm status`                             | Show system overview                                                      |
| `ccpm doctor`                             | Diagnose env, auth health, root-vs-profile drift, symlink integrity       |
| `ccpm set-default <name>`                 | Set the default profile for IDEs                                          |
| `ccpm remove <name>`                      | Delete a profile                                                          |
| `ccpm sync`                               | Sync global installs into profiles                                        |
| `ccpm import default`                     | Import assets from `~/.claude` (symlinked via shared store)               |
| `ccpm import from-profile`                | Clone assets from one ccpm profile to another                             |
| `ccpm skill add/remove/list/link`         | Manage Claude Code skills                                                 |
| `ccpm mcp add/remove/list/import`         | Manage MCP servers                                                        |
| `ccpm settings set/get/apply/show`        | Manage Claude Code settings                                               |
| `ccpm auth status/refresh/backup/restore` | Manage authentication                                                     |

## How it works

ccpm uses one official mechanism: the `CLAUDE_CONFIG_DIR` environment variable.

1. `ccpm add` creates `~/.ccpm/profiles/<name>/` with its own config and credentials
2. `ccpm run` merges shared settings/MCP fragments, sets `CLAUDE_CONFIG_DIR`, and execs `claude`
3. Each terminal gets a completely isolated Claude Code instance

Skills, MCP servers, and settings can be installed globally (`--global`) to apply across all profiles, or per-profile (`--profile <name>`). Global skills are symlinked into each profile from `~/.ccpm/share/`; settings and MCP definitions are stored as JSON fragments and merged into each profile's `settings.json` at launch time.

No daemons. No patches. No magic.

## Privacy and security

ccpm is 100% local. It never makes network requests, never collects data, and never phones home.

- API keys are stored in your OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager)
- Vault backups use AES-256-GCM encryption with a master key in your OS keychain
- All data lives in `~/.ccpm/` on your machine
- No telemetry, analytics, or tracking

## Platform support

| Feature            | macOS                                    | Linux             | Windows                           |
| ------------------ | ---------------------------------------- | ----------------- | --------------------------------- |
| OAuth per-profile  | Keychain entry namespaced by profile dir | .credentials.json | .credentials.json                 |
| API key storage    | Keychain                                 | Secret Service    | Credential Manager                |
| Parallel sessions  | Yes                                      | Yes               | Yes                               |
| Shared skill dedup | Symlinks                                 | Symlinks          | Symlinks (Developer Mode) or copy |
| Shell hook         | zsh, bash, fish                          | zsh, bash, fish   | PowerShell                        |

> **Requires Claude Code `v2.1.56` or newer for macOS OAuth isolation.** Earlier versions share a single keychain entry across all profiles. `ccpm doctor` warns on older versions.

## MCP authentication model

MCP servers authenticate in one of three ways, and ccpm isolates each differently:

1. **Env-var-based (isolated)** — tokens live in the per-profile MCP fragment. Each profile can carry a different value. Use `ccpm mcp add <name> --env KEY=VALUE --profile <name>`.
2. **OAuth MCPs (isolated)** — Claude Code caches OAuth tokens inside `<CLAUDE_CONFIG_DIR>/.claude.json`, which is per-profile.
3. **Globally-cached MCPs (shared)** — MCPs that write to `~/.config/<service>` or a fixed-name keychain entry are shared across every profile. ccpm cannot isolate them without upstream changes.

## Known limitations

- **VS Code extension**: The Claude VS Code extension always reads from `~/.claude`. Use `ccpm set-default` to point it at a ccpm profile. On macOS this copies the namespaced keychain entry into the default slot.
- **Windows without Developer Mode**: ccpm falls back to copying shared assets instead of symlinking, and writes a marker at `~/.ccpm/.windows-copy-fallback`. Turn on Developer Mode for true deduplication.
- **Globally-cached MCP servers** (see the MCP auth model above) cannot be isolated per profile.
- **Linux headless**: `go-keyring` requires D-Bus and a secret service (gnome-keyring or kwallet). On headless servers, API key profiles need a running secret service.

## Build from source

```bash
git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/cli
go build -o ccpm .
./ccpm --version
```

## Contributing

Contributions are welcome. Please open an issue first to discuss what you want to change.

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/your-feature`)
3. Make your changes
4. Run tests (`cd cli && go test ./...`)
5. Open a pull request

## License

MIT

## Author

Built by [Nitin Gupta](https://x.com/nitingupta__7).
