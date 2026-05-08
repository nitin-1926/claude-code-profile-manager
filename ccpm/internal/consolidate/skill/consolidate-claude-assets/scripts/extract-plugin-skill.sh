#!/usr/bin/env bash
# Extract a single SKILL.md directory from an enabled plugin's cache and place
# it at host global as a standalone direct skill. Useful for keeping a small
# subset of a heavy plugin's skills while disabling the rest of the plugin.
#
# Usage:
#   bash extract-plugin-skill.sh <plugin-cache-skill-dir> [target-name]
#
#   <plugin-cache-skill-dir>   Path to a single plugin skill dir, e.g.
#                              ~/.ccpm/profiles/labs/plugins/cache/claude-plugins-official/superpowers/5.0.7/skills/dispatching-parallel-agents
#   [target-name]              Optional override for the skill name at host.
#                              Defaults to the basename of the source dir.
#
# Behavior:
#   - Refuses to overwrite if ~/.claude/skills/<name> already exists.
#   - Copies (does not symlink) so the extracted skill survives plugin updates
#     that may delete the cache.

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: bash $(basename "$0") <plugin-cache-skill-dir> [target-name]" >&2
  exit 2
fi

SRC="$1"
TARGET_NAME="${2:-$(basename "$SRC")}"
DEST="${HOME}/.claude/skills/${TARGET_NAME}"

if [ ! -d "$SRC" ]; then
  echo "Error: source not a directory: $SRC" >&2
  exit 1
fi

if [ ! -f "$SRC/SKILL.md" ]; then
  echo "Error: source has no SKILL.md (not a skill dir?): $SRC" >&2
  exit 1
fi

if [ -e "$DEST" ] || [ -L "$DEST" ]; then
  echo "Refusing to overwrite existing: $DEST" >&2
  echo "Remove or rename it first." >&2
  exit 1
fi

cp -R "$SRC" "$DEST"
echo "Extracted: $DEST (from $SRC)"
echo "Run 'ccpm sync' to cascade into all profiles."
