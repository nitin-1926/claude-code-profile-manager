#!/usr/bin/env bash
# Read-only inventory of Claude Code asset scopes. Emits a single JSON document
# on stdout that the consolidate skill or `ccpm consolidate` can consume.
#
# Usage: bash inventory.sh [> /tmp/claude-inventory.json]

set -euo pipefail

HOME_DIR="${HOME}"
HOST="${HOME_DIR}/.claude"
CCPM="${HOME_DIR}/.ccpm"
AGENTS="${HOME_DIR}/.agents"

# Helper: emit a JSON list of skill entries from a directory (name, type, target).
list_skills() {
  local dir="$1"
  [ ! -d "$dir" ] && { echo "[]"; return; }
  local first=1
  echo -n "["
  for entry in "$dir"/*; do
    [ ! -e "$entry" ] && [ ! -L "$entry" ] && continue
    local name; name="$(basename "$entry")"
    [ "$name" = "_sources" ] && continue
    local type="dir"
    local target=""
    if [ -L "$entry" ]; then
      type="symlink"
      target="$(readlink "$entry")"
    fi
    [ "$first" -eq 0 ] && echo -n ","
    first=0
    printf '{"name":"%s","type":"%s","target":"%s"}' \
      "$name" "$type" "$(echo "$target" | sed 's/\\/\\\\/g; s/"/\\"/g')"
  done
  echo -n "]"
}

# Helper: count loaded plugin SKILL.md under a plugin cache root.
count_plugin_skills() {
  local cache="$1"
  [ ! -d "$cache" ] && { echo 0; return; }
  find "$cache" -name SKILL.md 2>/dev/null | wc -l | tr -d ' '
}

# Helper: emit JSON keys (sorted) of an object field in a JSON file.
json_keys() {
  local file="$1"; local field="$2"
  [ ! -f "$file" ] && { echo "[]"; return; }
  python3 - <<EOF
import json, sys
try:
    d = json.load(open("${file}"))
    keys = list(d.get("${field}", {}).keys())
    print(json.dumps(sorted(keys)))
except Exception as e:
    print("[]")
EOF
}

json_count() {
  local file="$1"; local path="$2"
  [ ! -f "$file" ] && { echo 0; return; }
  python3 - <<EOF
import json
try:
    d = json.load(open("${file}"))
    cur = d
    for k in "${path}".split("."):
        cur = cur.get(k, {}) if isinstance(cur, dict) else []
    print(len(cur))
except Exception:
    print(0)
EOF
}

echo "{"
echo "  \"host\": {"
echo -n "    \"skills\": "; list_skills "${HOST}/skills"; echo ","
echo -n "    \"agents\": "; list_skills "${HOST}/agents"; echo ","
echo -n "    \"commands\": "; list_skills "${HOST}/commands"; echo ","
echo -n "    \"plugins_enabled\": "; json_keys "${HOST}/settings.json" "enabledPlugins"; echo ","
echo -n "    \"mcps\": "; json_keys "${HOME_DIR}/.claude.json" "mcpServers"; echo ","
echo -n "    \"permissions_count\": "; json_count "${HOST}/settings.json" "permissions.allow"
echo "  },"

if [ -d "${CCPM}" ]; then
  echo "  \"ccpm\": {"
  echo "    \"present\": true,"

  # Live profile names from the profiles directory
  echo -n "    \"profiles\": ["
  first=1
  if [ -d "${CCPM}/profiles" ]; then
    for p in "${CCPM}/profiles"/*/; do
      [ ! -d "$p" ] && continue
      [ "$first" -eq 0 ] && echo -n ","
      first=0
      printf '"%s"' "$(basename "$p")"
    done
  fi
  echo "],"

  # Manifest profiles
  echo -n "    \"manifest_profiles\": "
  python3 - <<EOF
import json
try:
    m = json.load(open("${CCPM}/installs.json"))
    s = sorted({p for i in m.get("installs", []) for p in i.get("profiles", [])})
    print(json.dumps(s))
except Exception:
    print("[]")
EOF
  echo ","

  echo -n "    \"share_skills\": "; list_skills "${CCPM}/share/skills"; echo ","
  echo -n "    \"agents_storage\": "; list_skills "${AGENTS}/skills"; echo ","

  # Per-profile plugin SKILL.md counts and enabledPlugins
  echo -n "    \"profile_details\": ["
  first=1
  if [ -d "${CCPM}/profiles" ]; then
    for p in "${CCPM}/profiles"/*/; do
      [ ! -d "$p" ] && continue
      name="$(basename "$p")"
      direct=$(ls "$p/skills/" 2>/dev/null | grep -v '^_' | wc -l | tr -d ' ')
      plugins=$(count_plugin_skills "$p/plugins/cache")
      [ "$first" -eq 0 ] && echo -n ","
      first=0
      printf '{"name":"%s","direct_skills":%s,"plugin_skills":%s}' \
        "$name" "$direct" "$plugins"
    done
  fi
  echo "]"
  echo "  }"
else
  echo "  \"ccpm\": { \"present\": false }"
fi

echo "}"
