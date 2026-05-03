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
