# ccpm

**Run multiple Claude Code accounts in parallel. Fully isolated. One command.**

[![CI](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/nitin-1926/claude-code-profile-manager/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@ngcodes/ccpm)](https://www.npmjs.com/package/@ngcodes/ccpm)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

ccpm (Claude Code Profile Manager) lets you keep multiple isolated Claude Code profiles on one machine. Each profile has its own credentials, settings, MCP servers, plugins, skills, and memory. Open two terminals, run two different accounts at the same time.

## Why

Claude Code reads its config from a single directory (`~/.claude`). Without ccpm you can only be signed into one account at a time and switching means logging out, swapping files, or both.

ccpm gives every profile its own config directory and sets `CLAUDE_CONFIG_DIR` to it when you launch claude. Two terminals, two profiles, zero conflicts.

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

# Run them in parallel
ccpm run personal    # terminal 1
ccpm run work        # terminal 2

# See what you have
ccpm list
```

Sample output:

```text
NAME       AUTH      STATUS
personal   oauth     ✓ nitin@gmail.com
work       api_key   ✓ sk-ant-...7f2k   ★
```

### Migrating your existing `~/.claude`

If you already use Claude Code, your first profile doesn't have to start empty. When `~/.claude` exists, the `ccpm add` wizard offers to **import your current config** — skills, MCP servers, hooks, agents, commands, rules, and settings all carry over into the new profile.

```bash
# the wizard prompts: start empty, import from ~/.claude, or clone a profile
ccpm add personal
```

To do it explicitly (or to pull in changes you made to `~/.claude` later), run `ccpm import default` — see the [Import](#import) section for `--only`, `--all`, and dry-run flags.

## Changelog

Release notes live on the docs site: **[ccpm.dev/changelog](https://ccpm.dev/changelog)**.

## Key features

- **Parallel sessions**: run different Claude Code accounts side-by-side, fully isolated.
- **OAuth + API key** per profile, with credentials in the OS keychain.
- **Per-profile assets**: skills, agents, commands, rules, hooks, plugins, MCP servers, and settings install globally, per-profile, or per-project.
- **MCP transports**: stdio, HTTP, and SSE. Remote MCPs authenticate via `ccpm mcp auth` so OAuth tokens land in the right profile.
- **Permissions and hooks** without hand-editing JSON.
- **Transparent arg forwarding**: unknown flags after `ccpm run <profile>` flow through to claude with no `--` separator.
- **IDE default**: `ccpm set-default` pins a profile for direct `claude` launches (VS Code, Cursor, Antigravity, any GUI extension). On macOS this is set system-wide via a LaunchAgent.
- **Encrypted vault**: AES-256-GCM credential backups with a master key in your OS keychain.
- **Shared store**: directory-based assets are symlinked from `~/.ccpm/share/` into profiles for deduplication.
- **macOS is the verified platform** today. Linux and Windows builds compile and ship; OAuth `set-default`, `auth backup/restore`, and keychain-based `status` on those platforms are experimental until verified on real hardware.

## Commands

### Profile lifecycle

| Command                       | Description                                                                              |
| ----------------------------- | ---------------------------------------------------------------------------------------- |
| `ccpm add <name>`             | Create a new profile (interactive: import wizard + OAuth or API key)                     |
| `ccpm run <name> [...]`       | Launch Claude Code with a profile. Unknown flags forward to claude.                      |
| `ccpm use <name>`             | Set the profile for the current shell session (requires `ccpm shell-init`)               |
| `ccpm list` / `ls`            | List profiles with auth status (`--json` for machine-readable output)                    |
| `ccpm status [name]`          | System overview, or auth health for one profile                                          |
| `ccpm rename <old> <new>`     | Rename a profile (migrates keychain entries and plugin paths)                            |
| `ccpm remove <name>` / `rm`   | Delete a profile (`--force` to skip confirm)                                             |
| `ccpm clone <src> <new>`      | Duplicate a profile (assets + settings + auth; `--no-auth` for assets only)              |
| `ccpm set-default [name]`     | Pin the default profile for direct `claude` launches; macOS sets it system-wide          |
| `ccpm unset-default`          | Clear the default                                                                        |
| `ccpm prompt`                 | Print the active profile name for a shell prompt (PS1 / starship / p10k)                 |
| `ccpm sync`                   | Re-apply global installs into one or all profiles                                        |
| `ccpm doctor`                 | Health check: env, auth, drift, symlinks, cascade (`--fix` prunes dangling symlinks)     |
| `ccpm consolidate`            | Audit (and optionally `--fix`) asset drift across host, share, and profile scopes        |
| `ccpm export <name>`          | Export a profile to a portable `.tar.gz` (credentials excluded by default)               |
| `ccpm import-bundle <file>`   | Restore a profile from an export bundle                                                  |
| `ccpm shell-init`             | Print the shell hook (auto-detects zsh / bash / fish / powershell)                       |
| `ccpm completion <shell>`     | Generate a shell completion script (bash / zsh / fish / powershell)                      |
| `ccpm uninstall`              | Remove all profiles, keychain entries, vault backups, and `~/.ccpm/`                     |

`ccpm run` intercepts four flags before forwarding to claude: `--ccpm-env KEY=VAL` (one-shot env override, repeatable), `--no-auto-adopt` (skip the host-asset cascade scan for this launch), `--help`, `--version`. Use `--` to forward `--help` or `--version` to claude.

### Assets: skills, agents, commands, rules

All four share the same command shape. Replace `skill` with `agent`, `command`, or `rule`.

| Command                                 | Description                                |
| --------------------------------------- | ------------------------------------------ |
| `ccpm skill add <path> --global`        | Install for every profile                  |
| `ccpm skill add <path> --profile <p>`   | Install for one profile                    |
| `ccpm skill remove <name> --global`     | Remove from every profile (alias `rm`)     |
| `ccpm skill link <name> --profile <p>`  | Link a shared asset into a profile         |
| `ccpm skill list`                       | List installed (alias `ls`)                |

Source may be a directory (skills require a `SKILL.md` marker) or a single file (agents/commands/rules are usually `.md`). Pass `--live-symlink` to keep the source linked so edits show up live, or `--copy` to snapshot it.

### Plugins

ccpm installs plugins itself, end-to-end, without a Claude Code session. Marketplaces clone into a shared store, plugin files cache once and symlink into each profile, and per-profile activation lives in the same settings fragment ccpm uses for everything else.

| Command                                                            | Description                                                  |
| ------------------------------------------------------------------ | ------------------------------------------------------------ |
| `ccpm plugin marketplace add <org>/<repo>`                         | Clone a marketplace (HTTPS default; `--ssh` to use `git@`)   |
| `ccpm plugin marketplace list` / `remove <name>`                   | Inspect or drop a marketplace                                |
| `ccpm plugin install <name>@<marketplace> --global`                | Install into every profile and enable                        |
| `ccpm plugin install <name>@<marketplace> --profile <p>`           | Install into one profile only                                |
| `ccpm plugin install ... --install-only`                           | Install without enabling                                     |
| `ccpm plugin remove <name>@<marketplace> --global` / `--profile`   | Remove the plugin from every profile or just one             |
| `ccpm plugin list [--profile <p>]`                                 | Show installed plugins + per-profile enabled state           |
| `ccpm plugin enable / disable <name>@<marketplace> --profile <p>`  | Toggle activation                                            |
| `ccpm plugin gc`                                                   | Drop unreferenced shared-cache entries (also part of `sync`) |

Plugin clones default to HTTPS so an `https://github.com/<org>/<repo>.git` URL works without an SSH key. If you prefer SSH, pass `--ssh`.

### Hooks

| Command                                              | Description                              |
| ---------------------------------------------------- | ---------------------------------------- |
| `ccpm hooks add <event> "<cmd>" --profile <p>`       | Append a hook to an event                |
| `ccpm hooks add <event> "<cmd>" --matcher <regex>`   | Restrict to a tool-name pattern          |
| `ccpm hooks remove <event> --profile <p> [--index]`  | Remove the last entry, or one by index   |
| `ccpm hooks list --profile <p>`                      | Show merged hooks for a profile          |

Events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`. Hook shell scripts in `~/.claude/hooks/` are managed separately via `ccpm import default --only hooks`.

### MCP servers

| Command                                                                | Description                                          |
| ---------------------------------------------------------------------- | ---------------------------------------------------- |
| `ccpm mcp add <name> --scope global --command <cmd>`                   | Stdio MCP for every profile (ccpm global fragment)   |
| `ccpm mcp add <name> --scope profile --profile <p> --command <cmd>`    | Stdio MCP for one profile                            |
| `ccpm mcp add <name> --scope project --command <cmd>`                  | Stdio MCP in the current repo's `.mcp.json`          |
| `ccpm mcp add <name> --transport http --url <url> [--header K=V]`      | Remote HTTP MCP                                      |
| `ccpm mcp add <name> --transport sse --url <url>`                      | SSE MCP                                              |
| `ccpm mcp auth <name> --profile <p>`                                   | Complete OAuth in the profile's scope                |
| `ccpm mcp remove <name> --scope <global\|profile\|project>`            | Remove a server                                      |
| `ccpm mcp import <file.json> --scope ...`                              | Import from a JSON file (`{mcpServers:{...}}`)       |
| `ccpm mcp list`                                                        | List MCPs with source (global / profile / project)   |

`--global` and `--profile <name>` are accepted as aliases for `--scope global` / `--scope profile`. For `--scope project`, ccpm discovers the project root by walking up from CWD looking for `.claude/settings.json`, `.claude/settings.local.json`, or `.mcp.json`, or pass `--project-dir <path>` explicitly.

### Permissions

`ccpm permissions` manages `permissions.{allow,ask,deny,defaultMode}` in the profile fragment (or with `--global`, in `~/.claude/settings.json`). Adding to one bucket removes from the other two so the lists stay disjoint.

| Command                                                                                          | Description                       |
| ------------------------------------------------------------------------------------------------ | --------------------------------- |
| `ccpm permissions allow "Bash(git status:*)" --profile <p>`                                      | Add to `permissions.allow`        |
| `ccpm permissions ask "Edit(**/*.md)" --profile <p>`                                             | Add to `permissions.ask`          |
| `ccpm permissions deny "Bash(rm:*)" --profile <p>`                                               | Add to `permissions.deny`         |
| `ccpm permissions remove "<rule>" --profile <p>`                                                 | Strip from all three              |
| `ccpm permissions list --profile <p>`                                                            | Show all rules + the default mode |
| `ccpm permissions mode <default\|acceptEdits\|plan\|auto\|dontAsk\|bypassPermissions> --profile <p>` | Set `permissions.defaultMode`     |

### Trust

A new repo's `.claude/settings.json` (hooks, permissions, MCP) is ignored until you grant trust to that directory.

| Command                       | Description                                                  |
| ----------------------------- | ------------------------------------------------------------ |
| `ccpm trust add [path]`       | Grant trust (defaults to CWD). Alias: `grant`                |
| `ccpm trust remove [path]`    | Revoke trust. Aliases: `rm`, `forget`, `revoke`              |
| `ccpm trust list`             | List trusted directories. Alias: `ls`                        |

### Environment variables

`ccpm env` persists env vars on a profile; they are layered into the process env at every `ccpm run`, below the parent process env and `--ccpm-env`.

| Command                                                | Description                          |
| ------------------------------------------------------ | ------------------------------------ |
| `ccpm env set KEY=VALUE [KEY=VALUE...] --profile <p>`  | Persist env vars on the profile      |
| `ccpm env unset KEY [KEY...] --profile <p>`            | Remove env vars                      |
| `ccpm env list --profile <p>`                          | List persisted env vars              |
| `ccpm run <p> --ccpm-env KEY=VALUE` (repeatable)       | One-shot env override at launch time |

`CLAUDE_CONFIG_DIR` and `ANTHROPIC_API_KEY` are reserved: ccpm always computes them and they cannot be set via `ccpm env`. Use `--ccpm-env` for a one-shot override.

### Sessions

| Command                              | Description                                                                |
| ------------------------------------ | -------------------------------------------------------------------------- |
| `ccpm sessions list <profile>`       | Sessions scoped to the current working directory (matches `claude --resume`) |
| `ccpm sessions list <profile> --all` | Sessions from every project the profile has worked on                      |

### Import

| Command                                            | Description                                                                    |
| -------------------------------------------------- | ------------------------------------------------------------------------------ |
| `ccpm import default --profile <p>`                | Interactive: import skills/commands/hooks/agents/rules/settings/MCP/plugins    |
| `ccpm import default --all --only skills`          | Non-interactive: import specific targets into every profile                    |
| `ccpm import default --profile <p> --no-share`     | Copy directly instead of symlinking from the shared store                      |
| `ccpm import from-profile --src <a> --profile <b>` | Clone assets from one ccpm profile into another                                |

`--only` accepts a comma-separated list of: `skills`, `commands`, `rules`, `hooks`, `agents`, `settings`, `mcp`, `plugins`.

### Backup & migrate profiles

`ccpm export` bundles a profile's directory — skills, agents, commands, rules, hooks, MCP fragments, plugin metadata, and settings — into a single `.tar.gz` you can copy to another machine and restore with `ccpm import-bundle`. `ccpm clone` duplicates a profile in place.

| Command                                            | Description                                                                              |
| -------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `ccpm export <name> [-o file.tar.gz]`              | Export a profile to a portable bundle (credentials **excluded** by default)              |
| `ccpm export <name> --include-credentials`         | Include credential files (sensitive — trusted same-user moves only)                      |
| `ccpm import-bundle <file.tar.gz> [--profile <p>]` | Restore a profile from a bundle (path-traversal-safe; `--profile` overrides the name)    |
| `ccpm clone <src> <new>`                           | Duplicate a profile, copying assets, settings, and credentials                           |
| `ccpm clone <src> <new> --no-auth`                 | Clone assets/settings only; leave the new profile unauthenticated                        |

Credentials are NOT included in an export by default: OS-keychain tokens are machine-bound, and `.credentials.json` / `.claude.json` hold secrets you usually don't want in a shareable file. Use `--include-credentials` only for a trusted same-user move (e.g. a Linux machine migration). When a restored or cloned profile has no credentials, authenticate it afterward with `ccpm auth refresh <name>`.

> **OAuth clone caveat**: a cloned OAuth profile shares the source account's tokens, so when Claude rotates the refresh token in one, the other goes stale. For a clone you'll use long-term against the same account, prefer `--no-auth` and run `ccpm auth refresh <new>` to give it its own login. API-key clones have no such caveat.

### Shell completion

`ccpm completion <shell>` prints a completion script for `bash`, `zsh`, `fish`, or `powershell`, with completion for profile names on `run` / `use` / `remove` / `rename` / `set-default` / `clone` / `export`.

```bash
# zsh: load on every new shell
echo 'source <(ccpm completion zsh)' >> ~/.zshrc

# bash
echo 'source <(ccpm completion bash)' >> ~/.bashrc

# fish
ccpm completion fish > ~/.config/fish/completions/ccpm.fish
```

Run `ccpm completion <shell> --help` for the per-shell install details.

### Shell prompt

`ccpm prompt` prints the active profile name so you can show which Claude Code account a terminal is bound to. It resolves `$CCPM_ACTIVE_PROFILE` (set by `ccpm use`), then `$CLAUDE_CONFIG_DIR`, then — only with `--show-default` — the configured default. It prints nothing (exit 0) when no profile is active, so it stays quiet in non-ccpm shells.

```bash
# bash/zsh PS1
PS1='$(ccpm prompt --format "[ccpm:%s] ")'"$PS1"

# starship custom command
command = "ccpm prompt"
```

### Settings

ccpm does not maintain its own global settings layer. The cross-profile baseline is `~/.claude/settings.json` (the file native Claude Code reads); ccpm merges it into every profile at launch. Use `--profile` for per-account overrides, and per-repo `.claude/settings.json` for project overrides.

| Command                                            | Description                                                                                  |
| -------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `ccpm settings set <key> <value> --profile <p>`    | Set a setting for one profile (dot notation, JSON values)                                    |
| `ccpm settings get <key> --profile <p>`            | Read the effective value                                                                     |
| `ccpm settings apply <file.json> --profile <p>`    | Deep-merge a JSON fragment                                                                   |
| `ccpm settings show --profile <p>`                 | Dump the fully-merged settings                                                               |
| `ccpm settings statusline "<cmd>" --profile <p>`   | Set the native `statusLine` block. Empty `""` removes it.                                    |
| `ccpm settings outputstyle <style> --profile <p>`  | Set `outputStyle` (allowlist: default, Build, Explanatory, Learning, Direct)                 |

### Authentication

| Command                    | Description                                |
| -------------------------- | ------------------------------------------ |
| `ccpm auth status`         | Credential validity across profiles        |
| `ccpm auth refresh <name>` | Re-authenticate a profile                  |
| `ccpm auth backup <name>`  | Create an encrypted credential backup      |
| `ccpm auth restore <name>` | Restore credentials from backup            |

### Drift fingerprint

`ccpm import default` snapshots the files under `~/.claude` it touched. Later you can ask whether the host config has drifted:

| Command                              | Description                                                |
| ------------------------------------ | ---------------------------------------------------------- |
| `ccpm default fingerprint update`    | Record the current `~/.claude` state as the baseline       |
| `ccpm default fingerprint check`     | Diff `~/.claude` against the baseline; suggests `import`   |
| `ccpm config set check_default_drift true`  | Show drift notifications on `ccpm run` / `ccpm use` |

### Config

| Command                                            | Description                                  |
| -------------------------------------------------- | -------------------------------------------- |
| `ccpm config set cascade_auto_adopt false`         | Disable the host-asset cascade               |
| `ccpm config set check_default_drift true`        | Enable drift notifications on run/use        |
| `ccpm config get default_dir`                      | Print the default profile's absolute path    |

