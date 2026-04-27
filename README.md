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
go install github.com/nitin-1926/claude-code-profile-manager/ccpm@latest
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
- **First-class asset management**: skills, agents, commands, rules, hooks, and MCP servers — install globally, per-profile, or per-project (`--scope global|profile|project` for MCP; `--global`/`--profile` for the rest)
- **MCP transports**: stdio, HTTP, and SSE. Remote MCPs authenticate via `ccpm mcp auth` so OAuth tokens land in the right profile.
- **Plugin activation per profile**: override which Claude Code plugins are enabled for each profile via `ccpm plugin enable|disable`
- **Permissions UI**: `ccpm permissions allow|ask|deny|mode` writes directly to `permissions.{allow,ask,deny,defaultMode}` so you never have to hand-edit JSON.
- **Per-profile env vars**: `ccpm env set KEY=VAL --profile <name>` persists env vars per profile; `ccpm run --ccpm-env KEY=VAL` overlays one-shot overrides.
- **Transparent arg forwarding**: unknown flags after the profile name (`--dangerously-skip-permissions`, `--model`, ...) flow through to claude without needing a `--` separator.
- **Session listing**: `ccpm sessions list <profile>` browses Claude Code's session .jsonl files.
- **Shared asset store**: directory-based assets are symlinked from `~/.ccpm/share/` into profiles for deduplication
- **Settings materialization**: full native-Claude-compatible merge hierarchy — existing profile state → `~/.claude/settings.json` → profile fragment → project `.claude/settings.{json,local.json}` → enterprise managed-settings (org policy, highest precedence).
- **Encrypted vault**: AES-256-GCM encrypted credential backups with master key in your OS keychain
- **IDE support**: set the default profile for VS Code with `ccpm set-default`
- **Shell integration**: `ccpm use` sets the profile for your entire shell session
- **Cross-platform**: macOS Keychain, Linux Secret Service, Windows Credential Manager

## Commands

### Profile management

| Command                   | Description                                    |
| ------------------------- | ---------------------------------------------- |
| `ccpm add <name>`         | Create a new profile (OAuth or API key)        |
| `ccpm run <name> [...]`   | Launch Claude Code with a profile. Unknown flags after `<name>` flow through to claude (no `--` separator needed). Pass `--ccpm-env KEY=VAL` for one-shot env overrides; use `-- --help` / `-- --version` to forward those two to claude. |
| `ccpm use <name>`         | Set profile for the current shell session      |
| `ccpm list`               | List all profiles and their status             |
| `ccpm status`             | Show system overview                           |
| `ccpm set-default <name>` | Set the default profile for IDEs               |
| `ccpm remove <name>`      | Delete a profile                               |
| `ccpm shell-init`         | Print shell hook for `ccpm use` support        |
| `ccpm sync`               | Sync global installs into all (or one) profile |

### Assets (skills, agents, commands, rules)

All four share the same command shape. Replace `skill` with `agent`, `command`, or `rule`.

| Command                                 | Description                                |
| --------------------------------------- | ------------------------------------------ |
| `ccpm skill add <path> --global`        | Install a skill for all profiles           |
| `ccpm skill add <path> --profile work`  | Install a skill for one profile            |
| `ccpm skill remove <name> --global`     | Remove a skill from all profiles           |
| `ccpm skill link <name> --profile work` | Link a shared skill into a profile         |
| `ccpm skill list`                       | List installed skills                      |
| `ccpm agent add <path> --global`        | Install a Claude Code agent (same pattern) |
| `ccpm command add <path> --profile x`   | Install a custom slash command             |
| `ccpm rule add <path> --global`         | Install a rule file                        |

Source may be a directory (skills require a `SKILL.md` marker) or a single file (agents/commands/rules are usually `.md` files). Pass `--live-symlink` to keep the source linked so edits show up live, or `--copy` to snapshot it.

### Plugins

Plugin files are installed by Claude Code itself via `/plugin install <name>` inside a session. ccpm manages the `enabledPlugins` key per profile so you can override which plugins are active where.

