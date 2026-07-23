# ccpm: model context for Ask Me

Use this block as the authoritative **user-facing** summary. Do not infer beyond it.

## What ccpm is

- **ccpm** (Claude Code Profile Manager) is a **local-only** Go CLI. It is not a fork of Claude Code, not an IDE extension, and does not collect telemetry or make network calls for its own operation.
- Isolation is via **`CLAUDE_CONFIG_DIR`**: each profile uses `~/.ccpm/profiles/<name>/` as the config root when you `ccpm run <name>` or `ccpm use <name>`.
- Shared content lives under **`~/.ccpm/share/`** (skills, MCP JSON fragments, settings fragments, etc.). Profiles typically **symlink** shared assets; MCP and settings are **merged** into launch-ready files.

## Running profiles in parallel (core use case)

- ccpm's headline feature is running **two or more Claude Code accounts at the same time**, fully isolated. Each profile uses its own `CLAUDE_CONFIG_DIR` (`~/.ccpm/profiles/<name>/`), so simultaneous sessions never share credentials, settings, MCP servers, or memory.
- To run two profiles **simultaneously**, open two terminals and run `ccpm run <profileA>` in one and `ccpm run <profileB>` in the other. They run side by side — no logging out, no swapping config files.
- Works the same for **OAuth** and **API key** profiles. Each OAuth profile authenticates independently via `claude /login` in its own profile context (macOS keychain isolation needs Claude Code v2.1.56+).
- `ccpm run <name>` needs no shell setup and is the simplest way to launch a specific profile. `ccpm use <name>` instead sets the active profile for a whole shell session (requires the shell hook from `ccpm shell-init`).

## Install

- **npm**: `npm i -g @ngcodes/ccpm`
- **curl**: `curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh`
- **go**: `go install github.com/nitin-1926/claude-code-profile-manager/ccpm@latest`

## Real top-level commands

These are the exact commands the CLI registers. Do not invent flags or commands beyond this list.

- **Profile lifecycle**: `ccpm add`, `ccpm remove` (alias `rm`), `ccpm rename`, `ccpm clone`, `ccpm list` (alias `ls`, `--json` for machine-readable output), `ccpm status` (`--json`), `ccpm use`, `ccpm run`, `ccpm sync` (`--dry-run` previews what would link/adopt).
- **Defaults / IDE**: `ccpm set-default`, `ccpm unset-default`.
- **Shell prompt**: `ccpm prompt` (prints the active profile name for PS1/starship/p10k; supports `--format` and `--show-default`).
- **Backup / migrate**: `ccpm export <profile>` (writes a `.tar.gz`; `-o`/`--output`, `--include-credentials`) and `ccpm import-bundle <file.tar.gz>` (`--profile` to rename the restored profile).
- **Health / drift**: `ccpm doctor` (`--fix` prunes dangling shared-asset symlinks), `ccpm consolidate`, `ccpm default fingerprint update`, `ccpm default fingerprint check`.
- **Exit codes (for scripting)**: 0 = success (stderr warnings never change the exit code), 1 = command failed, 3 = partial failure (`ccpm import` or `ccpm sync` did some work but at least one profile/step failed), 4 = `ccpm doctor` found health issues. (Code 2 is reserved for usage errors but not yet wired — flag/arg errors currently exit 1.)
- **Auth**: `ccpm auth status`, `ccpm auth refresh`, `ccpm auth backup`, `ccpm auth restore`.
- **Asset trees** (each has `add`, `remove`/`rm`, `list`/`ls`, `link`): `ccpm skill`, `ccpm agent`, `ccpm command`, `ccpm rule`.
- **MCP**: `ccpm mcp add|remove|list|import|auth` (`list --json` supported) with `--scope global|profile|project`, transports `stdio|http|sse`.
- **Plugins**: `ccpm plugin list|enable|disable|install|remove|gc` and `ccpm plugin marketplace add|remove|list`.
- **Hooks**: `ccpm hooks add|remove|list`.
- **Permissions**: `ccpm permissions allow|ask|deny|remove|list|mode`.
- **Env**: `ccpm env set|unset|list`.
- **Settings**: `ccpm settings set|get|apply|show|statusline|outputstyle`.
- **Sessions**: `ccpm sessions list <profile>` (with `--all`, `--limit N` — default 20 most recent, `--json`).
- **Usage**: `ccpm usage [profile]` reports token usage from the profile's Claude Code transcripts. On a terminal it opens an **interactive dashboard** (tabs Overview/Days/Models/Projects/Sessions, switch profile with `[`/`]`, cycle the time window with `w`, scroll with arrows, `q` to quit). For static/scriptable output use `--plain`, `--json`, or any of `--by-model`/`--by-project`/`--sessions`/`--all`/`--since <dur|date>` (these always print non-interactively). Default profile resolution matches `ccpm prompt`/`statusline` (active session, then default).
- **Import**: `ccpm import default` (with `--only skills,commands,rules,hooks,agents,settings,mcp,plugins`) and `ccpm import from-profile`.
- **Trust**: `ccpm trust add|remove|list` (manages which project dirs are allowed to contribute `.claude/settings.json` hooks/permissions/MCP).
- **Config**: `ccpm config set <key> <value>` and `ccpm config get <key>`. Keys: `cascade_auto_adopt` (bool), `check_default_drift` (bool), `statusline` (bool — auto-inject a default statusLine on `ccpm run`; default true), `usage_tracking` (bool — inject a SessionEnd hook that keeps the usage store warm; default false/opt-in, `ccpm usage` works without it), `default_dir` (read-only).
- **Shell**: `ccpm shell-init` (zsh, bash, fish, powershell) prints the `ccpm use` shell hook; `ccpm completion bash|zsh|fish|powershell` prints a Cobra completion script (with profile-name completion for run/use/remove/rename/set-default/clone/export).
- **Diff**: `ccpm diff <profile-a> <profile-b>` compares profile-scoped assets, settings-fragment keys, env var names (values never shown), MCP servers, and installed plugins.
- **Global flags**: `--log-level debug|info|warn|error` (default warn) on every command; `debug` traces cascade and lock activity.
- **Lifecycle**: `ccpm uninstall`.
- **Version**: `ccpm --version` (root flag) or `ccpm version`; `ccpm version --check-latest` makes an explicit opt-in request to the GitHub releases API (cached 24h) to report whether a newer release exists — this is the only network call in the CLI and never happens without the flag.

