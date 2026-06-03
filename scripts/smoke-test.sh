#!/usr/bin/env bash
#
# ccpm on-system smoke test
# =========================
# Exercises real ccpm behavior end-to-end against the CURRENT source, on THIS
# machine, to catch regressions that unit tests can't (real filesystem, real
# rename/cascade/merge/lock behavior).
#
# SAFETY — this script does NOT touch your real setup:
#   • Everything runs inside an isolated $HOME sandbox (a temp dir). Your real
#     ~/.ccpm, ~/.claude, default account, and shell env are never modified.
#   • Test profiles use file-based OAuth credentials, so by default NOTHING is
#     written to your macOS login keychain (no keychain prompts).
#   • Login is manual and is NOT tested here (by design).
#   • The sandbox is deleted on exit (including on Ctrl+C).
#
# Opt-in extras (off by default, because they have side effects):
#   --with-keychain    also test API-key storage (writes/reads keychain entries
#                      named smk_*; cleaned up — may show a keychain prompt once)
#   --with-set-default also test `set-default` (uses `launchctl setenv` which is
#                      session-wide; cleared on exit). macOS only.
#
# Usage:
#   scripts/smoke-test.sh [--with-keychain] [--with-set-default]
#
# Exit code 0 = all checks passed; non-zero = at least one failed.

set -uo pipefail   # NOT -e: we run every check and report a summary.

WITH_KEYCHAIN=0
WITH_SET_DEFAULT=0
for arg in "$@"; do
  case "$arg" in
    --with-keychain)    WITH_KEYCHAIN=1 ;;
    --with-set-default) WITH_SET_DEFAULT=1 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $arg (try --help)"; exit 2 ;;
  esac
done

# ---------------------------------------------------------------------------
# Locate the repo and build the binary under test
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(dirname "$SCRIPT_DIR")"
CCPM_DIR="$REPO/ccpm"

command -v go >/dev/null      || { echo "FATAL: go not found"; exit 2; }
command -v python3 >/dev/null || { echo "FATAL: python3 not found"; exit 2; }
[ -d "$CCPM_DIR" ]            || { echo "FATAL: $CCPM_DIR not found"; exit 2; }

BIN_DIR="$(mktemp -d)"
BIN="$BIN_DIR/ccpm"
echo "Building ccpm from $CCPM_DIR ..."
( cd "$CCPM_DIR" && go build -o "$BIN" . ) || { echo "FATAL: build failed"; exit 2; }

# ---------------------------------------------------------------------------
# Sandbox: isolate HOME so all ccpm/Claude state lives in a throwaway dir
# ---------------------------------------------------------------------------
SANDBOX="$(mktemp -d)"
export HOME="$SANDBOX"
unset CLAUDE_CONFIG_DIR CCPM_ACTIVE_PROFILE
mkdir -p "$SANDBOX/.ccpm/profiles" "$SANDBOX/.claude"

CFG="$SANDBOX/.ccpm/config.json"
echo '{"version":"1","profiles":{}}' > "$CFG"

KC_PROFILES=()   # keychain accounts we create (for cleanup)

