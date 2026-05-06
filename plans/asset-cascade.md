# Plan: Asset cascade — Project → Profile → Global → Host

Status: **implemented (2026-05-03)** — landed in `internal/sync/{host_adopt,host_plugins,sync}.go`, `internal/manifest/manifest.go` (ScopeHost), `internal/config/config.go` (CascadeAutoAdopt), `cmd/{run,sync,doctor,config,assetcmd}.go`. Tests in `internal/sync/host_adopt_test.go`. Decisions taken: cascade ON by default, plugin payload via symlink, new `scope=host` distinct from `global`, `ccpm run` prints project-asset summary, doctor flags shadowed cases.
Owner: @nitin-1926
Last updated: 2026-05-03

## Why this plan exists

A user installed skills via `npx skills@latest add mattpocock/skills` and they
never appeared inside their active ccpm profile. Investigation showed this is
not a bug in skill loading — it's an architectural gap that affects every
**directory-based asset kind** (skills, agents, commands, rules, hook
scripts) and likely plugins too.

Three facts collide:

1. Claude Code itself supports only two scopes for directory-based assets:
   **project** (`<repo>/.claude/<asset>/`) and **user**
   (`$CLAUDE_CONFIG_DIR/<asset>/`). It does not merge two user-level
   directories. There is no native Profile + Global cascade.
2. `ccpm run` sets `CLAUDE_CONFIG_DIR=~/.ccpm/profiles/<name>`. The moment
   that env var points at a profile dir, `~/.claude/<asset>/` becomes
   completely invisible to Claude Code.
3. ccpm's sync mechanism (`internal/sync/sync.go:31` `ApplyGlobals`) only
   links assets present in `~/.ccpm/installs.json`. Third-party tools that
   write straight to `~/.claude/<asset>/` (skills CLI, manual edits,
   plugin installers) bypass the manifest and are therefore invisible to
   every profile forever.

The user's mental model — **Project (repo-local) → Profile (ccpm) → Global
(`~/.claude/`)**, with project winning over profile winning over global —
is reasonable. It's also the model ccpm already implements for **MCP**
(`internal/settingsmerge/merge.go:385` auto-imports
`~/.claude.json#mcpServers` into every profile) and for **settings** (the
managed > project > profile > global merge pipeline). Directory assets are
the asymmetry.

This plan closes that asymmetry consistently across every asset kind.

## Goal

Every asset kind in ccpm resolves with the same priority: **Project > Profile
> Global**. Anything a user has installed at `~/.claude/<asset>/` —
regardless of how it got there — is visible inside every ccpm profile by
default. Profile-local entries override globals. Project-local entries
(when Claude Code is launched from inside a project tree) override both.

Non-goals for v1:

- Cross-tool cascade (Codex / Cursor / opencode). Tracked separately in
  `plans/codex-support.md`.
- A new "ccpm-global" layer above `~/.claude/`. We already removed that
  (see SUMMARY entry 2026-04-22). The host `~/.claude/` is the global tier.
- Live filesystem watchers. We resolve at launch and on explicit `sync`,
  not in the background.

## Per-asset audit

This is the matrix. Each row is one asset kind, what surface(s) it lives
on, what Claude Code does natively, what ccpm does today, and the gap.

| Asset           | Native Claude reads from                      | ccpm today                                                                                  | Cascade today | Gap                                                                                                  |
|-----------------|-----------------------------------------------|---------------------------------------------------------------------------------------------|---------------|------------------------------------------------------------------------------------------------------|
| **skills**      | `<project>/.claude/skills/`, `$CCD/skills/`   | Links manifest entries from `share/skills/` into `<profile>/skills/`                        | Project + Profile only | `~/.claude/skills/` invisible unless explicitly imported via `ccpm import default --only skills`     |
| **agents**      | `<project>/.claude/agents/`, `$CCD/agents/`   | Same as skills (`assetcmd.go` shared shape)                                                 | Project + Profile only | Same as skills                                                                                       |
| **commands**    | `<project>/.claude/commands/`, `$CCD/commands/`| Same as skills                                                                              | Project + Profile only | Same as skills                                                                                       |
| **rules**       | `<project>/.claude/rules/`, `$CCD/rules/`     | Same as skills                                                                              | Project + Profile only | Same as skills                                                                                       |
| **hook scripts**| `$CCD/hooks/<file>` (referenced from settings)| Dedup via `share/hooks/`                                                                    | Profile only  | `~/.claude/hooks/<file>` invisible (the *script body* never reaches the profile)                     |
| **hook activations** (`settings.json#hooks`) | `<project>/.claude/settings.json`, `$CCD/settings.json`, managed | Layered merge in `MaterializeAll` already pulls global → profile → project → managed         | **Full cascade ✓** | None                                                                                                 |
| **plugins (payload)** | `$CCD/plugins/<name>/`                  | New `internal/plugins` clones marketplaces under `<profile>/plugins/`                       | Profile only (TBC) | `~/.claude/plugins/<name>/` payloads installed by `/plugin install` outside ccpm are invisible        |
| **plugin enablement** (`settings.json#enabledPlugins`) | settings layer | Layered merge same as hook activations                                                      | **Full cascade ✓** | None — but only useful if the payload is also visible (see above)                                    |
| **MCP servers** | `~/.claude.json#mcpServers`, `<project>/.mcp.json`, `<project>/.claude/settings.json#mcpServers`, `$CCD/.claude.json#mcpServers`, managed-settings drop-ins | `MaterializeMCP` merges all five sources via `loadHostClaudeJSONMCP`, project, profile fragments, managed | **Full cascade ✓** | None — this is the precedent we're copying                                                            |
| **settings**    | layered native pipeline                       | `Materialize` already applies managed > project > profile-fragment > host (`~/.claude/`) > defaults, with owned-keys preservation | **Full cascade ✓** | None                                                                                                 |
| **memory** (CLAUDE.md / AGENTS.md) | project root + walk upward + `~/.claude/CLAUDE.md` | Not currently managed by ccpm | N/A | Out of scope; native walk handles it correctly because `~/.claude/CLAUDE.md` is read directly         |

