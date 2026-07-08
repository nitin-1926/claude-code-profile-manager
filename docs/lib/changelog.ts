/**
 * Curated public changelog for the docs site and README excerpts.
 * Source of truth for narrative is git history + maintainer notes; keep entries user-facing.
 *
 * Shape is two-level:
 *   Series (e.g. 0.4.x) -> Releases (themed change sets / published patches) -> bullets.
 * The page renders one main card per series, releases nested inside, bullets as a 3rd level.
 *
 * Docs-site-only changes do not live here; the changelog narrates ccpm package growth.
 */

export type ChangelogCategory = "Added" | "Improved" | "Fixed" | "Security";

export type ChangelogRelease = {
  /** Published patch version like "0.4.12", or undefined for an in-series themed change set. */
  version?: string;
  /** ISO date YYYY-MM-DD */
  date: string;
  title: string;
  categories: ChangelogCategory[];
  bullets: string[];
};

export type ChangelogSeries = {
  /** "0.4.x", "0.3.x", "0.2.x". One main card / chapter per series. */
  series: string;
  /** Short narrative line shown under the series header. */
  summary: string;
  /** Themed change sets / released patches, newest first. */
  releases: ChangelogRelease[];
};

export const CHANGELOG: ChangelogSeries[] = [
  {
    series: "0.5.x",
    summary:
      "Profile backup & cloning, shell completions, prompt and status-line integration, and concurrency-safe credential handling.",
    releases: [
      {
        date: "2026-07-08",
        title: "CCPM Desktop: download for macOS",
        categories: ["Added"],
        bullets: [
          "The optional desktop app is now a downloadable universal .dmg (Apple Silicon + Intel) on GitHub Releases — open it and drag CCPM into Applications. No more building from source to try the GUI.",
          "First launch needs a one-time Gatekeeper bypass (right-click → Open) because the app isn't notarized yet. The app uses the ccpm CLI for write actions, so keep the CLI installed.",
        ],
      },
      {
        date: "2026-07-01",
        title: "Usage: more accurate counts, cost estimates, and 5-hour blocks",
        categories: ["Improved", "Added", "Fixed"],
        bullets: [
          "Fixed a token undercount: usage is now deduplicated by (message id + request id) with the largest usage snapshot winning, matching how Claude Code appends growing snapshots for one response. This corrects both cross-request collisions and a first-seen under-count. Existing usage stores re-ingest once to adopt the corrected numbers.",
          "The desktop app's Usage tab now shows an API-equivalent dollar-cost estimate per total, model, and project (estimated from public list prices — not subscription billing).",
          "New live \"current 5-hour block\" card in the desktop Usage tab: cost so far, burn rate ($/hr), time left in the window, and projected end-of-block cost — inspired by ccusage.",
          "The desktop app also gained a Settings editor tab and plugin install/remove.",
        ],
      },
      {
        date: "2026-06-30",
        title: "CCPM Desktop — an optional native app for managing profiles",
        categories: ["Added"],
        bullets: [
          "A new desktop GUI (in `ccpm/desktop/`, built with Wails) sits alongside the CLI: a sidebar of profiles plus per-profile tabs for Overview, Cascade, Assets, MCP & Plugins, Permissions, Usage, and Health. It reuses ccpm's own engine for reads and shells out to the CLI for writes, so it stays exactly in sync with what `ccpm` does.",
          "The Cascade tab shows the effective host→global→profile config with provenance badges on every asset and setting, and flags overrides/shadowing — so you can finally see what actually resolves, and why.",
          "Clone, rename, delete, open, and run profiles from the toolbar; add/remove assets and MCP servers, toggle plugins, and edit permissions/env without touching JSON. The view auto-refreshes when the CLI changes things underneath it.",
          "Build it with `make desktop` (needs the Wails CLI). It's local-first, no signup, and unsigned for now (one-time Gatekeeper bypass on macOS). Creating a profile or importing `~/.claude` opens a Terminal to complete sign-in.",
        ],
      },
      {
        date: "2026-06-30",
        title: "`ccpm usage` heatmap is now amber",
        categories: ["Improved"],
        bullets: [
          "The contribution heatmap switched from purple to amber so the CLI matches the new CCPM Desktop theme. `NO_COLOR` still renders plain glyphs.",
        ],
      },
      {
        date: "2026-06-29",
        title: "`ccpm usage` — per-profile token usage with a contribution heatmap",
        categories: ["Added"],
        bullets: [
          "New `ccpm usage [profile]` reports token usage (input, output, cache-write, cache-read) read straight from a profile's Claude Code session transcripts — fully retroactive over your existing history, with no dollar cost computed.",
          "On a terminal it opens an interactive dashboard — tabbed Overview/Days/Models/Projects/Sessions, switch profile with `[`/`]`, cycle the time window with `w`, scroll with arrows. The Overview shows totals and a GitHub-style contribution heatmap rendered in purple. Use `--plain`, `--json`, or `--by-model`/`--by-project`/`--sessions`/`--all`/`--since` for static, scriptable output.",
          "Counts are correct by construction: Claude Code writes each response as several transcript lines sharing one message id, so totals are deduplicated by message id (a naive sum over-counts ~3x).",
          "Data is maintained incrementally in a local per-profile store (`<profileDir>/usage/`) — each run reads only new transcript bytes. Opt in to `ccpm config set usage_tracking true` to keep it warm via a SessionEnd hook; `ccpm usage` works without it.",
        ],
      },
      {
        date: "2026-06-25",
        title: "Status line now matches /usage and is colour-coded",
        categories: ["Improved"],
        bullets: [
          "The `5h`/`7d` usage segments now show the percentage **used**, matching Claude's own `/usage` panel, so the numbers line up instead of reading as the inverse.",
          "The line is now colour-coded: the usage percentage shades green → amber → red as your remaining headroom shrinks, with orange window labels and a yellow reset clock. Set `NO_COLOR` for plain text.",
        ],
      },
      {
        date: "2026-06-25",
        title: "In-TUI status line: see which profile is running, plus usage and limits",
        categories: ["Added"],
        bullets: [
          "`ccpm run` now shows which profile a session is using right inside the Claude Code window — a status line pinned to the bottom that reads e.g. `⬢ work · Sonnet 4.6 · ctx 34% · 5h 58% ↺16:15 · 7d 88% · $1.23`.",
          "For Claude Pro/Max accounts it surfaces how much of your rolling **5-hour and 7-day usage windows** is left (remaining %, with the reset time), plus current context fill and session cost. API-key profiles show profile, model, and cost.",
          "It's wired in automatically when a profile has no status line of its own, and never overwrites one you set in `~/.claude/settings.json`, a profile, or a trusted project. Opt out with `ccpm config set statusline false`, per-launch with `ccpm run <profile> --no-statusline`, or remove an injected one with `ccpm settings statusline \"\" --profile <name>`.",
        ],
      },
      {
        date: "2026-06-03",
        version: "0.5.0",
        title: "Profile backup, cloning, shell completions, and prompt integration",
        categories: ["Added"],
        bullets: [
          "`ccpm export <profile>` packages a profile's skills, agents, commands, rules, hooks, MCP servers, plugins, and settings into a portable `.tar.gz`. `ccpm import-bundle <file>` restores it on another machine (path-traversal-safe). Credentials are excluded by default — re-authenticate with `ccpm auth refresh` after restoring, or pass `--include-credentials` for a trusted same-user move.",
          "`ccpm clone <source> <new-name>` duplicates an existing profile (assets, settings, and auth). Pass `--no-auth` for an assets-only copy. Note: OAuth clones share the source account's tokens, so for a long-lived clone prefer `--no-auth` and a fresh login.",
          "`ccpm completion bash|zsh|fish|powershell` emits shell completion scripts, including live completion of your profile names for `run`, `use`, `remove`, `rename`, `set-default`, `clone`, and `export`.",
          "`ccpm prompt` prints the active profile name for embedding in your shell prompt (PS1, starship, powerlevel10k), so a terminal always shows which account it's bound to. Supports `--format` and `--show-default`.",
          "`ccpm list --json` emits machine-readable JSON for scripting, CI, and status lines.",
          "`ccpm doctor --fix` prunes dangling shared-asset symlinks (doctor was previously read-only).",
          "Distribution: the macOS/Linux/Windows builds now ship with cosign-signed checksums.",
        ],
      },
      {
        date: "2026-06-03",
        title: "Concurrency-safe credential handling and hardening",
        categories: ["Fixed", "Security"],
        bullets: [
          "Credential- and config-mutating commands (`set-default`, `rename`, `remove`, `add`, `clone`) now take a global advisory lock, so running two ccpm commands at once can no longer interleave a stale read with a fresh write and clobber a valid refresh token (a cause of spurious `401` re-logins).",
          "`config.json`, the shared-asset manifest, and the trust list are now written through the crash-safe atomic-write path, eliminating a window where a crash or concurrent write could corrupt them.",
          "Profile/vault directories and ccpm-created `~/.claude` directories are now created `0700` (owner-only), so credential files aren't enumerable by other local users on a shared host.",
          "`ccpm rename` reordered its keychain migration and plugin-metadata rewrite so a mid-rename failure can't leave plugin metadata pointing at a directory that was rolled back.",
          "Fixed a crash where a malformed or truncated stored API key could panic `ccpm status` / `list` / `auth status`; keys are now masked safely and validated on entry.",
          "The npm installer now verifies the downloaded binary's SHA-256 against the release checksums and pins the download to the installed package version (no more silently fetching `latest`).",
        ],
      },
    ],
  },
  {
    series: "0.4.x",
    summary:
      "Default-profile UX across IDEs, OAuth correctness on macOS, host-asset cascade, and atomic writes.",
    releases: [
      {
        date: "2026-05-12",
        title:
          "`ccpm set-default` now applies to Cursor, VSCode, and Antigravity (macOS)",
        categories: ["Improved", "Fixed"],
        bullets: [
          "`ccpm set-default <profile>` now registers the chosen profile system-wide on macOS via a user-level LaunchAgent and `launchctl setenv CLAUDE_CONFIG_DIR`. Every newly-launched claude process (terminal, Cursor, VSCode, Antigravity, any GUI app) inherits the profile automatically. Restart any open IDE windows once to pick up the change; future launches and future logins are automatic.",
          "`ccpm unset-default` removes the LaunchAgent and unsets the env.",
          "Works around a Claude Code v2.1.x bug where the startup OAuth refresh path misfires when CLAUDE_CONFIG_DIR resolves to bare `~/.claude` even with valid tokens in the keychain.",
          "Linux and Windows: keychain and identity sync work as before; the GUI-wide env mechanism is macOS-only for now. Terminal use on those platforms is already covered by `ccpm shell-init`.",
        ],
      },
      {
        date: "2026-05-12",
        title: "`ccpm shell-init` now wraps `claude` for terminal use",
        categories: ["Improved", "Fixed"],
        bullets: [
          "`ccpm shell-init` now emits a `claude()` shell function alongside `ccpm()`. When `CLAUDE_CONFIG_DIR` is unset, plain `claude` invocations from your terminal transparently route through whichever profile `ccpm set-default` selected. This sidesteps a Claude Code v2.1.138 startup-refresh path that 401s when `CLAUDE_CONFIG_DIR` resolves to the bare `~/.claude` even with valid OAuth tokens in the keychain.",
          "Added `ccpm config get default_dir`. Prints the default profile's absolute path, or empty if no default is set. Used by the `claude()` wrapper; useful for scripts that need the same lookup.",
          "After upgrading, open a fresh terminal (or `source ~/.zshrc`) to pick up the new wrapper. IDE extensions launch claude directly and are not covered by the wrapper.",
        ],
      },
      {
        date: "2026-05-11",
        title: "`set-default` no longer reintroduces stale OAuth tokens",
        categories: ["Fixed"],
        bullets: [
          "`ccpm set-default` now folds the current default-slot OAuth payload (and identity in `~/.claude.json`) back into the previously-default profile before loading the new selection. Without this, plain `claude` / VSCode invocations would silently rotate the refresh token in the default slot only, and a later `set-default` of the same profile could copy a stale, already-rotated refresh token back into the default slot, causing the next `claude` call to fail with `401 Invalid authentication credentials`.",
          "Save-back is best-effort: any failure prints a yellow warning and the new default still applies.",
        ],
      },
      {
        date: "2026-05-09",
        title: "Profile rename and default IDE account",
        categories: ["Fixed", "Improved"],
        bullets: [
          "`ccpm rename` rewrites absolute profile paths inside plugin metadata so renamed profiles keep plugins and skills resolving.",
          "`ccpm set-default` for OAuth profiles syncs cached identity fields into `~/.claude.json` so the CLI welcome banner matches the selected default profile.",
        ],
      },
      {
        date: "2026-05-09",
        title: "Consolidate and audit tooling",
        categories: ["Added"],
        bullets: [
          "`ccpm consolidate` audits drift across host, share, and profile scopes with safe auto-fixes for dangling symlinks and stale plugin caches; a bundled slash skill documents deeper cleanup.",
        ],
      },
      {
        date: "2026-05-07",
        title: "OAuth on macOS after renames and status accuracy",
        categories: ["Fixed", "Improved"],
        bullets: [
          "`ccpm rename` migrates namespaced macOS keychain OAuth entries when a profile directory moves.",
          "`ccpm list` and auth status treat short-lived access tokens paired with a refresh token as healthy instead of warning on hourly expiry.",
          "macOS keychain writes for OAuth use the Security CLI so stored payloads match what Claude Code reads (no go-keyring base64 wrapper).",
        ],
      },
      {
        date: "2026-05-07",
        title: "Windows OAuth path (experimental)",
        categories: ["Improved"],
        bullets: [
          "Experimental Windows credential-manager path for OAuth, mirroring the macOS namespacing model. Still needs real-host verification.",
        ],
      },
      {
        date: "2026-05-04",
        title: "Atomic writes and managed plugins",
        categories: ["Improved", "Security"],
        bullets: [
          "Multi-file updates (settings + `.claude.json`, manifest + symlinks) go through transactional atomic writes.",
          "First-class `ccpm plugin` marketplace install, per-profile enable/disable, and garbage collection of shared cache.",
        ],
      },
      {
        date: "2026-05-03",
        title: "Host asset cascade",
        categories: ["Added", "Improved"],
        bullets: [
          "Skills, agents, commands, rules, hooks, and host plugin trees under `~/.claude/` can auto-adopt into profiles (opt-out via config or `--no-auto-adopt`).",
          "`ccpm doctor` reports cascade and shadowed names where profile-local assets win.",
        ],
      },
    ],
  },
  {
    series: "0.3.x",
    summary:
      "Parity with native Claude Code: scoped MCP, transports, permissions, sessions, and a simpler settings model.",
    releases: [
      {
        date: "2026-04-24",
        title: "Parity with native Claude Code CLI",
        categories: ["Added", "Improved"],
        bullets: [
          "`ccpm run` forwards unknown flags to `claude` without a `--` separator; `--ccpm-env` for one-shot env overrides.",
          "MCP: scopes (global / profile / project), HTTP and SSE transports, `ccpm mcp auth` via native Claude in profile scope.",
          "`ccpm env`, `ccpm permissions`, `ccpm sessions list`, managed-settings merge layer, and project settings/MCP discovery from CWD.",
        ],
      },
      {
        date: "2026-04-22",
        title: "Settings model simplification",
        categories: ["Improved", "Fixed"],
        bullets: [
          "Removed the ccpm-managed global settings fragment. Cross-profile baseline is `~/.claude/settings.json`, with per-profile fragments and owned-keys for durable overrides.",
        ],
      },
      {
        date: "2026-04-20",
        title: "MCP materialization and imports",
        categories: ["Fixed", "Improved"],
        bullets: [
          "MCP servers materialize into `<profile>/.claude.json` (where Claude Code reads them), not `settings.json`.",
          "`MaterializeMCP` merges host `~/.claude.json` user-scope MCPs so installs outside ccpm still appear in profiles.",
          "`ccpm import default` can import MCP entries from `~/.claude.json` with per-item selection.",
        ],
      },
    ],
  },
  {
    series: "0.2.x",
    summary: "Release automation and cross-platform path handling.",
    releases: [
      {
        date: "2026-04-17",
        title: "Release automation and quality",
        categories: ["Added", "Improved"],
        bullets: [
          "`scripts/release.sh` automates version bump, checks, tag, GitHub Release, and npm publish with optional `--stash` for partial trees.",
          "Import and fingerprint paths handle symlinked directories and Windows test sandboxes (`USERPROFILE`) correctly.",
        ],
      },
    ],
  },
  {
    series: "0.1.x",
    summary:
      "The first public ccpm: profile isolation via `CLAUDE_CONFIG_DIR`, OAuth + API key auth, encrypted vault, shared assets, and a Go-built single binary on npm, curl, and go install.",
    releases: [
      {
        date: "2026-04-12",
        title: "First public release",
        categories: ["Added"],
        bullets: [
          "Core CLI commands: `ccpm add`, `list`, `status`, `use`, `set-default`, `remove`, `auth`, `run`, `shell-init`, `uninstall`.",
          "Per-profile isolation by setting `CLAUDE_CONFIG_DIR` to `~/.ccpm/profiles/<name>/`. Two terminals, two accounts, zero conflicts.",
          "OAuth and API key flows. API keys live in the OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager).",
          "Encrypted credential vault under `~/.ccpm/vault/` using AES-256-GCM with a master key in the OS keychain.",
          "Shared asset store at `~/.ccpm/share/` for skills and other assets, symlinked into profiles to avoid duplication.",
          "Distribution: single static Go binary published to npm (`@ngcodes/ccpm`), curl installer, and `go install`. CI via GitHub Actions; release packaging via GoReleaser.",
        ],
      },
    ],
  },
];

/** Latest N releases flattened, newest-first across all series. */
export function getLatestChangelogReleases(n = 3): ChangelogRelease[] {
  const flat = CHANGELOG.flatMap((s) => s.releases);
  flat.sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0));
  return flat.slice(0, n);
}