There is **no** `ccpm copy`, `ccpm drift`, `ccpm vault`, or `ccpm update` subcommand. Profile duplication is `ccpm clone`; portable export/restore is `ccpm export` / `ccpm import-bundle`. Vault backup/restore (encrypted credential backup) is exposed via `ccpm auth backup|restore`. Drift detection is `ccpm default fingerprint`.

## Run flags intercepted by ccpm

`ccpm run <profile> [args...]` forwards unknown flags to claude without a `--` separator. Five flags are intercepted before they reach claude: `--ccpm-env KEY=VALUE` (repeatable, one-shot env override), `--no-auto-adopt` (skip the host-asset cascade scan), `--no-statusline` (skip injecting the default statusLine for this launch), `--help`, `--version`. To forward `--help` or `--version` to claude, use `ccpm run <profile> -- --help`.

## Default profile (IDE / GUI)

`ccpm set-default <name>` pins a profile as the machine-wide default for **every** `claude` launch — the integrated terminal, IDE extensions (Cursor, VSCode, Antigravity), and any GUI app that spawns `claude` — not just shell-bypassing launches. It is the way to set a default for tools that don't go through the `ccpm use` shell hook. Call it without a name in a TTY for a profile picker.

For an **OAuth** profile on macOS it does three things: copies the profile's namespaced keychain entry into the default slot (folding the previous default's slot back into its own profile first, but only when those tokens are fresher, so a recent `ccpm run <p> claude /login` is never clobbered), syncs identity fields (`oauthAccount`, `userID`) into `~/.claude.json` so the welcome banner shows the right account, and registers a machine-wide `CLAUDE_CONFIG_DIR` via `launchctl setenv` plus a user-level LaunchAgent (`com.ccpm.default-config-dir`) that re-applies it at every login. The `launchctl setenv` part is what extends the default to terminals and GUI apps, and also works around a Claude Code v2.1.x startup-refresh bug that 401s when `CLAUDE_CONFIG_DIR` resolves to bare `~/.claude`.

For an **API-key** profile, `set-default` instead clears any machine-wide `CLAUDE_CONFIG_DIR` pin, removes the default-slot OAuth entry, and writes `ANTHROPIC_API_KEY` into `~/.claude/settings.json`'s `env` block (so terminal and agent invocations use that key). The VSCode/Antigravity sidebar cannot show API-key logins as "signed in," but `claude` invocations do use the key.

Already-running IDE windows need one restart to pick up the new env; future launches and future logins are automatic. `ccpm unset-default` removes the LaunchAgent, runs `launchctl unsetenv`, strips the `ANTHROPIC_API_KEY` env block, and clears the default pointer. On Linux and Windows the keychain/credentials and identity sync still apply; the `launchctl` machine-wide mechanism is macOS-only for now (terminal use on those platforms is covered by the `ccpm use` shell hook).

