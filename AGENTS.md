# AGENTS.md — ccpm context for coding tools

This file is the canonical briefing for any coding assistant (Cursor, Claude Code, Codex, Copilot, etc.) touching this repository. Read it once at the start of a session and again if your task changes scope. Nothing in this file is load-bearing for runtime — it exists purely to make agents less wrong.

---

## 1. What ccpm is (and what it is not)

`ccpm` (Claude Code Profile Manager) is a Go CLI that lets a single machine run **multiple, isolated Claude Code accounts in parallel**. Each profile is a self-contained `CLAUDE_CONFIG_DIR` with its own credentials, skills, MCP servers, hooks, agents, commands, rules, and settings.

It is **not**:

- a fork of Claude Code,
- an IDE extension,
- a telemetry / SaaS product,
- a replacement for `go-keyring`, Anthropic's auth flow, or the MCP protocol.

It is an orchestration layer that composes the official `CLAUDE_CONFIG_DIR` env var, the OS keychain, filesystem symlinks, and JSON merge logic to give the user per-profile isolation without patching Claude Code itself.

## 2. Core mental model

- **`CLAUDE_CONFIG_DIR`** — Claude Code honors this env var as its config root. ccpm points it at `~/.ccpm/profiles/<name>/` per profile. This is how isolation happens.
- **Shared store (`~/.ccpm/share/`)** — the "source of truth" for content that can be used by more than one profile: `skills/`, `mcp/`, `settings/`. Profiles symlink into the store for skills; MCP and settings live as JSON **fragments** that are merged into each profile's `settings.json` at launch.
- **Fragments**:
  - `share/mcp/` — `global.json` (applies to every profile) and `<profile>.json` (one profile).
  - `share/settings/` — **only** `<profile>.json` (per-profile). There is no ccpm-managed global settings fragment; the cross-profile baseline is the host `~/.claude/settings.json` file, merged in at `Materialize` time.
- **Manifest (`~/.ccpm/installs.json`)** — tracks every installed skill / MCP / setting, its scope (`global` or `profile`), and which profiles use it. Used by `ccpm sync`, `ccpm doctor`, and `ccpm skill/mcp list`.
- **Fingerprint (`~/.ccpm/default-claude-fingerprint.json`)** — SHA-256 hashes of files under `~/.claude` at the time of the last `ccpm import default`. Used for drift detection.

## 3. Profile lifecycle

1. `ccpm add <name>` creates `~/.ccpm/profiles/<name>/`.
2. If `~/.claude` exists or at least one other profile exists, the **import-source wizard** offers three options: start empty, import from `~/.claude`, or clone another profile.
3. Auth step: OAuth (browser login via `claude /login`) or API key (stored in the OS keychain via `go-keyring`).
4. `sync.ApplyGlobals` runs: shared skills are symlinked in, and `settingsmerge.Materialize` + `MaterializeMCP` write a launch-ready `settings.json` for the new profile.
5. `ccpm run <name>` execs `claude` with `CLAUDE_CONFIG_DIR` set. `ccpm use <name>` exports the env var into the current shell (requires the shell hook from `ccpm shell-init`).
6. `ccpm remove <name>` deletes the profile directory, removes the keystore entry, and (on macOS OAuth) removes the namespaced keychain entry.

## 4. Global vs per-profile assets

| Asset    | Cross-profile source                                | Profile path                         | Merge mechanism                               |
| -------- | --------------------------------------------------- | ------------------------------------ | --------------------------------------------- |
| Skills   | `~/.ccpm/share/skills/<name>`                       | `<profile>/skills/<name>` (link)     | Symlink                                       |
| MCP      | `~/.ccpm/share/mcp/global.json` + `<profile>.json`  | `<profile>/.claude.json#mcpServers`  | `settingsmerge.MaterializeMCP`                |
| Settings | `~/.claude/settings.json` + `share/settings/<profile>.json` | `<profile>/settings.json`    | `settingsmerge.Materialize` (with owned-keys) |
| Hooks    | (not shared) imported as copies                     | `<profile>/hooks/`                   | copy                                          |
| Agents   | `~/.ccpm/share/agents/<name>`                       | `<profile>/agents/<name>` (link)     | Symlink (dedup import)                        |
| Commands | `~/.ccpm/share/commands/<name>`                     | `<profile>/commands/<name>` (link)   | Symlink (dedup import)                        |
| Rules    | (not shared) imported as copies                     | `<profile>/rules/`                   | copy                                          |

## 5. Merge and precedence rules

`settingsmerge.Materialize(profileDir, profileName, projectRoot)` is the canonical implementation. Merge order (lowest → highest, higher wins):

