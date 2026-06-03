# ccpm

**Run multiple Claude Code accounts in parallel. Fully isolated. One command.**

[![CI](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@ngcodes/ccpm)](https://www.npmjs.com/package/@ngcodes/ccpm)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

ccpm (Claude Code Profile Manager) keeps multiple Claude Code profiles isolated on one machine. Each profile has its own credentials, settings, MCP servers, plugins, skills, and memory. Open two terminals, run two accounts.

## Why

Claude Code reads its config from a single directory (`~/.claude`), so without ccpm you can only be signed into one account at a time. ccpm gives every profile its own config directory and sets `CLAUDE_CONFIG_DIR` when launching claude. Two terminals, two profiles, zero conflicts.

## Install

```bash
# npm
npm i -g @ngcodes/ccpm

# curl (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh

# go
go install github.com/nitin-1926/claude-code-profile-manager/ccpm@latest
```

## Quick start

```bash
# Create profiles (each prompts for OAuth or API key)
ccpm add personal
ccpm add work

# Run in parallel
ccpm run personal    # terminal 1
ccpm run work        # terminal 2

# Check status
ccpm list
```

Sample output:

```
NAME       AUTH      STATUS
personal   oauth     ✓ nitin@gmail.com
work       api_key   ✓ sk-ant-...7f2k   ★
```

## Changelog

Release highlights and fixes: **[ccpm.dev/changelog](https://ccpm.dev/changelog)**.

## Key features

- **Parallel sessions**: run different Claude Code accounts in different terminals at the same time.
- **OAuth + API key** per profile, with credentials in the OS keychain.
- **Per-profile assets**: skills, agents, commands, rules, hooks, plugins, MCP servers, and settings install globally, per-profile, or per-project.
- **MCP transports**: stdio, HTTP, SSE. `ccpm mcp auth <server> --profile <p>` completes OAuth in the right profile.
- **Permissions and hooks** without hand-editing JSON.
- **IDE default**: `ccpm set-default` pins a profile for direct `claude` launches (VS Code, Cursor, Antigravity). On macOS this is set system-wide via a LaunchAgent.
- **Encrypted vault**: AES-256-GCM credential backups with a master key in your OS keychain.

## Commands

| Command                                                   | Description                                                          |
| --------------------------------------------------------- | -------------------------------------------------------------------- |
| `ccpm add <name>`                                         | Create a new profile (OAuth or API key) via an interactive wizard    |
| `ccpm run <name> [args...]`                               | Launch claude with the profile (unknown flags forward to claude)     |
| `ccpm use <name>`                                         | Set the profile for the current shell session                        |
| `ccpm list` / `ls`                                        | List profiles with auth status (`--json` for machine-readable output) |
| `ccpm status [name]`                                      | System overview, or auth health for one profile                      |
| `ccpm rename <old> <new>`                                 | Rename a profile (migrates keychain entries and plugin paths)        |
| `ccpm remove <name>` / `rm`                               | Delete a profile (`--force` to skip confirm)                         |
| `ccpm clone <src> <new>`                                  | Duplicate a profile (assets + settings + auth; `--no-auth` available) |
| `ccpm set-default [name]` / `ccpm unset-default`          | Pin or clear the default profile for direct `claude` launches        |
| `ccpm prompt`                                             | Print the active profile name for a shell prompt (PS1 / starship)    |
| `ccpm sync`                                               | Re-apply global installs into one or all profiles                    |
| `ccpm doctor`                                             | Health check: env, auth, drift, symlinks, cascade (`--fix` prunes symlinks) |
| `ccpm export / import-bundle`                             | Export a profile to a portable `.tar.gz` and restore it elsewhere    |
| `ccpm consolidate`                                        | Audit (and optionally `--fix`) asset drift across scopes             |
| `ccpm default fingerprint update / check`                 | Record or diff the `~/.claude` drift baseline                        |
| `ccpm trust add / remove / list`                          | Grant/revoke trust for project `.claude/` hooks, permissions, MCP    |
| `ccpm import default / from-profile`                      | Import assets from `~/.claude` or clone from another ccpm profile    |
| `ccpm skill / agent / command / rule add|remove|list|link`| Manage per-profile or global Claude Code asset trees                 |
| `ccpm plugin list / enable / disable / install / remove`  | Marketplaces + per-profile activation; `gc` cleans the shared cache  |
| `ccpm hooks add / remove / list`                          | Manage hook entries in profile settings                              |
| `ccpm mcp add / remove / list / import / auth`            | Manage MCP servers (stdio, http, sse) at global/profile/project scope |
| `ccpm env set / unset / list`                             | Persist env vars per profile (injected at `ccpm run`)                |
| `ccpm permissions allow / ask / deny / remove / list / mode` | Manage `permissions.*` rules and defaultMode                       |
| `ccpm sessions list <profile>`                            | List Claude Code sessions stored in a profile                        |
| `ccpm settings set / get / apply / show / statusline / outputstyle` | Manage Claude Code settings per profile                    |
| `ccpm auth status / refresh / backup / restore`           | Manage authentication and the encrypted vault                        |
| `ccpm config set / get`                                   | `cascade_auto_adopt`, `check_default_drift`, `default_dir` (get-only) |
| `ccpm shell-init`                                         | Print the shell hook (zsh / bash / fish / powershell)                |
| `ccpm completion <shell>`                                 | Generate a shell completion script (bash / zsh / fish / powershell)  |
| `ccpm uninstall`                                          | Remove all profiles, keychain entries, vault, and `~/.ccpm/`         |

Full command reference and guides: **[ccpm.dev/docs](https://ccpm.dev/docs)**.

`ccpm run` intercepts four flags before forwarding to claude: `--ccpm-env KEY=VAL` (one-shot env override, repeatable), `--no-auto-adopt` (skip the host-asset cascade scan), `--help`, `--version`. Use `--` to forward `--help` or `--version` to claude.

## How it works

1. `ccpm add` creates `~/.ccpm/profiles/<name>/` with its own config and credentials.
2. `ccpm run` merges shared settings/MCP fragments, sets `CLAUDE_CONFIG_DIR`, and execs `claude`.
3. Each terminal gets a completely isolated Claude Code instance.

Assets install globally (`--global`, stored in `~/.ccpm/share/`) or per-profile (`--profile <name>`). For settings, the cross-profile baseline is the native `~/.claude/settings.json`; ccpm merges it into every profile at launch. Per-repo overrides in `./.claude/settings.json` are honored automatically.

No daemons. No patches. No magic.

## Privacy and security

ccpm is 100% local. It never makes network requests, collects data, or phones home.

- API keys live in the OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager).
- Vault backups use AES-256-GCM with a master key in your OS keychain.
- All data lives in `~/.ccpm/`. No telemetry, no analytics, no tracking.

## Platform support

| Feature            | macOS ✓ verified                         | Windows ⚠ experimental                    | Linux ⚠ experimental |
| ------------------ | ---------------------------------------- | ----------------------------------------- | -------------------- |
| OAuth per-profile  | Keychain entry namespaced by profile dir | wincred entry, same namespacing model     | `.credentials.json`  |
| API key storage    | Keychain                                 | Credential Manager                        | Secret Service       |
| Parallel sessions  | Yes                                      | Yes                                       | Yes                  |
| Shared asset dedup | Symlinks                                 | Symlinks (Developer Mode) or copy         | Symlinks             |
| Shell hook         | zsh, bash, fish                          | PowerShell                                | zsh, bash, fish      |
| `set-default` (system-wide GUI) | LaunchAgent + `launchctl setenv` | Keychain / identity only              | Keychain / identity only |

> Requires **Claude Code v2.1.56+** on macOS for OAuth isolation. `ccpm doctor` warns when your Claude Code is too old.

## MCP authentication model

1. **Env-var MCPs** (e.g. `GITHUB_TOKEN`): isolated per profile. Configure with `ccpm mcp add <name> --env KEY=VALUE --profile <p>`.
2. **OAuth MCPs**: isolated, because tokens cache inside `<CLAUDE_CONFIG_DIR>/.claude.json` which is per-profile.
3. **Globally-cached MCPs** (servers that write to `~/.config/<service>` or a non-namespaced OS keychain entry): **shared** across profiles. ccpm cannot isolate them.

## Known limitations

- **IDE extensions ignore `CLAUDE_CONFIG_DIR`**: VS Code, Cursor, and Antigravity launch claude directly. Use `ccpm set-default` to pin a profile for them. macOS is system-wide; Linux and Windows keep keychain and identity sync but no LaunchAgent equivalent yet.
- **Windows without Developer Mode**: ccpm falls back to copying shared assets instead of symlinking and writes a marker at `~/.ccpm/.windows-copy-fallback`.
- **Globally-cached MCP servers** cannot be isolated per profile (see the MCP auth model).
- **Headless Linux**: `go-keyring` requires D-Bus and a secret service. API-key profiles need one running.

## Build from source

```bash
git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/ccpm
go build -o ccpm .
./ccpm --version
```

## Contributing

Open an issue first to discuss what you want to change, then fork, branch, test (`cd ccpm && go test ./...`), and open a PR.

## License

MIT

## Author

Built by [Nitin Gupta](https://x.com/nitingupta__7).