cleanup() {
  if [ "$WITH_SET_DEFAULT" = 1 ]; then
    launchctl unsetenv CLAUDE_CONFIG_DIR 2>/dev/null
    rm -f "$SANDBOX"/Library/LaunchAgents/*ccpm* 2>/dev/null
  fi
  if [ "$WITH_KEYCHAIN" = 1 ]; then
    for n in "${KC_PROFILES[@]:-}"; do
      [ -n "$n" ] && security delete-generic-password -s ccpm -a "$n" >/dev/null 2>&1
    done
  fi
  rm -rf "$SANDBOX" "$BIN_DIR"
}
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# Tiny test harness
# ---------------------------------------------------------------------------
P=0; F=0; FAILS=()
GRN=$'\033[32m'; RED=$'\033[31m'; YLW=$'\033[33m'; BLD=$'\033[1m'; NC=$'\033[0m'

section(){ printf "\n${BLD}== %s ==${NC}\n" "$1"; }
pass(){ printf "   ${GRN}✓${NC} %s\n" "$1"; P=$((P+1)); }
fail(){ printf "   ${RED}✗ %s${NC}\n" "$1"; F=$((F+1)); FAILS+=("$1"); [ -n "${2:-}" ] && printf "      ${YLW}↳ %s${NC}\n" "$2"; }

OUT=""; RC=0
runx(){ OUT="$("$@" 2>&1)"; RC=$?; }                       # capture stdout+stderr+rc
ok_rc(){ [ "$RC" -eq 0 ] && pass "$1" || fail "$1" "exit=$RC | ${OUT}"; }
bad_rc(){ [ "$RC" -ne 0 ] && pass "$1" || fail "$1" "expected non-zero exit"; }
has(){ case "$OUT" in *"$2"*) pass "$1";; *) fail "$1" "missing '$2'";; esac; }
exists(){ [ -e "$2" ] && pass "$1" || fail "$1" "missing: $2"; }
absent(){ [ ! -e "$2" ] && pass "$1" || fail "$1" "should be gone: $2"; }
jsonok(){ python3 -c "import json;json.load(open('$2'))" 2>/dev/null && pass "$1" || fail "$1" "invalid JSON: $2"; }

# pv <profile> -> prints "true"/"false"/"__MISSING__" from `ccpm list --json`
pv(){ "$BIN" list --json 2>/dev/null | python3 -c "import json,sys;d=json.load(sys.stdin);print(next((str(p['valid']).lower() for p in d if p['name']=='$1'),'__MISSING__'))"; }
valid_is(){ local g; g="$(pv "$2")"; [ "$g" = "$3" ] && pass "$1" || fail "$1" "$2 valid=$g want=$3"; }
# pcount -> number of profiles in config
pcount(){ python3 -c "import json;print(len(json.load(open('$CFG')).get('profiles',{})))"; }
in_cfg(){ python3 -c "import json,sys;sys.exit(0 if '$1' in json.load(open('$CFG')).get('profiles',{}) else 1)" 2>/dev/null; }

# Seed one base profile directly (file-based OAuth = no keychain), with assets.
seed_base(){
  local name="$1" dir="$SANDBOX/.ccpm/profiles/$1"
  mkdir -p "$dir/skills/seedskill" "$dir/agents"
  echo "# seed skill" > "$dir/skills/seedskill/SKILL.md"
  echo '{"accessToken":"tok","refreshToken":"ref","expiresAt":"2099-01-01T00:00:00Z"}' > "$dir/.credentials.json"
  python3 - "$name" "$dir" <<'PY'
import json,os,sys
name,d=sys.argv[1],sys.argv[2]
cfgp=os.path.expanduser("~/.ccpm/config.json")
cfg=json.load(open(cfgp))
cfg.setdefault("profiles",{})[name]={"name":name,"dir":d,"auth_method":"oauth",
  "created_at":"2026-01-01T00:00:00Z","last_used":"2026-01-01T00:00:00Z"}
json.dump(cfg,open(cfgp,"w"),indent=2)
PY
}

printf "\n${BLD}ccpm on-system smoke test${NC}\n"
printf "sandbox HOME : %s\n" "$SANDBOX"
printf "binary       : freshly built from %s\n" "$CCPM_DIR"
printf "keychain test: %s | set-default test: %s\n" \
  "$([ $WITH_KEYCHAIN = 1 ] && echo on || echo off)" \
  "$([ $WITH_SET_DEFAULT = 1 ] && echo on || echo off)"

# ---------------------------------------------------------------------------
section "1. base profile + real config writes (clone)"
# ---------------------------------------------------------------------------
seed_base smk_base
jsonok "config.json valid after seed" "$CFG"
runx "$BIN" list ;            ok_rc "ccpm list runs"
runx "$BIN" list --json ;     ok_rc "ccpm list --json runs"
has  "list --json is valid + contains base" "smk_base"

# Create more profiles via the REAL ccpm code path (clone => withConfigLock + config.Save).
runx "$BIN" clone smk_base smk_a ;            ok_rc "clone smk_base -> smk_a (copies creds)"
runx "$BIN" clone smk_base smk_b ;            ok_rc "clone smk_base -> smk_b"
runx "$BIN" clone smk_base smk_c --no-auth ;  ok_rc "clone smk_base -> smk_c (--no-auth)"
jsonok "config.json valid after 3 clones" "$CFG"
in_cfg smk_a && pass "smk_a registered in config" || fail "smk_a registered in config"

# ---------------------------------------------------------------------------
section "2. login-state detection (status / list --json)"
# ---------------------------------------------------------------------------
valid_is "smk_a reports authenticated (creds copied)"        smk_a true
valid_is "smk_c reports NOT authenticated (--no-auth)"       smk_c false
runx "$BIN" status ;  ok_rc "ccpm status runs without crashing"
runx "$BIN" auth status ; ok_rc "ccpm auth status runs"

# ---------------------------------------------------------------------------
section "3. rename round-trip (the historically risky path)"
# ---------------------------------------------------------------------------
runx "$BIN" rename smk_a smk_a_renamed ;  ok_rc "rename smk_a -> smk_a_renamed"
jsonok "config.json valid after rename" "$CFG"
in_cfg smk_a_renamed && pass "renamed profile in config" || fail "renamed profile in config"
in_cfg smk_a && fail "old name should be gone from config" || pass "old name gone from config"
exists "renamed dir on disk" "$SANDBOX/.ccpm/profiles/smk_a_renamed"
absent "old dir removed"     "$SANDBOX/.ccpm/profiles/smk_a"
valid_is "renamed profile still authenticated (creds moved)" smk_a_renamed true
# ... and back
runx "$BIN" rename smk_a_renamed smk_a ;  ok_rc "rename back smk_a_renamed -> smk_a"
in_cfg smk_a && pass "original name restored" || fail "original name restored"
absent "renamed dir gone after revert" "$SANDBOX/.ccpm/profiles/smk_a_renamed"
exists "original dir restored" "$SANDBOX/.ccpm/profiles/smk_a"
valid_is "restored profile still authenticated" smk_a true

# ---------------------------------------------------------------------------
section "4. host-asset cascade + settings merge (ccpm sync)"
# ---------------------------------------------------------------------------
mkdir -p "$SANDBOX/.claude/skills/hostskill"
echo "# host skill" > "$SANDBOX/.claude/skills/hostskill/SKILL.md"
echo '{"model":"claude-smoke-test"}' > "$SANDBOX/.claude/settings.json"
runx "$BIN" sync --all ;  ok_rc "ccpm sync --all runs"
exists "host skill cascaded into smk_a"  "$SANDBOX/.ccpm/profiles/smk_a/skills/hostskill"
exists "host skill cascaded into smk_b"  "$SANDBOX/.ccpm/profiles/smk_b/skills/hostskill"
runx cat "$SANDBOX/.ccpm/profiles/smk_a/settings.json"
has  "host settings merged into smk_a/settings.json" "claude-smoke-test"

# ---------------------------------------------------------------------------
section "5. export / import-bundle round-trip"
# ---------------------------------------------------------------------------
BUNDLE="$SANDBOX/smk_a.tar.gz"
runx "$BIN" export smk_a -o "$BUNDLE" ;  ok_rc "export smk_a"
exists "bundle written" "$BUNDLE"
if tar tzf "$BUNDLE" 2>/dev/null | grep -q '\.credentials\.json'; then
  fail "bundle must NOT contain credentials"
else
  pass "bundle excludes credentials (default)"
fi
runx "$BIN" import-bundle "$BUNDLE" --profile smk_restored ;  ok_rc "import-bundle -> smk_restored"
exists "restored profile has its skills" "$SANDBOX/.ccpm/profiles/smk_restored/skills"
absent "restored profile has NO credentials" "$SANDBOX/.ccpm/profiles/smk_restored/.credentials.json"

# ---------------------------------------------------------------------------
section "6. doctor + doctor --fix (dangling symlink prune)"
# ---------------------------------------------------------------------------
runx "$BIN" doctor ;  has "doctor runs and prints sections" "Environment"
BROKEN="$SANDBOX/.ccpm/profiles/smk_b/skills/zz_broken"
ln -s "$SANDBOX/does_not_exist_target" "$BROKEN"
runx "$BIN" doctor ;        has "doctor flags the dangling symlink" "broken symlink"
# Note: `doctor` exits non-zero when it finds issues (e.g. unauthenticated test
# profiles), so assert on the prune action, not the exit code.
runx "$BIN" doctor --fix ;  has "doctor --fix runs and prunes" "pruned"
absent "doctor --fix pruned the dangling symlink" "$BROKEN"

# ---------------------------------------------------------------------------
section "7. concurrency / advisory lock (parallel mutations)"
# ---------------------------------------------------------------------------
before="$(pcount)"
"$BIN" clone smk_b smk_par1 --no-auth >/dev/null 2>&1 &
"$BIN" clone smk_b smk_par2 --no-auth >/dev/null 2>&1 &
"$BIN" clone smk_b smk_par3 --no-auth >/dev/null 2>&1 &
"$BIN" clone smk_b smk_par4 --no-auth >/dev/null 2>&1 &
wait
jsonok "config.json valid after 4 concurrent clones" "$CFG"
after="$(pcount)"
if [ "$after" = "$((before+4))" ]; then
  pass "no lost updates under concurrency ($before -> $after, +4)"
else
  fail "lost update under concurrency" "before=$before after=$after (expected +4)"
fi
in_cfg smk_b && pass "lock source profile intact after race" || fail "lock source profile intact after race"

# ---------------------------------------------------------------------------
section "8. consolidate (read-only audit)"
# ---------------------------------------------------------------------------
runx "$BIN" consolidate ;  ok_rc "ccpm consolidate (read-only) runs"

# ---------------------------------------------------------------------------
section "9. remove (cleanup path)"
# ---------------------------------------------------------------------------
runx "$BIN" remove smk_c --force ;  ok_rc "remove smk_c --force"
in_cfg smk_c && fail "removed profile should be gone from config" || pass "removed profile gone from config"
absent "removed profile dir deleted" "$SANDBOX/.ccpm/profiles/smk_c"

# ---------------------------------------------------------------------------
section "10. API-key masking + panic guard  (--with-keychain)"
# ---------------------------------------------------------------------------
if [ "$WITH_KEYCHAIN" = 1 ]; then
  # Seed an api_key-type profile so checkAPIKey (the masking/panic path) runs.
  mkdir -p "$SANDBOX/.ccpm/profiles/smk_kc"
  python3 - <<'PY'
import json,os
p=os.path.expanduser("~/.ccpm/config.json"); c=json.load(open(p))
c["profiles"]["smk_kc"]={"name":"smk_kc","dir":os.path.expanduser("~/.ccpm/profiles/smk_kc"),
  "auth_method":"api_key","created_at":"x","last_used":"x"}
json.dump(c,open(p,"w"),indent=2)
PY
  KC_PROFILES+=("smk_kc")   # auth refresh stores under keychain service "ccpm", account=smk_kc
  # A deliberately short key must be REJECTED at input (regression guard), not stored/panic.
  o="$(printf 'shortkey\n' | "$BIN" auth refresh smk_kc 2>&1)"; r=$?
  [ $r -ne 0 ] && pass "short API key rejected at input (no crash)" || fail "short API key rejected" "exit=$r: $o"
  # A normal-length key stores fine.
  o="$(printf 'sk-ant-smoketest-0123456789abcdef\n' | "$BIN" auth refresh smk_kc 2>&1)"; r=$?
  [ $r -eq 0 ] && pass "valid API key stored" || fail "valid API key stored" "exit=$r: $o"
  # status / auth status must read + MASK the key without panicking.
  runx "$BIN" status ;             has "status renders api-key profile without panic" "smk_kc"
  runx "$BIN" auth status smk_kc ; has "API key shown masked (contains ...)" "..."
else
  printf "   ${YLW}• skipped (pass --with-keychain to run; touches the login keychain)${NC}\n"
fi

# ---------------------------------------------------------------------------
section "11. set-default / unset-default  (--with-set-default)"
# ---------------------------------------------------------------------------
if [ "$WITH_SET_DEFAULT" = 1 ]; then
  runx "$BIN" set-default smk_a ;  ok_rc "set-default smk_a"
  def="$(python3 -c "import json;print(json.load(open('$CFG')).get('default_profile',''))")"
  [ "$def" = "smk_a" ] && pass "config default_profile = smk_a" || fail "config default_profile" "got '$def'"
  runx "$BIN" unset-default ;  ok_rc "unset-default"
  def2="$(python3 -c "import json;print(json.load(open('$CFG')).get('default_profile',''))")"
  [ -z "$def2" ] && pass "default cleared" || fail "default cleared" "got '$def2'"
else
  printf "   ${YLW}• skipped (pass --with-set-default to run; uses session-wide launchctl)${NC}\n"
fi

# ---------------------------------------------------------------------------
section "12. final integrity"
# ---------------------------------------------------------------------------
jsonok "config.json is still valid JSON at the end" "$CFG"
runx "$BIN" list ;  ok_rc "ccpm list still works at the end"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
printf "\n${BLD}──────────────────────────────────────────────${NC}\n"
printf "${BLD}Result:${NC} ${GRN}%d passed${NC}, " "$P"
if [ "$F" -eq 0 ]; then
  printf "${GRN}0 failed${NC}\n"
  printf "${GRN}ALL CHECKS PASSED — safe to publish.${NC}\n"
  exit 0
else
  printf "${RED}%d failed${NC}\n" "$F"
  printf "${RED}Failures:${NC}\n"
  for f in "${FAILS[@]}"; do printf "  ${RED}• %s${NC}\n" "$f"; done
  exit 1
fi
