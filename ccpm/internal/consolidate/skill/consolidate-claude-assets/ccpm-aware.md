# ccpm-aware — scopes, manifest, share semantics

This file applies only when `~/.ccpm/` exists. Skip otherwise.

## Scope precedence (Claude Code merge order)

Project > Profile > Host (`~/.claude/`)

## Three storage tiers (real bytes live here)

1. `~/.agents/skills/` — Anthropic-style canonical real dirs
2. `~/.ccpm/share/skills/` — ccpm intermediate cascade scope. Mix of symlinks → `~/.agents/` and real dirs.
3. `~/.claude/skills/_sources/<repo>/` — vendored upstream clones (e.g. mattpocock/skills)

## Cascade view (where Claude Code looks)

`~/.ccpm/profiles/<p>/skills/<name>` is always a **symlink** auto-managed by ccpm sync. Targets either:
- `~/.claude/skills/<name>` (host global), or
- `~/.ccpm/share/skills/<name>` (share scope)

If the profile dir contains a real (non-symlink) directory, that's a bug — flag and fix.

## installs.json schema

```json
{
  "version": 2,
  "installs": [
    {
      "id": "tdd",
      "kind": "skill",            // skill | agent | command | hook | plugin | mcp
      "scope": "host",            // host | global | profile
      "source": "host:/path",     // host:<path> or default:<path> or absolute path
      "profiles": ["cin", "labs", "work"],
      "created_at": "2026-04-18T19:32:10Z"
    }
  ]
}
```

### Scope semantics

| scope | meaning | auto-cascades to all profiles? |
|---|---|---|
| `host` | source is `~/.claude/...` | Yes |
| `global` | ccpm-installed at top level | Yes |
| `profile` | added per-profile only | No — only listed profiles |

If a skill should be available everywhere but is `scope=profile`, promote: edit the entry to `scope=host`, change source prefix from `default:` to `host:`. Then `ccpm sync`.

## Settings cascade — beware materialized vs source

Three layers:
1. Host: `~/.claude/settings.json`
2. Profile fragment: `~/.ccpm/share/settings/<p>.json` ← **source of truth for profile-specific values**
3. Profile owned-keys: `~/.ccpm/share/settings/<p>.owned.json` ← keys re-asserted during merge

Materialized output: `~/.ccpm/profiles/<p>/settings.json` ← **rewritten on every `ccpm sync`**.

**Common bug**: editing the materialized file directly. Changes survive until the next sync, then get clobbered. Always write profile-unique permissions/MCPs/plugins into the fragment file.

```bash
# Wrong (lost on next sync):
$EDITOR ~/.ccpm/profiles/cin/settings.json

# Right:
$EDITOR ~/.ccpm/share/settings/cin.json
ccpm sync
```

For permissions: the consolidate skill's `apply.go` MUST read/write the fragment, not the materialized output. Same for `enabledPlugins` and `mcpServers`.

## Adoption manifest behavior

`ccpm sync` does:
1. Reads host `~/.claude/*` and ccpm share `*` assets
2. Reconciles against `installs.json`
3. Computes per-profile merged view
4. Writes merged view atomically to `~/.ccpm/profiles/<p>/`
5. Cleans dangling profile symlinks (sometimes — not always reliable)
6. Garbage-collects unused plugin caches (opportunistic)

If `cascade_auto_adopt=true` (default), unmodified host items get auto-added to the manifest with `scope=host`. This is why direct edits to `~/.claude/skills/` work after a sync.

## Common ccpm-specific issues

- **Ghost profiles in manifest**: `installs.json` `profiles[]` references a name that's not in `ccpm list`. Likely from a `ccpm rename` or older version. Fix: rewrite the manifest with `python3` (always backup first).
- **Share-scope dupes vs `~/.agents/`**: same skill as real dir in both. Pick `~/.agents/` as canonical, replace share copy with symlink.
- **`scope=profile` skill that should be host**: skill cascades only to manifest-listed profiles, not new ones. Promote scope.
- **Owned-keys file out of sync**: rare. If profile-unique values keep getting lost on sync, check `<p>.owned.json` — it should list the keys ccpm preserves through the merge.

## Sync-after-fix invariant

After applying any fix, run `ccpm sync` exactly once. Don't run it twice — the second run is a no-op but adds latency. Don't run it before fixes are complete — it materializes intermediate state.

## Reading manifest in scripts

```python
import json
m = json.load(open(f"{HOME}/.ccpm/installs.json"))
for i in m["installs"]:
    print(i["id"], i["scope"], i.get("profiles", []))
```

## Detecting drift between fragment and materialized

```python
import json
HOME = "/Users/nitingupta"
for p in ("cin", "labs", "work"):
    frag = json.load(open(f"{HOME}/.ccpm/share/settings/{p}.json"))
    mat = json.load(open(f"{HOME}/.ccpm/profiles/{p}/settings.json"))
    # Anything in mat['permissions']['allow'] not in frag['permissions']['allow']
    # AND not in host settings = orphan that will be wiped on next sync
```
