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
- **Asset management**: skills, agents, commands, rules, hooks install globally or per-profile; MCP servers install globally, per-profile, or per-project (`ccpm mcp add --scope global|profile|project`)
- **MCP transports**: stdio, HTTP, and SSE. `ccpm mcp auth <server> --profile <p>` runs native claude's OAuth flow scoped to the profile.
- **Plugin activation per profile**: override `enabledPlugins` so a plugin can be on in one profile and off in another
- **Permissions UI**: `ccpm permissions allow|ask|deny|mode` writes `permissions.{allow,ask,deny,defaultMode}` directly — no JSON surgery needed
- **Per-profile env vars**: `ccpm env set` persists them; `ccpm run --ccpm-env KEY=VAL` overlays one-shot overrides. Unknown flags (`--dangerously-skip-permissions`, `--model`, ...) flow through to claude with no `--` separator needed.
- **Sessions**: `ccpm sessions list <profile>` browses Claude Code's session files
- **Settings**: native-Claude-compatible merge — `~/.claude/settings.json` → profile fragment → project `.claude/settings.{json,local.json}` → enterprise managed-settings (org policy, highest precedence)
- **Encrypted vault**: AES-256-GCM encrypted credential backups with master key in your OS keychain
- **IDE support**: set the default profile for VS Code with `ccpm set-default`
- **Shell integration**: `ccpm use` sets the profile for your entire shell session
- **Cross-platform**: macOS Keychain, Linux Secret Service, Windows Credential Manager

## Commands

| Command                                                      | Description                                                               |
| ------------------------------------------------------------ | ------------------------------------------------------------------------- |
| `ccpm add <name>`                                            | Create a new profile (OAuth or API key) with an interactive import wizard |
| `ccpm run <name> [claude-args...]`                           | Launch Claude Code with a profile (unknown flags forward to claude)       |
| `ccpm use <name>`                                            | Set profile for the current shell session                                 |
| `ccpm list`                                                  | List all profiles and their status                                        |
| `ccpm status`                                                | Show system overview                                                      |
| `ccpm doctor`                                                | Diagnose env, auth health, root-vs-profile drift, symlink integrity       |
| `ccpm set-default <name>`                                    | Set the default profile for IDEs                                          |
| `ccpm remove <name>`                                         | Delete a profile                                                          |
| `ccpm sync`                                                  | Sync global installs into profiles                                        |
| `ccpm import default` / `ccpm import from-profile`           | Import assets from `~/.claude` or clone from another ccpm profile         |
