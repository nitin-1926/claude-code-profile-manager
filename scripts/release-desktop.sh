#!/usr/bin/env bash
# CCPM Desktop release script — bump, tag, and let CI build + publish the app.
#
# The desktop app versions independently of the CLI: it ships on `desktop-v*`
# tags (the CLI uses `v*` via scripts/release.sh), so a desktop-only change never
# needs a CLI/npm release. This is the desktop analogue of scripts/release.sh.
#
# Usage:
#   scripts/release-desktop.sh <patch|minor|major|X.Y.Z> [flags]
#
# Examples:
#   scripts/release-desktop.sh patch            # 0.1.0 -> 0.1.1
#   scripts/release-desktop.sh minor            # 0.1.0 -> 0.2.0
#   scripts/release-desktop.sh 0.3.0            # explicit version
#   scripts/release-desktop.sh patch --dry-run  # show the plan, change nothing
#   scripts/release-desktop.sh patch -y         # skip the confirmation prompt
#
# What it does:
#   1. Preflight: git/gh installed, on main, clean tree, in sync with origin,
#      gh authed, target tag unused.
#   2. Bumps ccpm/desktop/wails.json `info.productVersion` (Info.plist version;
#      the binary's runtime version is injected from the tag by the CI build).
#   3. Sanity-builds the desktop Go packages (skip with --skip-build).
#   4. Commits "chore: release desktop-vX.Y.Z", tags it, pushes both.
#   5. Watches the "Desktop Release" workflow and confirms the GitHub Release
#      lands with .dmg assets.
set -euo pipefail

readonly REPO_SLUG="nitin-1926/claude-code-profile-manager"
readonly DEFAULT_BRANCH="main"
readonly RELEASE_WORKFLOW="desktop-release.yml"
readonly RELEASE_WAIT_TIMEOUT=1200 # seconds (20 min; the macOS build is slow)

if [[ -t 1 ]]; then
  C_RED="$(printf '\033[31m')"; C_GREEN="$(printf '\033[32m')"; C_YELLOW="$(printf '\033[33m')"
  C_BLUE="$(printf '\033[34m')"; C_CYAN="$(printf '\033[36m')"; C_BOLD="$(printf '\033[1m')"
  C_RESET="$(printf '\033[0m')"
else
  C_RED="" C_GREEN="" C_YELLOW="" C_BLUE="" C_CYAN="" C_BOLD="" C_RESET=""
fi
step()  { printf '\n%s▸%s %s%s%s\n' "$C_BLUE" "$C_RESET" "$C_BOLD" "$1" "$C_RESET"; }
info()  { printf '  %s\n' "$1"; }
ok()    { printf '  %s✓%s %s\n' "$C_GREEN" "$C_RESET" "$1"; }
warn()  { printf '  %s!%s %s\n' "$C_YELLOW" "$C_RESET" "$1"; }
fatal() { printf '\n%s✗ %s%s\n' "$C_RED" "$1" "$C_RESET" >&2; exit 1; }
confirm() {
  [[ "$ASSUME_YES" == "1" ]] && return 0
  printf '  %s?%s %s [y/N] ' "$C_CYAN" "$C_RESET" "$1"; local a; read -r a; [[ "$a" =~ ^[Yy]$ ]]
}

usage() {
  cat <<EOF
Usage: $(basename "$0") <patch|minor|major|X.Y.Z> [flags]

Flags:
  --dry-run     Print the plan; modify nothing.
  --skip-build  Skip the local 'go build ./desktop/...' sanity check.
  -y, --yes     Skip the confirmation prompt.
  -h, --help    Show this help.
EOF
}

[[ $# -eq 0 ]] && { usage; exit 1; }
BUMP=""; DRY_RUN=0; SKIP_BUILD=0; ASSUME_YES=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --dry-run) DRY_RUN=1 ;;
    --skip-build) SKIP_BUILD=1 ;;
    -y|--yes) ASSUME_YES=1 ;;
    patch|minor|major) BUMP="$1" ;;
    [0-9]*.[0-9]*.[0-9]*) BUMP="$1" ;;
    *) fatal "unknown argument: $1" ;;
  esac
  shift
done
[[ -n "$BUMP" ]] || { usage; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"
readonly WAILS_JSON="ccpm/desktop/wails.json"

current_version() {
  node -e 'const p=require("./'"$WAILS_JSON"'"); process.stdout.write((p.info&&p.info.productVersion)||"")' 2>/dev/null \
    || fatal "could not read info.productVersion from $WAILS_JSON"
}

bump_semver() {
  local cur="$1" level="$2"
  [[ "$level" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] && { printf '%s' "$level"; return; }
  local major minor patch; IFS='.' read -r major minor patch <<<"$cur"
  case "$level" in
    patch) patch=$((patch + 1)) ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    major) major=$((major + 1)); minor=0; patch=0 ;;
    *) fatal "invalid bump level: $level" ;;
  esac
  printf '%d.%d.%d' "$major" "$minor" "$patch"
}

