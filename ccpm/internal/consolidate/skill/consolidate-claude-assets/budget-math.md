# Skill description budget math

## What "skill descriptions dropped" means

Claude Code injects a `<system-reminder>` listing all loaded skills with their descriptions, so the model can proactively trigger them. This list has a soft byte/character ceiling. When total skill description length exceeds the ceiling, lower-priority entries are dropped from the auto-suggest list. **Dropped skills still load when explicitly invoked** (`/skill-name` or `Skill` tool) — they just don't get auto-suggested.

The `/doctor` slash command shows the count of dropped descriptions.

## Counting reachable skills per profile

Reachable = direct skills in profile cascade + plugin-bundled SKILL.md from enabled plugins.

```bash
# Direct skills (symlinks under profile cascade)
N_DIRECT=$(ls ~/.ccpm/profiles/<p>/skills/ 2>/dev/null | grep -v '^_' | wc -l)

# Plugin-bundled SKILL.md (each enabled plugin contributes its skill dirs)
N_PLUGIN=$(find ~/.ccpm/profiles/<p>/plugins/cache -name SKILL.md 2>/dev/null | wc -l)

# For non-ccpm users:
N_DIRECT=$(ls ~/.claude/skills/ 2>/dev/null | grep -v '^_' | wc -l)
N_PLUGIN=$(find ~/.claude/plugins -name SKILL.md 2>/dev/null | wc -l)

echo "Total: $((N_DIRECT + N_PLUGIN))"
```

## Empirical threshold

In observed Claude Code 2.x: warning starts around **180-200** loaded skills. Single-digit drops at 200; large drops (50+) at 240+.

Use 180 as a safe ceiling — leaves headroom for plugins added later.

## Heaviest plugins (typical)

| Plugin | SKILL.md count |
|---|---|
| `vercel@claude-plugins-official` | 126 |
| `compound-engineering@compound-engineering-plugin` | 39 |
| `chrome-devtools-mcp@claude-plugins-official` | 24 |
| `superpowers@claude-plugins-official` | 14 |
| `context7@claude-plugins-official` | 0 (MCP-only, no skills) |
| `github@claude-plugins-official` | 0 (MCP-only) |
| `gopls-lsp@claude-plugins-official` | 0 (LSP-only) |

Promoting `vercel` to global on a profile that already has 30+ direct skills + another mid-size plugin tips most users over the budget.

## Reduction strategies (in order of impact)

1. **Demote heaviest plugin from global to one profile** that needs it. Single biggest reduction.
2. **Disable plugins overlapping with direct skills** — e.g. `code-review`, `code-simplifier`, `pr-review-toolkit`, `skill-creator`, `frontend-design`, `playwright` if you have direct equivalents (mattpocock + ccpm-shared often cover these).
3. **Extract 1-2 useful plugin SKILL.md files as direct, disable the rest of the plugin** — durable surgical reduction.
4. **Prune cache directories** of disabled plugins (cosmetic; doesn't affect load count once plugin is disabled).

## What `detect-budget.py` reports

For each profile (or just `~/.claude/` if no ccpm):

```
profile=cin direct=33 plugin=81 total=114 budget=180 status=ok
profile=labs direct=33 plugin=38 total=71 budget=180 status=ok
profile=work direct=33 plugin=39 total=72 budget=180 status=ok
profile=foo direct=30 plugin=200 total=230 budget=180 status=OVERFLOW (50 over)
```

When status is `OVERFLOW`, surface a `proposals.md` § "Budget overflow" question.
