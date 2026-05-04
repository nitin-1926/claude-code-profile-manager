# Plan: Codex CLI support in ccpm

Status: **draft, awaiting decisions**
Owner: @nitin-1926
Last updated: 2026-05-02

## Goal

Let users create and run isolated profiles for OpenAI's `codex` CLI, in
parallel terminals, the same way they do for `claude` today. Reach feature
parity with claude profiles for the **launch + auth + MCP** path; explicitly
defer Claude-only concepts (skills, hooks, plugins, permissions, output
styles) to a later release.

Non-goal for this PR: Cursor, Gemini, opencode. Those are separate decisions
gated on whether Codex's abstraction holds up.

## Why a Tool interface (a small one)

Without an abstraction, every command grows
`if profile.tool == "claude" { ... } else { ... }` branches and the codebase
rots quickly. With a small interface — just enough to abstract launch + auth
+ paths — Codex slots in cleanly and we get a foundation for opencode later.
The full cross-tool asset abstraction (skills/hooks/MCP everywhere) is **not**
in scope here.

## Scope

### In v1

1. Per-profile `tool` field (`"claude" | "codex"`), default `claude` for
   existing profiles.
2. Minimal `internal/tool/` package + `Tool` interface with `claude` and
   `codex` implementations.
3. `ccpm add --tool codex <name>` — creates profile dir, runs Codex auth
   (browser OAuth or API key), writes `cli_auth_credentials_store = "file"`
   into the per-profile `config.toml` so OAuth tokens land in the
   per-profile `auth.json` (the aisw trick).
4. `ccpm run <name>` — dispatches to the right binary + env var
   (`CODEX_HOME` for codex, `CLAUDE_CONFIG_DIR` for claude) based on the
   profile's tool.
5. `ccpm list` / `ccpm status` — show tool column.
6. `ccpm doctor` — verify `codex` binary on PATH if any codex profile
   exists.
7. `ccpm import default --tool codex` — import existing `~/.codex/auth.json`
   + `config.toml` into a ccpm profile.
8. `ccpm mcp add/remove/list --scope profile` — extends to write
   `[mcp_servers.<name>]` blocks in the codex profile's `config.toml` for
   stdio MCPs. HTTP/SSE/OAuth-MCP not supported for codex in v1.
9. Live-import path: `ccpm add --tool codex --from-live <name>` reads
   `~/.codex/auth.json`, hashes the token for identity dedup (refuse to
   create a duplicate profile pointing at the same account).

### Deferred (claude-only in v1, error clearly on codex profiles)

- `ccpm hooks` — codex has no hook system
- `ccpm permissions` — different keyspace, revisit
- `ccpm plugin` — claude-only concept
- `ccpm skill / agent / command / rule` — claude-only
- `ccpm settings` — claude JSON-shaped, codex needs separate TOML
  surface; v2
- `ccpm sessions` — codex sessions doable but not free; v2
- HTTP/SSE/OAuth MCPs in codex profiles — v2

## Decisions to lock in (questions for owner)

1. **Profile namespace.** Single global namespace (names unique across
   tools) vs. per-tool namespace (profile = `(tool, name)`).
   Recommendation: **single namespace**. Matches today's mental model;
   avoids `--tool` flag pollution on every command. Cost: can't have
   `work` for both claude and codex.
2. **Codex auth methods.** Recommendation: **both** (ChatGPT OAuth + API
   key), parity with claude.
3. **Live import.** Recommendation: **support it**, with token-hash
   identity dedup so a user doesn't accidentally create two profiles for
   the same ChatGPT account.
4. **MCP scope for codex.** Recommendation: **stdio-only in v1**; document
   HTTP/SSE/OAuth as a v2 item. Closes the loop on "set up profile and
   actually use it" without a manual TOML edit step.
5. **`--profile` flag overlap.** Codex has its own `--profile <preset>`
   flag for config presets. ccpm forwards unknown flags after `<name>`
   transparently today. Recommendation: **forward as-is, document the
   gotcha** in README.
6. **`ccpm sync` semantics.** Recommendation: **iterate all profiles,
   skip non-applicable assets per-profile**. Simpler invariant.
7. **Default tool for `ccpm add` without `--tool`.** Recommendation:
   **claude** (backwards compat).

## Architecture

### `internal/tool/`