write_wails_version() {
  node -e '
    const fs=require("fs"), path=process.argv[1], v=process.argv[2];
    const p=JSON.parse(fs.readFileSync(path,"utf8"));
    p.info=p.info||{}; p.info.productVersion=v;
    fs.writeFileSync(path, JSON.stringify(p,null,2)+"\n");
  ' "$WAILS_JSON" "$1"
}

preflight() {
  step "Preflight checks"
  for tool in git gh node; do command -v "$tool" >/dev/null 2>&1 || fatal "$tool is not installed"; done
  ok "git, gh, node present"
  [[ -f "$WAILS_JSON" ]] || fatal "missing $WAILS_JSON"

  local branch; branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "$DEFAULT_BRANCH" ]] || fatal "on '$branch'; release from '$DEFAULT_BRANCH'"
  ok "on branch $DEFAULT_BRANCH"

  [[ -z "$(git status --porcelain)" ]] || { git status --short >&2; fatal "working tree not clean — commit or stash first"; }
  ok "working tree clean"

  info "fetching origin..."; git fetch origin --tags --quiet
  [[ "$(git rev-parse HEAD)" == "$(git rev-parse "origin/$DEFAULT_BRANCH")" ]] \
    || fatal "local $DEFAULT_BRANCH out of sync with origin — pull/push first"
  ok "in sync with origin/$DEFAULT_BRANCH"

  gh auth status >/dev/null 2>&1 || fatal "gh not authenticated — run 'gh auth login'"
  ok "gh authenticated as $(gh api user --jq '.login' 2>/dev/null || echo '?')"
}

check_tag_unused() {
  local tag="$1"
  git rev-parse "$tag" >/dev/null 2>&1 && fatal "tag $tag already exists locally"
  git ls-remote --exit-code --tags origin "$tag" >/dev/null 2>&1 && fatal "tag $tag already exists on origin"
  gh release view "$tag" >/dev/null 2>&1 && fatal "GitHub release $tag already exists"
  ok "tag $tag is unused"
}

run_build() {
  [[ "$SKIP_BUILD" == "1" ]] && { warn "skipping local build (--skip-build)"; return; }
  step "Sanity build"
  info "go build ./desktop/... (darwin)"
  ( cd ccpm && go build ./desktop/... )
  ok "desktop packages build"
}

main() {
  preflight
  local cur new tag
  cur="$(current_version)"; [[ -n "$cur" ]] || fatal "empty current version"
  new="$(bump_semver "$cur" "$BUMP")"; tag="desktop-v$new"

  step "Plan"
  info "current version : $C_BOLD$cur$C_RESET"
  info "bump            : $C_BOLD$BUMP$C_RESET"
  info "new version     : $C_BOLD$C_GREEN$new$C_RESET"
  info "tag             : $C_BOLD$tag$C_RESET"
  check_tag_unused "$tag"

  [[ "$DRY_RUN" == "1" ]] && { warn "dry run — stopping before any mutation"; exit 0; }
  confirm "proceed with desktop release $tag?" || fatal "aborted by user"

  step "Bumping version"
  write_wails_version "$new"; ok "wrote $new to $WAILS_JSON"
  run_build

  step "Committing, tagging, pushing"
  git add "$WAILS_JSON"
  git commit -m "chore: release $tag"; ok "committed"
  git tag -a "$tag" -m "CCPM Desktop $tag"; ok "tagged $tag"
  git push origin "$DEFAULT_BRANCH"; ok "pushed $DEFAULT_BRANCH"
  git push origin "$tag"; ok "pushed $tag"

  step "Waiting for the Desktop Release workflow"
  sleep 5
  local run_id
  run_id="$(gh run list --workflow="$RELEASE_WORKFLOW" --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || true)"
  if [[ -n "$run_id" ]]; then
    info "watching workflow run $run_id..."
    gh run watch "$run_id" --exit-status || fatal "release workflow failed — 'gh run view $run_id --log-failed'"
    ok "workflow succeeded"
  else
    warn "couldn't find the workflow run; polling the release endpoint"
  fi

  info "polling GitHub Release $tag (timeout ${RELEASE_WAIT_TIMEOUT}s)..."
  local elapsed=0
  while (( elapsed < RELEASE_WAIT_TIMEOUT )); do
    if gh release view "$tag" --json assets --jq '.assets[].name' 2>/dev/null | grep -q '\.dmg$'; then
      step "Done"
      ok "CCPM Desktop $tag shipped — https://github.com/$REPO_SLUG/releases/tag/$tag"
      return 0
    fi
    sleep 10; elapsed=$((elapsed + 10)); printf '.'
  done
  echo; fatal "timed out waiting for $tag release assets"
}

main "$@"