1. Existing `<profileDir>/settings.json` — preserves keys Claude Code auto-wrote that nothing else redefines.
2. `~/.claude/settings.json` — the host/user file native Claude Code already uses. ccpm treats it as the shared baseline. **There is no ccpm-managed global settings fragment.**
3. `share/settings/<profileName>.json` — ccpm-managed per-profile fragment.
4. **Owned-keys override** — any leaf key path recorded in `share/settings/<profile>.owned.json` is re-applied from the profile fragment, so values set via `ccpm settings set --profile` survive Claude Code rewriting `settings.json`.
5. `<projectRoot>/.claude/settings.json` — per-repo override discovered by walking up from CWD at `ccpm run` time.
6. `<projectRoot>/.claude/settings.local.json` — gitignored per-machine override for the same project.

`projectRoot` is `""` from non-launch codepaths (add/use/sync/import) so they don't bake CWD-relative state into a profile.

MCP merge (`MaterializeMCP(profileDir, profileName, projectRoot)`): existing profile `.claude.json#mcpServers` → host `~/.claude.json#mcpServers` → `share/mcp/global.json` → `share/mcp/<profileName>.json` → project `.claude/settings.json#mcpServers` → project `.mcp.json`. Isolation invariant (never iterate `share/mcp/*`) is still enforced and tested in `internal/settingsmerge/merge_test.go`.

## 6. Authentication matrix

| Method  | Platform | Storage                                                                                                            |
| ------- | -------- | ------------------------------------------------------------------------------------------------------------------ |
| OAuth   | macOS    | Login Keychain under service `Claude Code-credentials-<sha256(abs(CLAUDE_CONFIG_DIR))[:8]>` (Claude Code v2.1.56+) |
| OAuth   | Linux    | `<profileDir>/.credentials.json`                                                                                   |
| OAuth   | Windows  | `<profileDir>/.credentials.json`                                                                                   |
| API key | All      | `go-keyring` — service `ccpm`, account `<profile>`                                                                 |
| Vault   | All      | AES-256-GCM under `~/.ccpm/vault/<profile>.enc`, master key in `go-keyring` (service `ccpm-vault`)                 |

Helpers live in `internal/credentials/`. `macos_keychain.go` (build-tagged `darwin`) computes the namespaced service name and reads/writes the Claude Code payload via `go-keyring`. A stub `macos_keychain_other.go` keeps non-darwin builds compiling.

## 7. MCP authentication model

Three categories — any new MCP-related feature must document which it targets.

1. **Env-var-based** — tokens live in the per-profile MCP fragment. Fully isolated.
2. **OAuth MCPs (cache in `.claude.json`)** — isolated automatically because `CLAUDE_CONFIG_DIR` is per-profile.
3. **Globally-cached MCPs (fixed-name `~/.config/<service>` or keychain entry)** — **shared** across profiles. Not fixable by ccpm. Documented in `README.md`.

## 8. Platform differences

- **Windows**: `syscall.Exec` is not available, so `claude.Exec` is split into `exec_unix.go` and `exec_windows.go`. The Windows path spawns a child and propagates the exit code. Symlinks require Developer Mode; `share.Link` falls back to a recursive copy and writes `~/.ccpm/.windows-copy-fallback` when it does. A one-time stderr warning is emitted.
- **macOS**: OAuth isolation requires Claude Code `v2.1.56+`. `ccpm doctor` warns on older versions. `ccpm set-default` copies the namespaced keychain entry into the "default" slot so IDE extensions pick up the right account.
- **Linux**: Requires D-Bus and a running secret service (gnome-keyring or kwallet) for API-key profiles.

## 9. Directory layout

```
~/.ccpm/
├── config.json                       # profile registry
├── installs.json                     # manifest
├── default-claude-fingerprint.json   # drift detection snapshot
├── .windows-copy-fallback            # present iff Windows couldn't symlink
├── share/
│   ├── skills/<name>/                # shared skill directory (source of truth)
│   ├── agents/<name>/                # shared agent directory
│   ├── commands/<name>/              # shared command directory
│   ├── mcp/{global,<profile>}.json   # MCP fragments
│   └── settings/<profile>.json       # per-profile settings fragments (+ <profile>.owned.json sidecars)
├── profiles/<name>/
│   ├── settings.json                 # materialized
│   ├── skills/<name>                 # symlink → share/skills/<name>
│   ├── .credentials.json             # OAuth (Linux/Windows)
│   └── .claude.json                  # Claude Code's own config
└── vault/<name>.enc                  # encrypted credential backup
```

## 10. Commands at a glance