## Authentication

- **OAuth**: browser flow via `claude /login` in profile context. macOS uses Keychain under a **namespaced** service derived from `CLAUDE_CONFIG_DIR` (Claude Code **v2.1.56+** required).
- **API key**: stored via OS keychain (`go-keyring`), service `ccpm`, account `<profile>`.
- **Vault**: encrypted backup under `~/.ccpm/vault/<profile>.enc` (AES-256-GCM, master key in OS keychain). Created with `ccpm auth backup <name>`, restored with `ccpm auth restore <name>`.

## Backup, clone, and migrate profiles

- **`ccpm clone <source> <new>`** duplicates a profile in place (skills, agents, commands, rules, hooks, MCP fragments, plugins, settings, and — unless `--no-auth` — credentials). For an OAuth clone the new profile **shares the source account's tokens**, so refresh-token rotation in one staleness the other; for a long-term clone against the same account prefer `--no-auth` and run `ccpm auth refresh <new>` to give it its own login. API-key clones have no such caveat.
- **`ccpm export <profile>`** bundles the profile directory into a portable `.tar.gz` (`-o`/`--output` to choose the path, default `<profile>.ccpm.tar.gz`). **Credentials are excluded by default** because keychain tokens are machine-bound and `.credentials.json` / `.claude.json` hold secrets; use `--include-credentials` only for a trusted same-user move and treat the bundle as sensitive.
- **`ccpm import-bundle <file.tar.gz>`** restores a profile from an export bundle (path-traversal-safe). The restored profile takes the bundle's original name unless `--profile <name>` overrides it. If the bundle had no credentials (the default), authenticate afterward with `ccpm auth refresh <name>`.

## Shell prompt integration

`ccpm prompt` prints the active profile name so a shell prompt (PS1, starship, powerlevel10k) can show which Claude Code account a terminal is bound to. Resolution order: `$CCPM_ACTIVE_PROFILE` (set by `ccpm use`) → `$CLAUDE_CONFIG_DIR` (matched back to a known profile dir) → the configured default, **only** with `--show-default`. It prints nothing (exit 0) when no profile is active. `--format '%s'` wraps the name (must contain a single `%s`).

`ccpm statusline` is the in-TUI status line for a launched session — it shows **which profile is running** plus usage/limits at the bottom of the Claude Code window. It reads Claude Code's status JSON on stdin and renders a line like `⬢ work · Sonnet 4.6 · ctx 34% · 5h 42% ↺16:15 · 7d 12% · $1.23`, where the `5h`/`7d` segments are the **percentage used** of the rolling subscription usage windows — matching Claude's own `/usage` panel (Pro/Max accounts only; they appear after the first API response and are absent for API-key profiles, which collapse to `⬢ work · Opus 4.8 · $0.12`). The line is **ANSI-coloured** by default — the `5h`/`7d` percentage shades green→amber→red as remaining headroom shrinks; set `NO_COLOR` for plain text. The `$…` segment is the API-equivalent cost estimate of the session, not a subscription charge. `ccpm run` injects this automatically as the profile's `statusLine` when the profile has none of its own — it never overwrites a statusLine you set in `~/.claude/settings.json`, a profile, or a trusted project. Turn the auto-injection off persistently with `ccpm config set statusline false`, per-launch with `ccpm run <profile> --no-statusline`, and remove an already-injected one with `ccpm settings statusline "" --profile <name>`. You don't normally call `ccpm statusline` yourself; Claude Code invokes it.

## Usage tracking (`ccpm usage`)

