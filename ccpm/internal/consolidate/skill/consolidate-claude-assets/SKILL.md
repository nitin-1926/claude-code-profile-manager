---
name: consolidate-claude-assets
description: Audit, dedupe, and consolidate Claude Code assets (skills, MCPs, plugins, hooks, permissions) across host, profile, and project scopes. Use when the user reports duplicate skills, "skill descriptions dropped" or budget-overflow warnings, broken or dangling symlinks, ghost profile entries, plugin/skill overlap, or asks to clean up, tidy, audit, or consolidate their Claude Code setup. Detects ccpm at runtime — works for both ccpm and non-ccpm setups.
---

# Consolidate Claude Assets

## When to use

- User says "tidy / clean up / consolidate / audit / dedupe my Claude setup"
- Claude Code shows "N skill descriptions dropped" warning
- `ls -la` reveals dangling symlinks under `~/.claude/skills/` or `~/.ccpm/profiles/*/skills/`
- User suspects duplicate skills, plugin/skill overlap, ghost profile names in manifest, or stale plugin caches
- User wants to promote/demote assets between host, profile, and project scopes

## Workflow (read this first)

This skill runs in three phases, separated by user confirmation:

1. **Detection** (read-only) — inventory all scopes, list issues
2. **Proposal** (interactive) — present per-issue choices via `AskUserQuestion`
3. **Application** (`--fix` only) — apply confirmed fixes after backup

Never skip the backup. Never apply destructive edits without per-issue confirmation.

## Phase 1: Detection

```bash
bash scripts/inventory.sh        # JSON snapshot of all scopes → /tmp/claude-inventory.json
python3 scripts/detect-duplicates.py   # dupes, dangling symlinks, ghost manifest, broken empty files
python3 scripts/detect-budget.py       # per-profile loaded SKILL.md count vs ~180 budget
```

Read [`detection.md`](detection.md) for the full issue catalog and how to interpret each script's output.

If `~/.ccpm/` exists, also read [`ccpm-aware.md`](ccpm-aware.md) for ccpm-specific scopes (`installs.json`, share scope, owned-keys, manifest reconciliation).

## Phase 2: Proposal

For each issue surfaced in Phase 1, walk the decision tree in [`proposals.md`](proposals.md):

- **Real-dir dupe across scopes** → choose canonical home, replace others with symlinks
- **Dangling symlink** → confirm target gone, remove symlink + manifest entry
- **Ghost manifest entry** → reconcile profile names against `ccpm list`
- **Broken empty file** → restore from backup or symlink to canonical source
- **Plugin/direct overlap** → keep one; for plugin → either disable or extract specific SKILL.md as direct
- **Budget overflow** → demote unused plugins from global, prune niche skills
- **Hooks duplicated** → keep at host global, strip from profile settings
- **Permissions duplicated** → promote intersection to host global, leave unique per-profile

Use `AskUserQuestion` (multi-select where appropriate, max 4 options each). Never assume — even small edits affect downstream sync.

## Phase 3: Application (`--fix` only)

```bash
bash scripts/backup.sh                 # tarball ~/.claude + ~/.ccpm before any write
# apply confirmed fixes from Phase 2
ccpm sync                              # if ccpm present
bash scripts/verify-cascade.sh         # diff against pre-fix snapshot
```

Always print a summary at the end: what changed, where backups are, what to verify in the next Claude Code session.

## Anti-patterns (do not)

- Apply any destructive edit without `scripts/backup.sh` first
- Auto-merge plugin skills into direct skills without explicit user confirmation
- Edit `~/.ccpm/installs.json` without writing a `.bak-consolidate-<timestamp>` next to it
- Run `ccpm sync` before applying fixes — sync materializes current state and may overwrite intermediate changes
- Modify `~/.ccpm/share/settings/*.owned.json` (advanced ccpm internal — leave to ccpm CLI)
- Delete a skill at host global without first removing dependent symlinks in profile cascades

## References

- [`inventory.md`](inventory.md) — exact paths to scan per scope
- [`detection.md`](detection.md) — issue catalog with fingerprints
- [`proposals.md`](proposals.md) — decision trees per issue category
- [`ccpm-aware.md`](ccpm-aware.md) — ccpm scopes, manifest, share semantics
- [`budget-math.md`](budget-math.md) — skill description budget math
