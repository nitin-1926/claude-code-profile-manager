#!/usr/bin/env bash
# Tarball ~/.claude and ~/.ccpm before any destructive consolidation step.
# Prints the resulting backup path on stdout.
#
# Usage:
#   bash backup.sh                  # writes ~/.claude-ccpm-backup-<ts>.tgz
#   BACKUP_DIR=/tmp bash backup.sh  # custom destination

set -euo pipefail

DEST_DIR="${BACKUP_DIR:-${HOME}}"
TS="$(date +%Y%m%d-%H%M%S)"
OUT="${DEST_DIR}/.claude-ccpm-backup-${TS}.tgz"

# Build tar arguments only for paths that exist; tar fails hard if any are missing.
ARGS=()
[ -d "${HOME}/.claude" ] && ARGS+=(.claude)
[ -d "${HOME}/.ccpm" ] && ARGS+=(.ccpm)
[ -d "${HOME}/.agents" ] && ARGS+=(.agents)

if [ ${#ARGS[@]} -eq 0 ]; then
  echo "No Claude Code asset directories present (~/.claude, ~/.ccpm, ~/.agents). Nothing to back up." >&2
  exit 1
fi

tar -czf "$OUT" -C "$HOME" "${ARGS[@]}" 2>/dev/null
echo "$OUT"
