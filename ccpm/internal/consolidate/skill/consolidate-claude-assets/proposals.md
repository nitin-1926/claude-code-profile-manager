# Proposals — decision trees per issue category

For each issue category, present the user with a `multiSelect` or single-select question via `AskUserQuestion`. Below is the option matrix per category. Always include the `Why:` and `How to apply:` lines so the user can judge.

---

## Dangling symlink

**Question**: "<path> points at <missing target>. What to do?"

| Option | Action | When to pick |
|---|---|---|
| Remove symlink (recommended) | `rm <path>` + remove matching `installs.json` entry | Target gone permanently. Default. |
| Restore target from backup | Restore the missing target file | If target was deleted by mistake. |
| Re-point at a new target | Replace symlink to a different existing skill | If target moved. |

After removal, run `ccpm sync` to clean up downstream profile cascades.

---

## Real-dir duplicate

**Question**: "Skill `<name>` is a real directory in <scopeA> and <scopeB> (identical). Which is canonical?"

| Option | Action |
|---|---|
| `~/.agents/skills/` canonical (recommended) | Keep `~/.agents/`, replace others with symlinks |
| `~/.ccpm/share/skills/` canonical | Keep share-scope copy, symlink others |
| `~/.claude/skills/` canonical | Keep host global, symlink others |
| Skip (leave both) | No change. Use only if dupes are intentional. |

Then: `rm -rf <non-canonical>` and `ln -s <canonical> <non-canonical-path>`.

---

## Ghost manifest entry

**Question**: "`installs.json` references profile `<ghost>` which doesn't exist. Map to: ?"

| Option | Action |
|---|---|
| Replace with current profile names (recommended) | If only one live profile, use it. If multiple, ask user to pick. |
| Drop the install entry entirely | Use if the install is orphan and not referenced elsewhere. |
| Leave as-is (cosmetic) | Skip — cascade still works empirically. |

After edit: `python3 -c "import json; ..."` rewrite + `cp installs.json installs.json.bak-consolidate-<ts>` first. Always.

---

## Broken empty file

**Question**: "`<path>` is a 0-byte file (broken install). Recover how?"

| Option | Action |
|---|---|
| Symlink to canonical source (recommended) | If the same skill exists at `~/.agents/skills/`, replace the broken file with `ln -s ~/.agents/skills/<name> <path>` |
| Restore from backup tarball | If a recent `~/.claude-ccpm-backup-*.tgz` exists, extract just that file |
| Remove the broken file | If skill is no longer wanted |

---

## Plugin/direct overlap

**Question**: "Plugin skill `<plugin>:<skill>` overlaps direct skill `<direct>`. Which to keep?"

| Option | Action |
|---|---|
| Keep direct, disable plugin's overlap skills | Most often safest — direct skills tend to be hand-curated. |
| Keep plugin, remove direct skill | Rare — only if plugin is the user's source of truth. |
| Extract plugin skill as direct, then disable plugin | If plugin has 1-2 useful skills among many. Use `scripts/extract-plugin-skill.sh`. |
| Keep both (accept overlap noise) | Triple-coverage ok if you don't mind cognitive overhead. |

If extracting: `cp -R <plugin-cache>/skills/<name> ~/.claude/skills/<name>` then disable plugin in `enabledPlugins`. Note: cache may be re-populated on plugin update; the extracted direct skill is durable.

---

## Budget overflow

**Question**: "Profile `<p>` loads `<N>` skills (over ~180 budget). Reduce by:"

| Option | Action |
|---|---|
| Demote heavy plugin from global to specific profile (recommended) | Identify which global plugin contributes most (count plugin SKILL.md files); move to one profile that needs it. |
| Disable unused plugin skills (cache prune) | Fragile (plugin update reverts) — only as stop-gap. |
| Extract a few useful plugin skills as direct, disable rest of plugin | Most durable surgical reduction. |
| Accept the warning | Dropped skills still load when explicitly invoked. |

Heuristic for which plugin is heaviest:
```bash
for d in ~/.ccpm/profiles/*/plugins/cache/*/*/*/skills; do
  echo "$d: $(find $d -name SKILL.md | wc -l)"
done | sort -k2 -n
```

---

## Hook duplication

**Question**: "Hook `<matcher>:<cmd>` exists at host AND in profile `<p>`. Resolve:"

| Option | Action |
|---|---|
| Keep at host, strip from profile (recommended) | Host hook fires for all profiles via cascade. Profile copy is redundant. |
| Keep at profile only, strip from host | Use if other profiles should NOT have this hook. |
| Keep in both | No-op; cascade dedupes during merge but bytes remain. |

To strip: edit profile `settings.json` and remove the `hooks` block (or the specific matcher entry).

---

## Permissions intersection

**Question**: "These permission entries appear in `<N>` profiles. Promote to host global?"

Show the user the intersection list (entries in ≥2 profiles). User picks which to promote.

| Option | Action |
|---|---|
| Promote intersection to host (recommended) | Append to `~/.claude/settings.json` `permissions.allow`; remove from each profile's allow list. |
| Per-entry choice | Let user toggle each. |
| Skip | Leave duplicates per profile. Wastes a few hundred bytes. |

**Warning**: ccpm sync rewrites materialized profile `settings.json` from the share-scope fragment. Profile-unique permissions must be written into `~/.ccpm/share/settings/<p>.json`, NOT the materialized output, or they'll be lost on next sync. See `ccpm-aware.md` § "Settings cascade".

---

## Plugin scope drift

**Question**: "Plugin `<plugin>` enabled at <scopeA> and <scopeB>. Consolidate to:"

| Option | Action |
|---|---|
| Host global only (recommended for multi-profile use) | Add to `~/.claude/settings.json`, remove from profiles. |
| One profile only | Pick which profile. Remove from others. |
| Project only | Add to repo `.claude/settings.local.json`. Remove from all higher scopes. |

---

## MCP scope drift

Same as plugin scope drift — pick canonical scope, remove from others. Then `ccpm sync`.

---

## Stale plugin cache

**Question**: "Cache for disabled plugin `<plugin>` still on disk (`<path>`, <size>). Delete?"

| Option | Action |
|---|---|
| Delete (recommended) | `rm -rf <cache-path>`. Reversible by re-enabling plugin. |
| Keep | If you might re-enable soon. |
