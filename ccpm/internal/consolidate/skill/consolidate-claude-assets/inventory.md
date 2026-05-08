# Inventory — paths to scan per scope

## Host global (always present)

| Path | What lives here |
|---|---|
| `~/.claude/skills/` | Direct skill dirs and symlinks. Each entry has `SKILL.md`. |
| `~/.claude/agents/` | Agent definition `.md` files. |
| `~/.claude/commands/` | Slash command `.md` files. |
| `~/.claude/hooks/` | Hook scripts (e.g. PreToolUse, PostToolUse). |
| `~/.claude/settings.json` | Global settings (deep-merged into profiles). Keys of interest: `enabledPlugins`, `permissions.allow`, `hooks`. |
| `~/.claude.json` | Global MCP servers under `mcpServers`. |
| `~/.claude/plugins/` | Plugin caches + marketplaces (when present). |

## ccpm scopes (only if `~/.ccpm/` exists)

| Path | Role |
|---|---|
| `~/.ccpm/installs.json` | Manifest of installed assets. Each entry: `id, kind, scope, source, profiles[]`. |
| `~/.ccpm/profiles/<p>/` | One profile dir per profile. |
| `~/.ccpm/profiles/<p>/skills/` | Auto-managed symlinks → host or share. |
| `~/.ccpm/profiles/<p>/settings.json` | Materialized merged settings (NOT source of truth). |
| `~/.ccpm/profiles/<p>/.claude.json` | Materialized merged MCP servers. |
| `~/.ccpm/share/skills/` | ccpm intermediate cascade scope. May be symlinks or real dirs. |
| `~/.ccpm/share/settings/<p>.json` | Profile-specific settings fragment. **Source of truth** for profile-unique values. |
| `~/.ccpm/share/settings/<p>.owned.json` | Owned-keys list — keys re-asserted during sync to survive merge. |
| `~/.agents/skills/` | Anthropic-style canonical skill content (real dirs, not ccpm-managed). |

## Project scope

| Path | Role |
|---|---|
| `<repo>/.claude/settings.json` | Project settings (committed). Highest precedence. |
| `<repo>/.claude/settings.local.json` | Personal/local overrides (usually gitignored). |
| `<repo>/.claude/skills/` | Project-specific skills. |
| `<repo>/.claude/CLAUDE.md` | Project instructions for Claude Code. |
| `<repo>/CLAUDE.md` (root) | Same role; some repos use root vs `.claude/`. |

## Detection-time queries

```bash
# All host global skills (with target if symlink)
ls -la ~/.claude/skills/ | grep -v '^total\|^d'

# Profile cascade entries per profile
for p in ~/.ccpm/profiles/*/; do
  echo "=== $(basename $p) ==="
  ls -la "$p/skills/" 2>/dev/null | grep -v '^total\|^d'
done

# Manifest profiles in use
python3 -c "import json; m=json.load(open('$HOME/.ccpm/installs.json')); print(set(p for i in m['installs'] for p in i.get('profiles',[])))"

# Live profiles (from ccpm CLI)
ccpm list

# MCP servers per scope
python3 -c "import json; print('global:', list(json.load(open('$HOME/.claude.json')).get('mcpServers',{}).keys()))"
for p in ~/.ccpm/profiles/*/; do
  python3 -c "import json; print('$(basename $p):', list(json.load(open('$p/.claude.json')).get('mcpServers',{}).keys()))"
done

# Plugin caches (size proxy for unused load)
du -sh ~/.ccpm/profiles/*/plugins/cache/* 2>/dev/null | sort -h
```

## What `inventory.sh` collects

Single JSON document on stdout, structured as:

```json
{
  "host": {
    "skills": [{"name": "...", "type": "symlink|dir", "target": "..."}],
    "agents": [...],
    "commands": [...],
    "hooks": {...},
    "plugins_enabled": [...],
    "mcps": [...],
    "permissions_count": N
  },
  "ccpm": {
    "present": true,
    "profiles": ["cin", "labs", "work"],
    "share_skills": [...],
    "manifest_profiles": ["cin", "labs", "work", "rocketium"],
    "agents_storage": [...]
  },
  "projects": [
    {"path": "<repo>", "skills": [...], "plugins_enabled": [...], "permissions_count": N}
  ]
}
```

The detection scripts consume this snapshot.