| Command                                                           | Description                                             |
| ----------------------------------------------------------------- | ------------------------------------------------------- |
| `ccpm plugin list`                                                | Show installed plugins + enabled state per profile      |
| `ccpm plugin list --profile work`                                 | Limit output to one profile                             |
| `ccpm plugin enable <name@marketplace> --profile work`            | Turn on a plugin for one profile                        |
| `ccpm plugin disable <name@marketplace> --profile work`           | Turn off a globally-enabled plugin in one profile       |

Global activation (affecting all profiles) lives in `~/.claude/settings.json` under `enabledPlugins` — Claude Code writes there automatically when you install a plugin with user scope.

### Hooks

| Command                                              | Description                                         |
| ---------------------------------------------------- | --------------------------------------------------- |
| `ccpm hooks add PreToolUse "<cmd>" --profile work`   | Append a hook to an event                           |
| `ccpm hooks add PostToolUse "<cmd>" --matcher Edit`  | Restrict to a tool-name pattern                     |
| `ccpm hooks remove PreToolUse --profile work`        | Remove the last entry (or `--index <i>`)            |
| `ccpm hooks list --profile work`                     | Show merged hooks for a profile                     |

Events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`. Hook shell scripts in `~/.claude/hooks/` are managed separately via `ccpm import default --only hooks`.

### MCP servers

| Command                                                                                        | Description                                                        |
| ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `ccpm mcp add <name> --scope global --command <cmd>`                                           | Add a stdio MCP for all profiles (ccpm global fragment)            |
| `ccpm mcp add <name> --scope profile --profile work --command <cmd>`                           | Add a stdio MCP for one profile                                    |
| `ccpm mcp add <name> --scope project --command <cmd>`                                          | Add a stdio MCP to the current repo's `.mcp.json`                  |
| `ccpm mcp add <name> --scope profile --profile work --transport http --url <url>`              | Add a remote HTTP MCP (use `--header KEY=VAL` for auth tokens)     |
| `ccpm mcp add <name> --transport sse --url <url>`                                              | Add an SSE MCP (same shape as http)                                |
| `ccpm mcp auth <name> --profile work`                                                          | Complete OAuth for a remote MCP in the profile's scope             |
| `ccpm mcp remove <name> --scope <global\|profile\|project>`                                    | Remove an MCP server                                               |
| `ccpm mcp import <file.json> --scope <global\|profile\|project>`                               | Import MCP servers from a JSON file (accepts `{mcpServers:{...}}`) |
| `ccpm mcp list`                                                                                | List MCPs with source (ccpm-global / ccpm-profile / host / project) |

`--global` and `--profile <name>` are still accepted as aliases for `--scope global` / `--scope profile`. For `--scope project`, ccpm discovers the project root by walking up from CWD looking for `.claude/settings.json`, `.claude/settings.local.json`, or `.mcp.json` — or pass `--project-dir <path>` explicitly.

### Permissions

`ccpm permissions` manages `permissions.{allow,ask,deny,defaultMode}` in the profile fragment (or, with `--global`, in `~/.claude/settings.json`).

| Command                                                                   | Description                                                  |
| ------------------------------------------------------------------------- | ------------------------------------------------------------ |
| `ccpm permissions allow "Bash(git status:*)" --profile work`              | Add a rule to `permissions.allow`                            |
| `ccpm permissions ask "Edit(**/*.md)" --profile work`                     | Add a rule to `permissions.ask`                              |
| `ccpm permissions deny "Bash(rm:*)" --profile work`                       | Add a rule to `permissions.deny`                             |
| `ccpm permissions remove "Bash(git status:*)" --profile work`             | Strip a rule from all three lists                            |
| `ccpm permissions list --profile work`                                    | Show all rules + the default mode                            |
| `ccpm permissions mode <default\|acceptEdits\|plan\|auto\|dontAsk\|bypassPermissions> --profile work` | Set `permissions.defaultMode` |

Adding to one bucket automatically removes from the other two so the lists stay disjoint.

### Environment variables

`ccpm env` persists env vars on a profile; they're layered in below the parent process env at every `ccpm run`.

| Command                                                          | Description                                      |
| ---------------------------------------------------------------- | ------------------------------------------------ |
| `ccpm env set KEY=VALUE [KEY=VALUE...] --profile work`           | Persist one or more env vars on the profile     |
| `ccpm env unset KEY [KEY...] --profile work`                     | Remove env vars from the profile                |
| `ccpm env list --profile work`                                   | List persisted env vars                         |
| `ccpm run work --ccpm-env KEY=VALUE` (repeatable)                | One-shot env override at launch time            |

`CLAUDE_CONFIG_DIR` and `ANTHROPIC_API_KEY` are reserved — ccpm always computes them — and cannot be set via `ccpm env`. Use `--ccpm-env` for a one-shot override when you really need to.

### Sessions

| Command                              | Description                                                            |
| ------------------------------------ | ---------------------------------------------------------------------- |
| `ccpm sessions list <profile>`       | Show sessions from `<profileDir>/projects/<encoded-cwd>/*.jsonl`        |
| `ccpm sessions list <profile> --all` | Show sessions from every project the profile has worked on             |

By default, `ccpm sessions list` is scoped to the current working directory, mirroring how native `claude --resume` scopes its picker.

### Import

| Command                                            | Description                                                                    |
| -------------------------------------------------- | ------------------------------------------------------------------------------ |
| `ccpm import default --profile <name>`             | Import skills/commands/hooks/agents/settings from `~/.claude` into one profile |
| `ccpm import default --all --only skills`          | Import specific targets into every profile                                     |
| `ccpm import default --profile <name> --no-share`  | Copy assets directly instead of symlinking from the shared store               |
| `ccpm import from-profile --src <a> --profile <b>` | Clone assets from one ccpm profile into another                                |

### Settings

ccpm no longer manages its own global settings layer. Shared-across-profiles
defaults go in `~/.claude/settings.json` (the file native Claude Code already
uses); ccpm merges it into every profile at launch. Use `--profile` for
per-account overrides, and per-repo `.claude/settings.json` for project
overrides.

| Command                                               | Description                                                                |
| ----------------------------------------------------- | -------------------------------------------------------------------------- |
| `ccpm settings set <key> <value> --profile work`      | Set a setting for one profile                                              |
| `ccpm settings apply <file.json> --profile work`      | Apply a JSON settings fragment                                             |
| `ccpm settings get <key> --profile work`              | Get the effective value of a setting                                       |
| `ccpm settings show --profile work`                   | Show full merged settings for a profile                                    |
| `ccpm settings statusline "<cmd>" --profile work`     | Set the native `statusLine` block (`{type: "command", command: "<cmd>"}`). Empty `""` removes it. |
| `ccpm settings outputstyle <style> --profile work`    | Set `outputStyle` (allowlist: default, Build, Explanatory, Learning, Direct) |

### Authentication

| Command                    | Description                                |
| -------------------------- | ------------------------------------------ |
| `ccpm auth status`         | Check credential validity for all profiles |
| `ccpm auth refresh <name>` | Re-authenticate a profile                  |
| `ccpm auth backup <name>`  | Create encrypted credential backup         |
| `ccpm auth restore <name>` | Restore credentials from backup            |

## How it works

ccpm uses one official mechanism: the `CLAUDE_CONFIG_DIR` environment variable.

1. `ccpm add` creates `~/.ccpm/profiles/<name>/` with its own config and credentials
2. `ccpm run` merges shared settings/MCP fragments, sets `CLAUDE_CONFIG_DIR`, and execs `claude`
3. Each terminal gets a completely isolated Claude Code instance

### Global vs per-profile installs

For **skills**, **agents**, **commands**, **rules**, **hooks**, and **MCP servers**, `--global` installs into the shared store (`~/.ccpm/share/`) and is linked/merged into every profile; new profiles inherit global installs automatically. `--profile <name>` applies only to one.

**Plugins** are different: Claude Code owns the install pipeline (`/plugin install` inside a session), so ccpm does not manage plugin files. ccpm only manages the `enabledPlugins` activation key per profile, so the same installed plugin can be on in one profile and off in another.

For **settings**, ccpm does **not** maintain a separate global layer. The shared baseline is `~/.claude/settings.json` — the same file native Claude Code reads when you run it without ccpm. Edit it directly (or run `claude /config …` natively) and every ccpm profile picks up the change on the next launch. Use `ccpm settings set --profile <name>` only for per-profile overrides.

```
~/.ccpm/
├── config.json          # profile registry
├── installs.json        # manifest of installed assets (skills/agents/commands/rules/hooks/mcp)
├── share/
│   ├── skills/          # shared skill directories (symlinked into profiles)
│   ├── agents/          # shared agent files
│   ├── commands/        # shared slash-command files
│   ├── rules/           # shared rule files
│   ├── hooks/           # shared hook scripts
│   ├── mcp/             # MCP server fragments (global.json, <profile>.json)
│   └── settings/        # per-profile settings fragments (<profile>.json)
├── profiles/
│   ├── personal/        # CLAUDE_CONFIG_DIR for "personal"
│   │   ├── skills/      # symlinks → share/skills/*
│   │   ├── agents/      # symlinks → share/agents/*
│   │   └── settings.json # materialized at launch
│   └── work/
└── vault/               # encrypted credential backups
```

**Settings merge order** (lowest → highest, higher wins):

1. `<profile>/settings.json` existing state (preserves keys Claude auto-wrote)
2. `~/.claude/settings.json` — shared baseline
3. `~/.ccpm/share/settings/<profile>.json` — ccpm-managed per-profile fragment
4. Profile owned-keys re-assertion (values set via `ccpm settings set`)
5. `./.claude/settings.json` at project root
6. `./.claude/settings.local.json` at project root (gitignored local overrides)
7. Enterprise managed-settings — `/Library/Application Support/ClaudeCode/managed-settings.json` (macOS), `/etc/claude-code/managed-settings.json` (Linux), `C:\ProgramData\ClaudeCode\managed-settings.json` (Windows), plus sibling `managed-settings.d/*.json` drop-ins merged alphabetically. Highest precedence so org policy always wins.

**MCP merge order**: host `~/.claude.json#mcpServers` → ccpm global fragment → ccpm profile fragment → project `.claude/settings.json#mcpServers` → project `.mcp.json` → managed `mcpServers` (highest).

Objects merge key-by-key; arrays and scalars from a higher-precedence source replace the lower one.

No daemons. No patches. No magic.

## Privacy and security

ccpm is 100% local. It never makes network requests, never collects data, and never phones home.

- API keys are stored in your OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager)
- Vault backups use AES-256-GCM encryption with a master key in your OS keychain
- All data lives in `~/.ccpm/` on your machine
- No telemetry, analytics, or tracking

## Platform support

| Feature            | macOS                                    | Linux               | Windows                               |
| ------------------ | ---------------------------------------- | ------------------- | ------------------------------------- |
| OAuth per-profile  | Keychain entry namespaced by profile dir | `.credentials.json` | `.credentials.json`                   |
| API key storage    | Keychain                                 | Secret Service      | Credential Manager                    |
| Parallel sessions  | Yes                                      | Yes                 | Yes                                   |
| Shared skill dedup | Symlinks (`~/.ccpm/share`)               | Symlinks            | Symlinks (Developer Mode) or copy[^1] |
| Shell hook         | zsh, bash, fish                          | zsh, bash, fish     | PowerShell                            |

[^1]: Without Developer Mode or admin, Windows users cannot create symlinks; ccpm falls back to copying and leaves a marker at `~/.ccpm/.windows-copy-fallback`. Turn on Developer Mode for true deduplication.

> **Requires Claude Code `v2.1.56` or newer for macOS OAuth isolation.** Earlier versions share a single keychain entry across all profiles, so multiple OAuth profiles cannot be kept authenticated at the same time. `ccpm doctor` prints a warning if your Claude Code is too old.

## MCP authentication model

MCP servers authenticate in one of three ways, and ccpm isolates each differently:

1. **Environment-variable MCPs (e.g. `GITHUB_TOKEN`)** — stored inside the per-profile MCP fragment at `~/.ccpm/share/mcp/<profile>.json`. Each profile can carry a different token. Configure with `ccpm mcp add <name> --env KEY=VALUE --profile <name>`.
2. **OAuth MCPs (servers that open a browser)** — Claude Code caches tokens inside `<CLAUDE_CONFIG_DIR>/.claude.json` under `mcpOAuth`. Because `CLAUDE_CONFIG_DIR` is per-profile, OAuth sessions are automatically isolated.
3. **Globally-cached MCPs (servers that write to `~/.config/<service>` or the user keychain under a fixed service name)** — these are **shared across profiles**. ccpm cannot isolate them without cooperation from the MCP server itself. Treat them as "one account for all profiles" and plan accordingly.

## Known limitations

- **VS Code extension**: The Claude VS Code extension always reads from `~/.claude`. Use `ccpm set-default` to point it at a specific ccpm profile. On macOS, `set-default` copies the profile's namespaced keychain OAuth entry into the default slot; on Linux/Windows it copies `.credentials.json`.
- **Linux headless**: `go-keyring` requires D-Bus and a secret service (gnome-keyring or kwallet). On headless servers, API-key profiles need a running secret service.
- **Globally-cached MCP servers**: see the MCP auth section above — these cannot be isolated across profiles.

## Build from source

```bash
git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/ccpm
go build -o ccpm .
./ccpm --version
```

## Releasing

The `scripts/release.sh` script handles the full end-to-end release (bump → verify → tag → GitHub Release → npm publish) with preflight checks so you can't ship a broken release by accident.

```bash
# Bump 0.1.0 → 0.1.1, run full release
./scripts/release.sh patch

# Bump 0.1.0 → 0.2.0
./scripts/release.sh minor

# Bump 0.1.0 → 1.0.0
./scripts/release.sh major

# Explicit version
./scripts/release.sh 0.3.0

# See what would happen without changing anything
./scripts/release.sh patch --dry-run
```

Flags: `--skip-tests`, `--skip-npm` (GitHub release only), `--stash` (auto-stash uncommitted work for the release and pop it back on exit), `--allow-dirty` (unsafe; uncommitted changes will not be in the tag/binary/npm package), `-y` (skip confirmation). See `scripts/release.sh --help` for the full list.

Preflight checks the script runs before touching anything: `git`/`go`/`node`/`npm`/`gh` on PATH, on `main`, clean working tree, in sync with `origin/main`, logged in to `gh`, logged in to npm with publish access to `@ngcodes/ccpm`, and the target tag is unused locally, on origin, and on GitHub Releases. The release only starts if every check passes.

### Releasing a subset of in-flight work

If you have a pile of uncommitted changes in your tree and only want to ship some of them, commit the subset you want to release and use `--stash` to set the rest aside:

```bash
# stage + commit only the files you want in this release
git add cli/cmd/foo.go cli/internal/bar.go
git commit -m "feat: ship foo and bar"

# release just those; the rest of your tree is stashed and restored on exit
./scripts/release.sh patch --stash
```

`--stash` uses `git stash push --include-untracked` with a unique label, installs an `EXIT` trap that pops the stash back whether the release succeeds or fails, and preserves the original staged/unstaged split via `git stash pop --index`. If the pop hits a conflict (rare — only if your stashed work touched `cli/cmd/root.go` or `npm/package.json`), the script leaves the stash in place and tells you the ref so you can resolve it manually.

`--allow-dirty` is different and intentionally limited: it lets the release proceed with a dirty tree but does **not** include your uncommitted changes in the tag, binary, or npm package. Use it only if you know what you're doing.

## Contributing

Contributions are welcome. Please open an issue first to discuss what you want to change.

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/your-feature`)
3. Make your changes
4. Run tests (`cd ccpm && go test ./...`)
5. Open a pull request

## License

MIT

## Author

Built by [Nitin Gupta](https://x.com/nitingupta__7).