| Command                                   | What it does                                                            | Side effects                                               |
| ----------------------------------------- | ----------------------------------------------------------------------- | ---------------------------------------------------------- |
| `ccpm add`                                | Create profile, optionally run import wizard, then auth                 | Writes profile dir, keychain entry, manifest               |
| `ccpm run`                                | Exec `claude` with `CLAUDE_CONFIG_DIR` set                              | Replaces current process on Unix                           |
| `ccpm use`                                | Print `export CLAUDE_CONFIG_DIR=...` for shell hook                     | Requires `ccpm shell-init` output in rc file               |
| `ccpm remove`                             | Delete profile dir, keychain entry, manifest references                 | Irreversible; vault backup is preserved                    |
| `ccpm list` / `ccpm status`               | Read-only inventory                                                     | None                                                       |
| `ccpm doctor`                             | Check env, auth, claude version, diff vs `~/.claude`, symlink integrity | None (warnings never fail)                                 |
| `ccpm import default`                     | Copy/link targets from `~/.claude`                                      | Writes to share and profile dirs                           |
| `ccpm import from-profile`                | Clone assets from one profile into another                              | Writes to target profile dir                               |
| `ccpm skill / mcp / settings`             | CRUD against the shared store + manifest                                | Fragment writes + owned-keys sidecar                       |
| `ccpm auth status/refresh/backup/restore` | Auth lifecycle                                                          | On macOS OAuth, reads/writes the namespaced keychain entry |
| `ccpm set-default`                        | Point IDEs at a profile                                                 | On macOS OAuth, copies keychain into default slot          |

## 11. Invariants contributors must preserve

1. **MCP isolation** — `MaterializeMCP` must only read `global.json` and `<profileName>.json`. Never iterate the whole directory. Regression test lives in `merge_test.go`.
2. **Windows build** — anything using `syscall.Exec`, `unix.*`, or POSIX signal files must be behind a `//go:build !windows` tag.
3. **macOS keychain access** — all `go-keyring` calls that target Claude Code's service name must go through `credentials.KeychainService(profileDir)` to avoid hard-coding the sha8 or using the wrong account.
4. **Owned-keys** — `ccpm settings set` and `ccpm settings apply` must call `settingsmerge.MarkOwned` / `MarkOwnedFromPatch`. Skipping this means user-set values get silently overwritten on `ccpm run`. Owned-keys live **per profile only** now; there is no global owned-keys sidecar.
5. **No ccpm-global settings layer** — the cross-profile settings baseline is `~/.claude/settings.json`, read directly by `settingsmerge.Materialize` via `loadHostClaudeSettings`. Do not reintroduce `share/settings/global.json`, a `--global` flag on `ccpm settings set/apply`, or any mechanism that makes ccpm the authoritative store for shared defaults. If you need to share a value across profiles, edit the host file or use `ccpm settings set --profile` on each profile.
6. **Dedup by default on import** — `ccpm import default` and `ccpm add`-with-wizard default to `Dedupe=true` for skills/agents/commands. `--no-share` is the opt-out.
7. **No network calls** — ccpm is local-only. Never add telemetry, update checks, or remote fetch.
8. **Failure modes never delete credentials** — `ccpm remove` is the only command allowed to delete a keychain entry.

## 12. Known limitations tracked upstream

- macOS OAuth isolation depends on Claude Code v2.1.56+ keychain namespacing. We cannot backfill older versions.
- Globally-cached MCP servers can't be isolated without upstream changes from each MCP.
- Windows without Developer Mode silently falls back to copies; there is no workaround without admin.

## 13. Agent responsibilities

When you make any substantive change to this repository (bug fix, feature, build/CI change, refactor with observable behavior, documentation that changes facts), you MUST append a new entry to `SUMMARY.md` in the format the file defines. Rules:

- Add the entry **in the same commit / PR** as the code change — never as a separate "log-only" commit.
- One entry per logically independent change. Do not batch unrelated fixes.
- Entries go at the top of the `## Log` section (reverse chronological).
- If a change is purely cosmetic (whitespace, typo, doc link rename), you may skip the log.
- Never rewrite or delete past entries. Correct a factual mistake with a new entry referencing the old one.

The entry template is defined in `SUMMARY.md` itself; follow it exactly.

Secondary agent hygiene:

- Always run `go build ./...` and `GOOS=windows go build ./...` from `ccpm/` before finishing a change that touches Go code. Windows cross-compile is a non-negotiable CI step.
- Always run `go test ./...` after changes to `internal/settingsmerge`, `internal/defaultclaude`, `internal/credentials`, `internal/share`, `internal/wizard`.
- Prefer extending tests in place over writing new throwaway smoke tests.
- Never update `go-keyring`, `cobra`, or other top-level deps without flagging it in the SUMMARY entry.
- Never publish a release manually. Use `scripts/release.sh <patch|minor|major|X.Y.Z>` — it enforces the preflight (auth, clean tree, sync with origin, unused tag) and sequences tag push → goreleaser wait → `npm publish` in the correct order. If you change release mechanics, update both the script and this file in the same PR.