`ccpm usage [profile]` reports per-profile token usage by reading the Claude Code session transcripts a profile accumulates under `<profileDir>/projects/**/*.jsonl`. It shows raw token totals (input, output, cache-write, cache-read) — **no dollar cost is computed** (transcripts carry no price, and a subscription's real cost is unrelated to API-equivalent pricing). On a TTY it opens an interactive dashboard (tabbed Overview/Days/Models/Projects/Sessions, `[`/`]` switch profile, `w` cycles the time window, arrows scroll, `q` quits); `--plain` prints the static report instead. The static view prints totals, a GitHub-style **contribution heatmap in amber** (intensity by tokens/day; set `NO_COLOR` for plain glyphs), and a per-model breakdown; `--by-project`, `--by-model`, and `--sessions` swap the body, `--since <dur|date>` windows it, `--all` aggregates every profile, and `--json` emits a stable snake_case contract. Counts are deduplicated by `(message.id + requestId)` with the **largest token snapshot winning** — Claude Code writes each response as several transcript lines sharing one `message.id`, sometimes with growing usage snapshots, so a naive sum over-counts ~3× and a first-seen dedup can under-count. `ccpm usage` reports token counts only (no dollar cost); the desktop app's Usage tab additionally shows an **API-equivalent cost estimate** and a live **5-hour block** monitor (see below). Data is maintained incrementally in a ccpm-owned `<profileDir>/usage/` store (ingest cursor + session index + daily index); each run reads only new transcript bytes since the last. By default the store refreshes lazily whenever you run `ccpm usage`; enable `ccpm config set usage_tracking true` to also inject a `SessionEnd` hook (`ccpm usage sync`) that keeps it warm after every session (opt-in because it adds a hook + per-session process; the lazy refresh already keeps `ccpm usage` accurate without it).

## MCP

- **Isolation invariant**: merge reads only `share/mcp/global.json` and `share/mcp/<profile>.json`. It never iterates arbitrary files in `share/mcp/`.
- Materialized MCP lives in **`<profile>/.claude.json#mcpServers`**, not `settings.json`.
- **Scopes**: global, profile, project (`--scope` or legacy `--global` / `--profile`). Transports: stdio, http, sse. `ccpm mcp auth <name> --profile <p>` runs native Claude scoped to the profile to complete OAuth.

## Settings merge (low to high, higher wins)

1. Existing profile `settings.json`.
2. Host `~/.claude/settings.json`.
3. ccpm profile fragment `share/settings/<profile>.json`.
4. **Owned-keys** sidecar (values explicitly set via `ccpm settings set --profile`).
5. Project `./.claude/settings.json`.
6. Project `./.claude/settings.local.json`.
7. Enterprise managed-settings (highest).

There is **no** ccpm-managed global settings fragment; the shared baseline is the host `~/.claude/settings.json` directly.

## Host asset cascade

Optional auto-adopt from `~/.claude/{skills,agents,commands,rules,hooks,plugins}` into every profile (`scope=host` in the manifest). **Profile-local always wins** over host. Opt out: `ccpm config set cascade_auto_adopt false` or `--no-auto-adopt` on `run` / `sync`.

## Desktop app (CCPM Desktop)

An optional native desktop GUI for managing profiles, alongside the CLI (the CLI remains the primary interface). Built with **Wails** (Go backend + native OS webview) in the same Go module, so it reuses ccpm's own engine for reads and shells out to the `ccpm` CLI for writes (same lock/keychain/atomic/validation — the GUI can't corrupt state the CLI wouldn't). Local-first, no signup.

- **What it does**: left sidebar of profiles + a tabbed view per profile — **Overview** (asset counts, details), **Cascade** (the effective host→global→profile config with per-asset/per-setting provenance badges and override/shadow hints), **Assets** (add/remove skills/agents/commands/rules/hooks), **MCP & Plugins** (list + add/remove MCP, install/remove + enable/disable plugins), **Permissions** (allow/ask/deny rules, default mode, env vars), **Settings** (edit effective settings as JSON via `ccpm settings set`), **Usage** (native amber heatmap + token totals + by-model/project + sessions, PLUS an **API-equivalent dollar-cost estimate** and a live **"current 5-hour block"** card with burn rate, time-left, and projected cost — inspired by ccusage; cost is estimated from public API list prices, not subscription billing), and **Health** (`ccpm doctor` output). Profile actions: Run, Clone, Rename, Delete, Open folder. It auto-refreshes when the CLI or Claude Code change files underneath it.
- **First run**: detects existing profiles and opens to the list; with zero profiles it shows a one-screen explainer plus Create / Import buttons.
- **Auth**: in-GUI OAuth is not implemented yet — creating a profile or importing `~/.claude` opens a Terminal running the proven `ccpm add` wizard to complete sign-in.
- **Download**: prebuilt `.dmg`s are published on GitHub Releases under the desktop-scoped tag list `https://github.com/nitin-1926/claude-code-profile-manager/releases?q=desktop-v&expanded=true` (do NOT use `/releases/latest` — that resolves to the CLI release, which has no `.dmg`) — a separate ~3–4 MB build per chip: **Apple Silicon** (`CCPM-<version>-arm64.dmg`) and **Intel** (`CCPM-<version>-amd64.dmg`). Open the `.dmg` for your Mac and drag **CCPM** into **Applications**. Because the app is **not notarized** yet, macOS Gatekeeper blocks the first launch — right-click the app → **Open** (or System Settings → Privacy & Security → "Open Anyway") once. The app uses the `ccpm` CLI for write actions, so the CLI must also be installed (it's resolved from standard locations like Homebrew/`/usr/local/bin`/npm-global even though a GUI app doesn't inherit your shell PATH).
- **Auto-updates**: the app checks GitHub Releases on launch and, when a newer build exists, shows a bottom-right "Update available" prompt with an **Update now** button. It downloads the new build itself, verifies its SHA-256, and **swaps the app in place, then relaunches — no re-dragging to Applications and no repeat of the Gatekeeper prompt** (self-downloaded builds aren't quarantined). The one-time right-click→Open only applies to the very first install. Updates are integrity-checked over HTTPS (Apple notarization / signed updates are a later hardening step).
- **Versioning & release**: the desktop app versions **independently of the CLI** — it ships on `desktop-v*` git tags (the CLI uses `v*`), built by a dedicated `Desktop Release` GitHub Actions workflow that publishes per-arch `.dmg` + `.app.zip` + `checksums.txt`. The running version is baked into the binary at build time.
- **Build/run**: requires the Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`), then `make desktop` (or `cd ccpm/desktop && wails build`) produces `desktop/build/bin/CCPM.app`; `make desktop-dev` runs a hot-reload window. The app is **ad-hoc signed / unsigned** for now, so first launch on macOS needs a one-time Gatekeeper bypass (right-click → Open).
- **Status**: the desktop app is newer/secondary to the CLI; if unsure about a desktop-only detail, prefer pointing users at the CLI equivalent.

## Platforms

- **macOS**: verified path for OAuth isolation and keychain. The desktop app targets macOS first (the Run/create/import Terminal bridges are macOS-only; other OSes get a copyable command).
- **Linux / Windows**: ship and cross-compile; some OAuth / keychain paths are **experimental** until verified on real hosts. Windows may fall back to **copy** instead of symlink without Developer Mode (`~/.ccpm/.windows-copy-fallback` marker).

## Privacy

No telemetry; no remote calls from the CLI itself. Secrets are not logged in plaintext.

## Ask Me guardrails

- The docs site's Ask Me endpoint accepts only same-origin JSON POST bodies (verified via `Origin` or same-host `Referer`) with a string `question`.
- Questions are trimmed, length-capped, stripped of control characters and zero-width / BOM characters, and rejected when they contain obvious prompt-injection framing, HTML-comment framing, script-like markup, JavaScript URLs, or requests to reveal hidden/system prompt content.
- Requests are rate-limited per detected client IP. The Portkey API key and virtual key stay server-side; upstream errors are never echoed back to the client verbatim.
- Backend flow: the system prompt lives in code (not a Portkey saved prompt). The server sends a **system + user** message (this markdown as the trusted context, plus the user's `question`) to Portkey **`chat/completions`** routed by **`PORTKEY_VIRTUAL_KEY`** (the Gemini integration), and streams the reply token by token. The free **`gemini-2.5-flash-lite`** model is used by default (override via `PORTKEY_MODEL`).
- Answers are grounded by this checked-in context (capped at 64 KB). The client renders returned markdown through a sanitizer before display; HTML, scripts, styles, and inline event attributes are stripped.

## Troubleshooting pointers

- **MCP tools missing**: confirm servers appear under `<profile>/.claude.json#mcpServers` after `ccpm use` / `ccpm run`.
- **IDE shows wrong account after `set-default`** (OAuth): re-run `set-default` on a current binary and confirm `~/.claude.json` identity fields match the profile.
- **First-line health check**: `ccpm doctor` (auth, drift, symlinks, cascade). `ccpm doctor --fix` prunes dangling shared-asset symlinks.
- **Broken symlinks**: `ccpm doctor` detects them; `ccpm doctor --fix` prunes dangling shared-asset symlinks (deeper drift: `ccpm consolidate --fix`).
- **IDE still on the wrong account after `set-default`**: already-running IDE windows need one restart to pick up the new `CLAUDE_CONFIG_DIR`; future launches are automatic.
- **Running sessions don't see a change**: `set-default`, `rename`, and `remove` do not affect already-running Claude Code sessions — restart the session (and IDE window).
- **Claude Code too old (macOS OAuth)**: needs v2.1.56+ for per-profile keychain isolation; `ccpm doctor` reports the installed version.
- **Project settings ignored**: a new repo's `.claude/settings.json` is ignored until you run `ccpm trust add` in that directory.

## When you do not know

If the user asks something not covered here, say you can only answer from this documentation and suggest `ccpm --help`, the GitHub repo, or the published docs site.
