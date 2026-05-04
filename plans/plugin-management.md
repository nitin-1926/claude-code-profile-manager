# Plan: Plugin management fixes in ccpm

Status: **draft, awaiting decisions**
Owner: @nitin-1926
Last updated: 2026-05-02

## Why this plan exists

Four separate issues showed up while testing plugin behavior in profiles:

1. **`ccpm plugin list` is silently broken.** It hard-codes
   `~/.claude/plugins/installed_plugins.json` and the schema parsers in
   `loadInstalledPlugins` don't match what current Claude Code actually
   writes. Result: command says "no plugins installed" no matter what.
2. **There's no way to install a plugin from outside a session.** Today
   plugins must be installed by running `/plugin install <name>` inside a
   `ccpm run` session. That's awkward and means a plugin installed in
   profile A is invisible to profile B even when the user wanted it
   everywhere.
3. **No transactional safety on `ccpm run` materialization.** When ccpm
   merges shared `~/.claude/settings.json` + ccpm fragments + project
   overrides into a profile's `settings.json` (and writes related files)
   at launch, there is no rollback if a write fails partway through. A
   crash, disk-full, or permissions error mid-merge leaves the profile in
   an inconsistent state — settings half-written, MCP fragment partially
   updated, or symlinks dangling. aisw solved the same class of problem
   with a snapshot/stage/commit/rollback pattern in
   [`src/live_apply.rs`](https://github.com/burakdede/aisw/blob/main/src/live_apply.rs);
   ccpm should do the same. (See "Cross-cutting issue" below.)
4. **Cloning fails when Claude Code defaults to SSH and the user has no
   SSH keys.** Reproduced 2026-05-02 trying to add the notion plugin
   marketplace inside a `ccpm run cin` session:

   ```
   Failed to install: Failed to clone repository: Cloning into
   '/Users/nitingupta/.ccpm/profiles/cin/plugins/cache/temp_github_…'
   git@github.com: Permission denied (publickey).
   ```

   **Root cause is in Claude Code, not ccpm.** Verified the user's
   `git config --global --list` has no `url.*.insteadOf` rewrite forcing
   SSH; Claude Code is choosing `git@github.com:` itself when cloning
   marketplaces. ccpm's only contribution is setting
   `CLAUDE_CONFIG_DIR`, which redirects the clone destination — not the
   protocol.

   **Why this is in scope here**: Approach B below (ccpm-managed
   install) sidesteps the bug entirely because ccpm would issue the
   clone itself and can default to HTTPS. So this isn't a separate fix
   to land — it's evidence that Approach B has practical value beyond
   dedup.

   **Immediate user workaround** (no code change required):

   ```sh
   git config --global url."https://github.com/".insteadOf "git@github.com:"
   ```

   This makes git rewrite SSH URLs to HTTPS for github.com, which fixes
   any tool (not just Claude Code) that hard-codes SSH. Document this in
   ccpm's troubleshooting section so users hitting the error today have
   an answer before Approach B ships.

## Bugs to fix

### Bug A — wrong path

`ccpm/cmd/plugin.go:243` uses `~/.claude/plugins/installed_plugins.json`
as the source of truth. With ccpm running, plugins live at
`<profile_dir>/plugins/installed_plugins.json`, so the global path is
either empty or out of sync.

**Fix**: when iterating profiles in `runPluginList`, read each
profile's `<profile_dir>/plugins/installed_plugins.json` rather than the
global one. Build the union across profiles.

### Bug B — wrong schema

The current loader tries three shapes; none match what Claude Code
writes today (verified against
`~/.ccpm/profiles/rocketium/plugins/installed_plugins.json` on
2026-05-02):

```json
{
  "version": 2,
  "plugins": {
    "vercel@claude-plugins-official": [
      {
        "scope": "user",
        "installPath": "...",
        "version": "0.40.1",
        "installedAt": "...",
        "lastUpdated": "...",
        "gitCommitSha": "..."
      }
    ]
  }
}
```

**Fix**: add a fourth shape to `loadInstalledPlugins` that handles
`{version: int, plugins: map[id][]entry}`. Take the most recent entry
per ID (or the first; they're effectively versions of the same plugin).
Keep the old shapes as fallbacks for older Claude Code versions.

## Feature: plugin install / remove from outside

### Why

Without external install, the only path is `claude /plugin install` inside
a session, which:

- Forces an interactive session just to install
- Hides the install behind whichever profile happened to be active
- Has no shared-store dedup (every profile re-downloads marketplaces and
  plugin caches)

### Surface

```
ccpm plugin install <name>@<marketplace> --profile <name>
ccpm plugin install <name>@<marketplace> --global
ccpm plugin install <github-org>/<repo>  # adds the marketplace + installs
ccpm plugin remove <name>@<marketplace> --profile <name>
ccpm plugin remove <name>@<marketplace> --global
ccpm plugin marketplace add <github-org>/<repo>
ccpm plugin marketplace remove <name>
ccpm plugin marketplace list
```

`--global` installs into `~/.ccpm/share/plugins/` and links into every
profile (mirrors how skills/agents work today). `--profile <name>`
installs only for that profile.

### How it works

Two viable approaches; I recommend **Approach B** (shared store) but
list both with tradeoffs.

#### Approach A — shell out to `claude /plugin install`

Per profile, run `CLAUDE_CONFIG_DIR=<profile_dir> claude /plugin install
<id>` non-interactively (if Claude Code supports a non-interactive plugin
install — must verify; it may not).

Pros:
- Smallest amount of new code; we let Claude Code do the work
- Always tracks Claude Code's evolving plugin format

Cons:
- Likely requires an interactive session — defeats the point
- Spawns a `claude` process per profile per plugin (slow at scale)
- No dedup across profiles

#### Approach B — shared store + write the manifests ourselves

Mirror how skills are handled today.

1. **Marketplace**: `ccpm plugin marketplace add <repo>` clones into
   `~/.ccpm/share/plugins/marketplaces/<name>/` and parses the
   marketplace's `plugin.json` / `marketplace.json` to know what plugins
   exist.
2. **Plugin install (global)**: download/fetch the plugin into
   `~/.ccpm/share/plugins/cache/<marketplace>/<plugin>/<version>/`.
   Symlink from each profile's `plugins/cache/<marketplace>/<plugin>/<version>/`
   into the shared cache. Update each profile's
   `plugins/installed_plugins.json` and `plugins/known_marketplaces.json`
   to match. Set `enabledPlugins.<id>=true` in each profile's
   settings fragment.
3. **Plugin install (profile)**: same as global but only one profile
   touched.
4. **Plugin remove**: reverse — strip from
   `installed_plugins.json`, `known_marketplaces.json` (if last user),
   `enabledPlugins`, and the symlinks. Garbage-collect the shared cache
   when no profile references it.

Pros:
- Disk dedup (current rocketium profile alone has multi-GB plugin caches)
- Works without a Claude Code session
- Consistent with how `skill`/`agent`/`command`/`rule` are managed
- No assumption about Claude Code's CLI supporting headless install

Cons:
- We have to track Claude Code's plugin manifest schema (it's already
  shifted twice — see Bug B). Mitigation: same schema-fallback pattern
  used for `loadInstalledPlugins`.
- We're writing files Claude Code considers "its". If Claude Code adds
  fields, our writes might lose them. Mitigation: load → mutate →
  write-back rather than replace, like `settingsmerge` does.

### Reverse-engineering the marketplace format (one-time discovery)

Need to confirm before implementation:

- Format of `plugin.json` / marketplace metadata Claude Code clones from
- Whether plugins have a single canonical install location or vary by
  marketplace
- Whether `installPath` in `installed_plugins.json` is required to be
  absolute (it currently is — see live data); if so, profile symlinks
  need to record the symlink path, not the shared-store path
- What Claude Code does on conflict (same plugin@marketplace at
  different versions across profiles → does the cache directory layout
  permit it? The current layout
  `cache/<marketplace>/<plugin>/<version>/` suggests yes).

Allocate a small spike (a few hours) to confirm this before writing any
of the install code.

## Cross-cutting issue: transactional file writes

This is broader than plugins, but it lands here because plugin install /
remove is the most write-heavy operation we'd add and the consequences of
a half-applied install are the most user-visible (a profile that
half-knows about a plugin is worse than one that doesn't know at all).

### Where the gap is today

`ccpm run` and the asset commands write multiple files in sequence with
no atomicity guarantee. Concretely:

- `internal/settingsmerge` writes `<profile>/settings.json` after
  computing a multi-source merge. A failure after writing settings but
  before updating the owned-keys marker leaves the marker out of sync
  with the materialized file.
- `internal/share` writes shared assets and creates symlinks into each
  profile. If the symlink step fails after the file write, the shared
  store and profile views diverge.
- MCP fragment writes touch both `~/.ccpm/share/mcp/<profile>.json` and,
  at launch, the merged `mcpServers` block in the profile's settings. A
  partial write leaves the two out of sync.
- (Future) plugin install touches
  `<profile>/plugins/installed_plugins.json`,
  `known_marketplaces.json`, the symlink tree, and the
  `enabledPlugins` block in `settings.json` — four writes that must
  succeed or fail together.

### Pattern to adopt (from aisw)

aisw's
[`src/live_apply.rs`](https://github.com/burakdede/aisw/blob/main/src/live_apply.rs)
implements `apply_transaction(changes Vec<LiveFileChange>)` with this
shape:

1. **Snapshot** the original state of every target path (contents +
   mode, or "absent" if the path doesn't exist).
2. **Stage** every write as a sibling temp file (e.g.
   `<path>.ccpm-staged-<rand>`).
3. **Commit** by atomic-renaming each staged file over its target.
   Deletes are also staged (recorded as deletion-on-commit).
4. **Rollback** on any failure: restore each path from its snapshot
   (re-create absent files, restore contents + mode for present ones).
5. **Cleanup** staged files in all paths (success or failure).

Key safety properties aisw enforces that we should keep:

- Refuse to modify symlink targets (write only to regular files / new
  paths).
- Refuse non-file targets.
- Reject duplicate target paths within one transaction.
- Use atomic rename (same filesystem) so partial reads of the target
  during commit are impossible.

### Where to apply it in ccpm

1. **New package**: `internal/atomicwrite` (or fold into
   `internal/share`) exposing
   `Apply(changes []FileChange) error` with `Write` / `Delete` variants.
2. **`ccpm run` materialization** — wrap the settings merge + MCP
   fragment write + any other launch-time writes in a single
   transaction.
3. **Asset add/remove** — `share.WriteAsset` + symlink creation become a
   transaction. Same for delete.
4. **Plugin install/remove (Phase 2 below)** — must use the transaction
   API from day one so we never ship a non-atomic plugin path.

### Tradeoffs

- **Pros**: catastrophic-failure safety, cleaner reasoning ("this op
  either fully happened or didn't"), enables future features like
  `ccpm run --dry-run` (we already snapshot what we'd change).
- **Cons**: ~200–300 LOC of careful unix-fs code (atomic rename + mode
  preservation + symlink refusal), slight write overhead (two writes per
  target — staged + rename). Acceptable: launch-time writes are tiny.
- **Cross-platform footnote**: atomic rename across filesystems is not
  guaranteed; ccpm always writes within `~/.ccpm/` or `~/.claude/` so
  this is fine in practice. Windows: `os.Rename` over an existing file
  is fine on modern Windows; document if older Windows turns out to be
  a problem.

### Implementation order

This belongs in **Phase 1.5** (between bug fixes and external install) so
that Phase 2 plugin work can rely on it. Concrete tasks:

1. Build `internal/atomicwrite` with the snapshot/stage/commit/rollback
   primitives + unit tests covering: write-new, overwrite, delete,
   mid-transaction failure rollback, symlink-target refusal, duplicate
   path rejection.
2. Migrate `ccpm run` launch-time writes to a single transaction.
3. Migrate `internal/share.WriteAsset` and asset removal to
   transactions.
4. Migrate MCP fragment writes.
5. Document the pattern in AGENTS.md so new commands use it by default.

Phase 1.5 is independently shippable — it only adds safety, no new
user-visible behavior.

## Decisions to lock in (questions for owner)

1. **Approach A vs B.** Recommendation: **B (shared store)**. Higher
   upfront cost, much better long-term properties. Confirm.
2. **Should `--global` install activate the plugin everywhere by default,
   or just install + leave activation to `ccpm plugin enable`?**
   Recommendation: **install + enable everywhere by default**, mirroring
   what `claude /plugin install` does at user scope. Add an
   `--install-only` flag to skip activation.
3. **Garbage collection**. When a plugin is removed from the last
   profile that references it, do we delete the shared cache
   automatically or leave it for `ccpm sync` / `ccpm plugin gc` to clean?
   Recommendation: **explicit `ccpm plugin gc` command**, run it in
   `ccpm sync` automatically. Avoids surprise data loss.
4. **Ordering vs Codex plan**. The plugin work is substantial (manifest
   schema, marketplace fetch, gc, schema fallbacks). Should it ship
   before, after, or in parallel with the Codex plan?
   Recommendation: **Phase 1 bug fixes first (small PR, few hours)**,
   **then Phase 1.5 transactional writes (foundational for everything
   else)**, then Codex plan and Phase 2 plugin work in either order.
   Phase 1 unblocks honest plugin observability immediately; Phase 1.5
   pays for itself the moment Phase 2 or Codex starts writing
   multi-file state.
5. **Documentation of the SSH-clone workaround.** Add the
   `git config insteadOf` workaround to README troubleshooting now (one
   line), so users hitting the notion error today have an answer
   without waiting for Phase 2. Confirm.

## Implementation order

### Phase 1 — bug fixes only (small PR, few hours)

1. Fix path in `loadInstalledPlugins` to take a profile dir parameter.
2. Add v2 schema shape (`{version, plugins: map[id][]entry}`) to
   `loadInstalledPlugins`.
3. Update `runPluginList` to call the loader once per profile and
   union results.
4. Add a unit test fixture from a real Claude Code v2 file.
5. Add SSH-clone workaround note to README troubleshooting (one
   paragraph: `git config --global url."https://github.com/".insteadOf
   "git@github.com:"`).

This phase ships independently and fixes the observable broken `ccpm
plugin list`.

### Phase 1.5 — transactional file writes (foundational, no new UX)

1. Build `internal/atomicwrite` package: `Apply([]FileChange) error`
   with `Write` / `Delete` variants, snapshot/stage/commit/rollback
   semantics, atomic rename, symlink-target refusal, duplicate-path
   rejection. Full unit-test coverage of failure modes.
2. Migrate `ccpm run` launch-time writes (settings materialization, MCP
   fragment merge) to a single transaction.
3. Migrate `internal/share.WriteAsset` (asset add) and asset removal to
   transactions.
4. Document the pattern in `AGENTS.md` so future commands inherit it.

Ships independently. No user-visible change beyond "fewer ways to
corrupt your profile."

### Phase 2 — external install (depends on schema spike + Phase 1.5)

1. Schema spike: confirm marketplace format + install layout.
2. `ccpm plugin marketplace add/remove/list` writing into
   `~/.ccpm/share/plugins/marketplaces/`. **Always clone via HTTPS**
   (sidesteps the SSH-keys-required failure we hit with the notion
   plugin); allow `--ssh` opt-in for users who want it.
3. `ccpm plugin install --global` (shared cache + per-profile symlinks
   + manifest writes + enable). All file writes go through the
   `atomicwrite` package from Phase 1.5.
4. `ccpm plugin install --profile <name>` (no shared cache, just the
   profile dir). Same atomicity guarantees.
5. `ccpm plugin remove --global / --profile`.
6. `ccpm plugin gc`.
7. Hook `ccpm plugin gc` into `ccpm sync`.
8. Docs: README + AGENTS.md updates, including the HTTPS-by-default
   note.

### Phase 3 — quality of life (optional, after main work lands)

1. `ccpm plugin update [<id>]` — re-fetch, bump versions.
2. `ccpm plugin info <id>` — show source, version, install path,
   enabled-in profiles.
3. Migration command: `ccpm plugin migrate-to-shared` — for users with
   existing per-profile installs, move duplicate caches into shared
   store and replace with symlinks.

## Risks

- **Schema drift in Claude Code**: plugin manifest format has shifted
  twice already. Mitigation: schema-shape fallback pattern + integration
  tests that exercise the latest known shape.
- **Concurrent ccpm runs**: two terminals installing different plugins
  into different profiles could race on the shared cache. Mitigation:
  flock on `~/.ccpm/share/plugins/` during writes, like
  `installs.json` already does (verify).
- **Downloads**: marketplaces are git repos. We need git on PATH (likely
  already a dependency for Claude Code itself; document in `ccpm
  doctor`).
- **Windows**: shared store uses symlinks; on Windows without Developer
  Mode this falls back to copy. Plugins are bigger than skills, so the
  copy-fallback bloat is worse. Document.

## Open questions for follow-up

- Do we want a "plugin lock" similar to `package-lock.json` so the
  exact gitCommitSha is reproducible across machines? Probably yes for
  team workflows, but not v1.
- Marketplace authentication (private repos)? Not v1.
- Plugin signing / verification? Not v1; defer to whatever Claude Code
  does.
