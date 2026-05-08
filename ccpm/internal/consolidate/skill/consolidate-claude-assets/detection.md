# Detection — issue catalog with fingerprints

Each issue has: a category, a fingerprint (how to find it), severity, and a remediation pointer to `proposals.md`.

## Severity tiers

- `info` — surface but never block
- `warn` — surface; require explicit user opt-in to fix
- `error` — block `ccpm sync` / Claude Code launch until resolved (rare)

---

## 1. Dangling symlink

- **Category**: `dangling`
- **Fingerprint**: `find ~/.claude/skills ~/.ccpm -type l ! -exec test -e {} \; -print`
- **Severity**: warn
- **Cause**: target file/dir was deleted but the symlink wasn't cleaned. Common after manual `rm` on host global without subsequent profile cleanup.
- **Remediation**: see `proposals.md` § "Dangling symlink"

## 2. Real-dir duplicate across scopes

- **Category**: `duplicate`
- **Fingerprint**: same skill name as real directories in two or more of `{~/.agents/skills/, ~/.ccpm/share/skills/, ~/.claude/skills/}`. Verify with `diff -rq <a> <b>` returning empty (identical).
- **Severity**: warn
- **Cause**: imported same skill via two routes (e.g. ccpm `add` + manual clone).
- **Remediation**: see `proposals.md` § "Real-dir duplicate"

## 3. Ghost manifest entry

- **Category**: `ghost`
- **Fingerprint**: `~/.ccpm/installs.json` contains `profiles[]` values that don't appear in `ccpm list` output.
- **Severity**: info
- **Cause**: profile renamed via `ccpm rename` (or older versions); some installs still reference the old name.
- **Remediation**: see `proposals.md` § "Ghost manifest entry"

## 4. Broken empty file

- **Category**: `broken`
- **Fingerprint**: `find ~/.ccpm ~/.claude ~/.agents -path '*/skills/*' -type f -size 0 -print`
- **Severity**: warn
- **Cause**: an `add` or `import` flow failed mid-write, leaving a zero-byte file where a SKILL.md or symlink should be.
- **Remediation**: see `proposals.md` § "Broken empty file"

## 5. Plugin/direct skill overlap

- **Category**: `overlap`
- **Fingerprint**: a skill name (e.g. `tdd`) exists as a direct skill at host global AND as a plugin-bundled skill (e.g. `compound-engineering:ce-debug` semantically overlaps `diagnose`). Detected by:
  - exact name match between direct and plugin SKILL.md files
  - heuristic keyword overlap on description (shared roots like "debug", "review", "tdd", "frontend")
- **Severity**: info
- **Cause**: user installed mattpocock direct skills + a plugin that ships its own engineering suite.
- **Remediation**: see `proposals.md` § "Plugin/direct overlap"

## 6. Skill description budget overflow

- **Category**: `budget`
- **Fingerprint**: per-profile reachable SKILL.md count > 180 (Claude Code's approximate description budget). See `budget-math.md` for the math.
- **Severity**: warn
- **Cause**: too many plugins enabled at the wrong scope.
- **Remediation**: see `proposals.md` § "Budget overflow"

## 7. Hooks duplicated host vs profile

- **Category**: `hook-dupe`
- **Fingerprint**: the same hook block (matcher + command path) appears in `~/.claude/settings.json` `hooks` AND in one or more `~/.ccpm/profiles/<p>/settings.json` `hooks`. Compare structurally, not byte-for-byte.
- **Severity**: warn
- **Cause**: profile inherited the host hook (cascade) but also has its own copy from earlier manual edit.
- **Remediation**: see `proposals.md` § "Hook duplication"

## 8. Permissions intersection across profiles

- **Category**: `perm-dupe`
- **Fingerprint**: any string in `permissions.allow` array appearing in `≥ 2` profile `settings.json` files.
- **Severity**: info
- **Cause**: org-wide tooling permissions accumulated per-profile rather than promoted to host.
- **Remediation**: see `proposals.md` § "Permissions intersection"

## 9. Plugin enabled at multiple scopes

- **Category**: `plugin-multi-scope`
- **Fingerprint**: same `<plugin>@<marketplace>` key in `enabledPlugins` at host AND profile, OR in two profiles independently.
- **Severity**: info
- **Cause**: plugin promoted to global but profile entries weren't cleaned, OR same plugin enabled per-profile when it should be global.
- **Remediation**: see `proposals.md` § "Plugin scope drift"

## 10. Stale plugin cache

- **Category**: `stale-cache`
- **Fingerprint**: `~/.ccpm/profiles/<p>/plugins/cache/<marketplace>/<plugin>/` exists but `<plugin>@<marketplace>` is not in any profile's `enabledPlugins`.
- **Severity**: info
- **Cause**: plugin disabled but cache dir remains. Eats disk; doesn't load.
- **Remediation**: `ccpm sync` claims plugin GC but sometimes leaves cruft. Manual cleanup safe.

## 11. MCP server defined at multiple scopes

- **Category**: `mcp-multi-scope`
- **Fingerprint**: same MCP server name in `mcpServers` of `~/.claude.json` AND a profile's `.claude.json`.
- **Severity**: info
- **Cause**: server promoted to global but profile entries left behind.
- **Remediation**: keep at one scope; remove from the lower-priority one.

---

## Detection script outputs

`detect-duplicates.py` prints one line per issue:

```
{category} | {severity} | {scope} | {asset} | {detail}
```

Example:
```
dangling | warn | host | ~/.claude/skills/caveman | target ~/.claude/skills/_sources/.../caveman missing
duplicate | warn | host+share | gitnexus-cli | identical real dirs in ~/.agents/skills/ and ~/.ccpm/share/skills/
ghost | info | manifest | tdd | profiles=[rocketium] not in live list [cin,labs,work]
broken | warn | share | code-review-excellence | 0-byte file at ~/.ccpm/share/skills/code-review-excellence
budget | warn | cin | 237 skills > 180 budget
```