```go
type Tool interface {
    Name() string                          // "claude" | "codex"
    Binary() string                        // "claude" | "codex"
    ConfigDirEnv() string                  // "CLAUDE_CONFIG_DIR" | "CODEX_HOME"
    LiveHomeDir(userHome string) string    // ~/.claude | ~/.codex

    // Materialize merges shared baseline + profile fragment + project
    // overrides into the profile dir before launch. Same contract as
    // today's claude path; codex implementation writes config.toml.
    MaterializeProfile(profileDir, profileName string) error

    // CaptureLiveAuth is used by `ccpm add --from-live`. Reads the
    // tool's live home dir and stamps the captured creds + config into
    // the new profile dir. Returns an identity hash for dedup.
    CaptureLiveAuth(profileDir string) (identityHash string, err error)

    // RunAuthFlow is used by `ccpm add` (no --from-live). Drives the
    // tool's interactive auth into the profile dir.
    RunAuthFlow(profileDir string, method AuthMethod) error
}

func ForName(name string) (Tool, error)  // lookup
func All() []Tool                         // for doctor / sync
```

`internal/tool/claude/` wraps the existing logic with no behavior change.
`internal/tool/codex/` is the new implementation.

### Profile config change

`internal/config/config.go` profile struct gains a `Tool string` field. On
read, empty/missing defaults to `"claude"`. On write, always populated.

### Codex implementation notes

- **Config dir env var**: `CODEX_HOME` relocates everything (config.toml,
  auth.json, sessions, history). Verified via OpenAI Codex docs.
- **Forced file-backed auth**: write
  `cli_auth_credentials_store = "file"` into `<profile>/config.toml`.
  Without this, codex tries the OS keyring with an account name derived
  from a SHA-256 of the canonical CODEX_HOME — which makes per-profile
  isolation effectively impossible. Same workaround aisw uses, for the
  same reason.
- **Auth flow**: shell out to `codex login` with `CODEX_HOME` set to the
  profile dir. Codex's existing browser-based ChatGPT OAuth flow handles
  the rest. For API key, write `auth.json` directly (`{"OPENAI_API_KEY":
  "..."}`), matching the format codex emits.
- **MCP TOML writer**: codex's `[mcp_servers.<name>]` block has keys
  `command`, `args`, `env`, `cwd`. Use `github.com/pelletier/go-toml/v2`
  (already a transitive dep? confirm) for read/write. Stdio only.

## Implementation order (each step independently mergeable)

1. **Refactor**: introduce `internal/tool/` package, `Tool` interface,
   `claude` impl wrapping existing behavior. **No behavior change.**
2. **Config**: add `tool` field to profile config; default `claude` on
   read.
3. **Codex skeleton**: `codex` impl + `ccpm add --tool codex` + `ccpm
   run` dispatch. Browser OAuth only at this step.
4. **Codex API-key auth**: `ccpm add --tool codex --auth api-key`.
5. **Live import**: `ccpm add --tool codex --from-live` + identity dedup
   + `ccpm import default --tool codex`.
6. **Codex MCP**: extend `ccpm mcp add/remove/list` for codex profiles
   (stdio).
7. **Listing & doctor**: `ccpm list/status/doctor` show + verify codex.
8. **Gating**: claude-only commands print clear error on codex profiles.
9. **Docs**: README + AGENTS.md updates.

Steps 1–3 alone are a shippable preview release ("codex profiles, no MCP
yet, browser auth only").

## Risks & open questions

- **Codex schema churn**: `auth.json` format has shifted between codex
  releases. Pin a minimum codex version in `ccpm doctor` and keep the
  schema-shape-fallback pattern used today for plugin parsing.
- **Keyring trick portability**: forcing file-backed auth means we never
  use the OS keyring for codex. Document this in README — users who
  prefer keyring will be surprised. Worth it for the parallel-isolation
  guarantee.
- **`--profile` flag collision**: codex's own `--profile` is config-preset,
  ccpm's is profile-dir. We forward transparently; user must be aware.
- **Schema evolution monitoring**: subscribe to openai/codex releases for
  config breaking changes. Same hygiene we already need for claude.

## Out of scope here, listed as follow-ups

- `ccpm settings` for codex (TOML key surface)
- `ccpm sessions list` for codex
- HTTP/SSE/OAuth MCPs for codex
- Cursor, Gemini, opencode (separate research + separate plan docs)
- GUI app (separate decision; see conversation notes)