**Summary:** the gap is exactly the **directory-based asset kinds**:
skills, agents, commands, rules, hook scripts, and plugin payloads. Six
kinds, one shared fix.

## Design

### Approach: auto-adoption at launch + doctor visibility

Mirror the existing host-MCP pattern (`merge.go:385`). On every `ccpm run`
(and on `ccpm add` / `ccpm sync`), `ApplyGlobals` will:

1. For each dedupable asset kind, scan `~/.claude/<plural>/`.
2. For each top-level entry not already present in the manifest, register
   it as `scope = global`, `source = "host:/Users/.../.claude/<plural>/<name>"`,
   and append to `installs.json` via `atomicwrite`.
3. Link the entry into every profile's `<profile>/<plural>/<name>` (using
   the existing share-store dedup path when feasible, or a direct symlink
   to `~/.claude/<plural>/<name>` when the user opts out of sharing).
4. Profile-local entries (added via `ccpm <asset> add`) keep priority —
   they're already in the manifest with `scope = profile`, and the link
   step short-circuits if the destination already resolves to a non-host
   target.

Project assets need no change: Claude Code reads `<project>/.claude/<asset>/`
natively, and `discoverProjectAssets` already surfaces them in
`ccpm <asset> list`. Project-local already wins because Claude Code's own
loader gives project precedence.

### Why auto-adoption is safe

- **Reversible.** Every auto-adopted entry is recorded in `installs.json`
  with a distinct provenance (`source = "host:..."`). `ccpm <asset> remove`
  works on it like any other entry.
- **Reproducible.** Because the entries land in the manifest, two machines
  with the same `~/.claude/` and the same `installs.json` produce the
  same profile. No silent magic.
- **Opt-out exists.** A new `ccpm.cascade.autoAdopt = false` setting (or
  `--no-auto-adopt` flag on `run`/`sync`) disables the scan for users who
  want strict reproducibility and prefer the explicit `ccpm import default`
  path.
- **One-time warning.** First time auto-adoption fires for a profile, print
  one stderr line: `"adopted N items from ~/.claude (skills:3 agents:2 …)
  — disable with `ccpm settings set ccpm.cascade.autoAdopt false`"`.

### Plugin payload handling

Plugins need a small extension. `ccpm plugin list` already parses
`installed_plugins.json`, but the new `internal/plugins` pipeline writes
under `<profile>/plugins/`. To cascade host-installed plugins:

- Treat `~/.claude/plugins/<name>/` payloads the same way: scan, register
  in the manifest as `kind = plugin, scope = global`, and either
  (a) symlink the payload directory into `<profile>/plugins/<name>/`, or
  (b) keep the payload host-side and write a pointer entry in the
  profile's `installed_plugins.json` that references `~/.claude/plugins/`.

Option (a) keeps everything inside the profile dir (consistent with skills/
agents). Option (b) avoids duplicating multi-megabyte plugin clones. **Open
decision below.**

### Hook scripts

Hook scripts in `~/.claude/hooks/<file>` are referenced from
`settings.json#hooks` by *path*. Today the settings layer cascades but the
script files themselves don't. Two valid approaches:

- Symlink `~/.claude/hooks/<file>` into `<profile>/hooks/<file>` and
  rewrite the path inside the merged hooks block to point at the profile-
  local copy.
- Leave the path absolute (pointing at `~/.claude/hooks/<file>`) and rely
  on the file existing on disk. Simpler; doesn't need path rewriting.

Recommend the second — hooks are infrequently profile-specific and the
absolute path resolves identically for every profile.

## Implementation steps

Single PR, in order. Each step is small and individually testable.

1. **Add `kindCascadeSpec`** in `internal/sync/sync.go` next to `kindDirs`.
   For each dedupable kind, declare the host-side scan dir
   (`~/.claude/<plural>/`). MCP, setting, plugin-enablement skipped — they
   already cascade.
