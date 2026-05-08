#!/usr/bin/env bash
# Verify cascade integrity post-fix. Compares current inventory against a
# pre-fix snapshot if present at /tmp/claude-inventory.before.json, otherwise
# prints current inventory only.
#
# Usage:
#   bash verify-cascade.sh
#
# Pre-fix workflow:
#   bash inventory.sh > /tmp/claude-inventory.before.json
#   <apply fixes>
#   bash verify-cascade.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INVENTORY="${SCRIPT_DIR}/inventory.sh"
SNAP_BEFORE="/tmp/claude-inventory.before.json"
SNAP_AFTER="/tmp/claude-inventory.after.json"

if [ ! -x "$INVENTORY" ]; then
  echo "Error: missing or non-executable inventory script: $INVENTORY" >&2
  exit 1
fi

bash "$INVENTORY" > "$SNAP_AFTER"

echo "=== Current inventory: $SNAP_AFTER ==="
if command -v jq >/dev/null 2>&1; then
  jq '.host | {skills_count: (.skills|length), plugins: .plugins_enabled, mcps: .mcps}' "$SNAP_AFTER"
  if jq -e '.ccpm.present == true' "$SNAP_AFTER" >/dev/null 2>&1; then
    jq '.ccpm.profile_details' "$SNAP_AFTER"
  fi
else
  python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps({'host_skills_count': len(d['host']['skills']), 'host_plugins': d['host']['plugins_enabled'], 'mcps': d['host']['mcps']}, indent=2)); print(json.dumps(d.get('ccpm',{}).get('profile_details',[]), indent=2))" "$SNAP_AFTER"
fi

if [ -f "$SNAP_BEFORE" ]; then
  echo
  echo "=== Diff vs pre-fix snapshot ==="
  diff -u "$SNAP_BEFORE" "$SNAP_AFTER" || true
fi

# Re-run dangling/broken detection to confirm we left nothing behind
echo
echo "=== Residual issues ==="
python3 "${SCRIPT_DIR}/detect-duplicates.py" || true

echo
echo "=== Budget per profile ==="
python3 "${SCRIPT_DIR}/detect-budget.py" || true