2. **`scanHostUnadopted(spec, manifest) []hostEntry`** — directory walk
   that returns `~/.claude/<plural>/` entries not already represented in
   the manifest. Tested against a fake home with 3 skills, 2 agents, one
   already-adopted, one symlink, one regular file.
3. **`autoAdoptHost(profile, spec, entries, opts)`** — for each entry,
   append to manifest (`atomicwrite`), then link into the profile (reuse
   `share.Link`). If `opts.Share` is true, route through the share store
   first; otherwise direct symlink to `~/.claude/<plural>/<name>`.
4. **Wire into `ApplyGlobals`** — call scan + auto-adopt before the
   existing manifest-driven link loop. Guard behind
   `ccpm.cascade.autoAdopt` (default `true`). One-time stderr warning
   per profile per scan that finds new entries.
5. **Plugin pipeline extension** — same scan/adopt for
   `~/.claude/plugins/<name>/`, behind the open decision below.
6. **Doctor check** — new `doctor` section: "host assets". For each kind,
   show count of host-only / adopted / profile-overridden. Already we
   have `ComputeProfileDiffs` (`internal/defaultclaude/diff.go`) which
   does most of this; promote it from a one-time-import helper to a
   recurring health signal.
7. **`--no-auto-adopt` flag** on `ccpm run` and `ccpm sync` for one-shot
   skips without changing the persistent setting.
8. **Settings key** `ccpm.cascade.autoAdopt` documented in README and
   AGENTS, persisted via the owned-keys mechanism so it survives merges.
9. **Tests:** unit tests for `scanHostUnadopted`, `autoAdoptHost`,
   `ApplyGlobals` end-to-end against a temp home with seeded `~/.claude/`
   trees, opt-out flag behavior, idempotency (running twice doesn't
   duplicate entries), and a regression that a profile-local override
   wins over an auto-adopted host entry of the same name.
10. **Sandbox test** — extend `/tmp/ccpm-sandbox.sh` with cases that
    drop matt-pocock-style skills into the fake `~/.claude/skills/`
    after profile creation and assert they appear in the profile after
    `ccpm sync`.

## Open decisions

These need user input before implementation lands.

1. **Default for `ccpm.cascade.autoAdopt`** — `true` (cascade on by
   default, opt-out for power users) or `false` (cascade is opt-in)?
   Recommend **`true`**: matches user mental model, matches host-MCP
   precedent, removes the "I installed a skill and it didn't show up"
   support load.
2. **Plugin payload strategy** — symlink full payload tree into each
   profile (option a, simpler, more disk) or pointer-only (option b,
   leaner, more code). Recommend **(a)** for consistency with skills;
   payloads are small in practice and the symlink itself is cheap.
3. **Naming for the manifest scope** — keep `scope = "global"` (overloads
   the existing meaning, which today implies "registered through ccpm
   add --global") or introduce `scope = "host"` to distinguish
   auto-adopted entries. Recommend **`scope = "host"`** so `ccpm list`
   can render a different column and `doctor` can flag them as a
   distinct category. Backwards-compat: existing global entries keep
   their scope.
4. **Project-asset surfacing in `ccpm run`** — today
   `discoverProjectAssets` only feeds the `list` commands. Worth printing
   a one-line stderr summary on `ccpm run` like
   `"3 project-local skills active in this directory"`? Pure UX call.
5. **Conflict resolution UX** — when a host entry and a profile-local
   entry share a name, profile wins silently. Should `doctor` flag this
   as a "shadowed" case (similar to how `which -a` shows shadowing)?
   Recommend yes, since silent shadowing is a known footgun.

## Validation plan

Before merging:

- All existing unit tests pass (`go test ./...`).
- New tests for the six functions above.
- Sandbox script (`/tmp/ccpm-sandbox.sh`) extended with at least three
  scenarios: clean profile + matt-pocock skills appear, opt-out flag
  suppresses adoption, profile override wins over host entry.
- Manual: install `mattpocock/skills` via `npx skills@latest`, run
  `ccpm sync`, restart `ccpm run rocketium`, verify all twelve skills
  show up in the active session.
- Manual: same but with the opt-out flag — verify they do not show up
  and `doctor` reports them as host-only.
- Cross-platform: build green for `linux/amd64` and `windows/amd64`
  (host scan must handle Windows paths and the existing symlink-fallback
  behavior).

## Risk + rollback

Risk is low: the change is additive on top of `ApplyGlobals` and gated by
a setting. Rollback is `ccpm settings set ccpm.cascade.autoAdopt false`
plus deleting the auto-adopted entries from `installs.json` (those are
the ones with `scope = "host"`). No schema migration; the manifest format
is forward-compatible.

The one tail risk is **silent adoption of an unwanted skill**. A user
might have a stale skill in `~/.claude/skills/` they forgot about, and
auto-adoption would suddenly start surfacing it inside every profile.
Mitigations: (a) the one-time stderr warning lists what was adopted,
(b) `doctor` exposes the host-asset count, (c) opt-out exists, (d) any
adopted entry can be removed with the standard `ccpm <asset> remove`.
