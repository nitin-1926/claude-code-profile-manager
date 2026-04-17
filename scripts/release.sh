#!/usr/bin/env bash
# ccpm release script — end-to-end version bump, tag, GitHub release, npm publish.
#
# Usage:
#   scripts/release.sh <patch|minor|major|X.Y.Z> [flags]
#
# Examples:
#   scripts/release.sh patch             # 0.1.0 -> 0.1.1
#   scripts/release.sh minor             # 0.1.0 -> 0.2.0
#   scripts/release.sh major             # 0.1.0 -> 1.0.0
#   scripts/release.sh 0.3.0             # explicit version
#   scripts/release.sh patch --dry-run   # show what would happen, change nothing
#   scripts/release.sh patch --skip-npm  # GitHub release only (no npm publish)
#   scripts/release.sh patch --skip-tests
#   scripts/release.sh patch -y          # skip confirmation prompt
#
# What this does (in order):
#   1. Preflight: verify git, go, node, npm, gh are installed; working tree clean;
#      on main; up to date with origin; logged in to npm with publish access;
#      logged in to gh; target tag doesn't already exist.
#   2. Bumps cli/cmd/root.go and npm/package.json to the new version.
#   3. Runs go build (host), GOOS=windows go build, go test ./..., docs tsc.
#   4. Commits "chore: release vX.Y.Z", tags vX.Y.Z, pushes both.
#   5. Waits for the GitHub "Release" workflow (goreleaser) to finish and
#      the GitHub Release to be published.
#   6. Runs `npm publish --access public` from npm/.
#
# Any failure aborts immediately. If a failure happens AFTER the commit+tag+push,
# the script tells you exactly which step to resume from manually.

set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────────────────────

readonly REPO_SLUG="nitin-1926/claude-code-profile-manager"
readonly NPM_PACKAGE="@ngcodes/ccpm"
readonly DEFAULT_BRANCH="main"
readonly RELEASE_WORKFLOW="release.yml"
readonly RELEASE_WAIT_TIMEOUT=900  # seconds (15 min)

# ─────────────────────────────────────────────────────────────────────────────
# UI helpers
# ─────────────────────────────────────────────────────────────────────────────

if [[ -t 1 ]]; then
  C_RED="$(printf '\033[31m')"
  C_GREEN="$(printf '\033[32m')"
  C_YELLOW="$(printf '\033[33m')"
  C_BLUE="$(printf '\033[34m')"
  C_CYAN="$(printf '\033[36m')"
  C_BOLD="$(printf '\033[1m')"
  C_DIM="$(printf '\033[2m')"
  C_RESET="$(printf '\033[0m')"
else
  C_RED="" C_GREEN="" C_YELLOW="" C_BLUE="" C_CYAN="" C_BOLD="" C_DIM="" C_RESET=""
fi

step()   { printf '\n%s▸%s %s%s%s\n' "$C_BLUE" "$C_RESET" "$C_BOLD" "$1" "$C_RESET"; }
info()   { printf '  %s\n' "$1"; }
ok()     { printf '  %s✓%s %s\n' "$C_GREEN" "$C_RESET" "$1"; }
warn()   { printf '  %s!%s %s\n' "$C_YELLOW" "$C_RESET" "$1"; }
fatal()  { printf '\n%s✗ %s%s\n' "$C_RED" "$1" "$C_RESET" >&2; exit 1; }
confirm() {
  if [[ "$ASSUME_YES" == "1" ]]; then return 0; fi
  printf '  %s?%s %s [y/N] ' "$C_CYAN" "$C_RESET" "$1"
  local ans; read -r ans
  [[ "$ans" =~ ^[Yy]$ ]]
}

# ─────────────────────────────────────────────────────────────────────────────
# Arg parsing
# ─────────────────────────────────────────────────────────────────────────────

usage() {
  cat <<EOF
Usage: $(basename "$0") <patch|minor|major|X.Y.Z> [flags]

Flags:
  --dry-run       Print planned actions. Does not modify files, commit, tag, or publish.
  --skip-tests    Skip the go build / go test / docs type-check verification.
  --skip-npm      Do GitHub release only, skip the npm publish step.
  --stash         Auto-stash uncommitted changes for the duration of the release
                  and pop them back when done (even on failure). Lets you release
                  only the already-committed subset of your work.
  --allow-dirty   Release with a dirty tree as-is (NOT recommended — your
                  uncommitted changes will NOT be in the tag / binary / npm package).
  -y, --yes       Skip the confirmation prompt before committing and pushing.
  -h, --help      Show this help.
EOF
}

if [[ $# -eq 0 ]]; then
  usage; exit 1
fi

BUMP=""
DRY_RUN=0
SKIP_TESTS=0
SKIP_NPM=0
ALLOW_DIRTY=0
AUTO_STASH=0
ASSUME_YES=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --dry-run) DRY_RUN=1 ;;
    --skip-tests) SKIP_TESTS=1 ;;
    --skip-npm) SKIP_NPM=1 ;;
    --stash) AUTO_STASH=1 ;;
    --allow-dirty) ALLOW_DIRTY=1 ;;
    -y|--yes) ASSUME_YES=1 ;;
    patch|minor|major) BUMP="$1" ;;
    [0-9]*.[0-9]*.[0-9]*) BUMP="$1" ;;
    *) fatal "unknown argument: $1" ;;
  esac
  shift
done

[[ -n "$BUMP" ]] || { usage; exit 1; }

if [[ "$AUTO_STASH" == "1" ]] && [[ "$ALLOW_DIRTY" == "1" ]]; then
  fatal "--stash and --allow-dirty are mutually exclusive"
fi

# ─────────────────────────────────────────────────────────────────────────────
# Repo root + paths
# ─────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

readonly GO_VERSION_FILE="cli/cmd/root.go"
readonly NPM_PKG_FILE="npm/package.json"
readonly STASH_LABEL="ccpm-release-autostash-$$"

# ─────────────────────────────────────────────────────────────────────────────
# Auto-stash helpers (only active when --stash is passed)
# ─────────────────────────────────────────────────────────────────────────────

STASH_PUSHED=0

pop_stash_on_exit() {
  local exit_code=$?
  if [[ "$STASH_PUSHED" != "1" ]]; then
    exit "$exit_code"
  fi
  echo
  step "Restoring stashed changes"
  # Find our stash by the unique label and pop it. Use --index to preserve the
  # original staged/unstaged split.
  local stash_ref
  stash_ref="$(git stash list --format='%gd %s' | awk -v label="$STASH_LABEL" '$0 ~ label {print $1; exit}')"
  if [[ -z "$stash_ref" ]]; then
    warn "could not find the auto-stash ($STASH_LABEL) — did you pop it manually?"
    exit "$exit_code"
  fi
  if git stash pop --index "$stash_ref" 2>&1; then
    ok "restored working tree from $stash_ref"
  else
    warn "stash pop had conflicts — your work is safe in $stash_ref; resolve manually with 'git stash pop'"
  fi
  exit "$exit_code"
}

# ─────────────────────────────────────────────────────────────────────────────
# Version helpers
# ─────────────────────────────────────────────────────────────────────────────

current_version() {
  # Extract: version = "X.Y.Z"
  local v
  v="$(grep -E '^[[:space:]]*version[[:space:]]*=[[:space:]]*"[^"]+"' "$GO_VERSION_FILE" | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$v" ]] || fatal "could not read current version from $GO_VERSION_FILE"
  printf '%s' "$v"
}

bump_semver() {
  local cur="$1" level="$2"
  if [[ "$level" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf '%s' "$level"
    return
  fi
  local major minor patch
  IFS='.' read -r major minor patch <<<"$cur"
  case "$level" in
    patch) patch=$((patch + 1)) ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    major) major=$((major + 1)); minor=0; patch=0 ;;
    *) fatal "invalid bump level: $level" ;;
  esac
  printf '%d.%d.%d' "$major" "$minor" "$patch"
}

write_go_version() {
  local new="$1"
  # Portable in-place sed for macOS + Linux.
  local tmp
  tmp="$(mktemp)"
  sed -E 's/^([[:space:]]*version[[:space:]]*=[[:space:]]*)"[^"]+"/\1"'"$new"'"/' "$GO_VERSION_FILE" >"$tmp"
  mv "$tmp" "$GO_VERSION_FILE"
}

write_npm_version() {
  local new="$1"
  # Use node so we don't risk touching other keys that happen to match a regex.
  node -e '
    const fs = require("fs");
    const path = process.argv[1];
    const newV = process.argv[2];
    const pkg = JSON.parse(fs.readFileSync(path, "utf8"));
    pkg.version = newV;
    fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + "\n");
  ' "$NPM_PKG_FILE" "$new"
}

# ─────────────────────────────────────────────────────────────────────────────
# Preflight
# ─────────────────────────────────────────────────────────────────────────────

preflight() {
  step "Preflight checks"

  # Tooling
  for tool in git go node npm gh; do
    command -v "$tool" >/dev/null 2>&1 || fatal "$tool is not installed or not on PATH"
  done
  ok "git, go, node, npm, gh are installed"

  # Git repo
  [[ -d .git ]] || fatal "not a git repo (did you run from the right place?)"
  ok "inside a git repo ($REPO_ROOT)"

  # Branch
  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  if [[ "$branch" != "$DEFAULT_BRANCH" ]]; then
    fatal "currently on '$branch'; release must be cut from '$DEFAULT_BRANCH'"
  fi
  ok "on branch $DEFAULT_BRANCH"

  # Working tree clean
  if [[ -n "$(git status --porcelain)" ]]; then
    if [[ "$AUTO_STASH" == "1" ]]; then
      info "working tree is dirty; stashing for the release (will pop on exit)..."
      # --include-untracked so new files (e.g. .claude/launch.json) also get saved.
      if ! git stash push --include-untracked --message "$STASH_LABEL" >/dev/null; then
        fatal "git stash push failed; aborting"
      fi
      STASH_PUSHED=1
      trap pop_stash_on_exit EXIT
      ok "stashed uncommitted changes as \"$STASH_LABEL\" (will pop on exit)"
      # Double-check the tree is actually clean now; if not, something is off.
      if [[ -n "$(git status --porcelain)" ]]; then
        fatal "working tree still dirty after stash — refusing to proceed"
      fi
    elif [[ "$ALLOW_DIRTY" == "1" ]]; then
      warn "working tree is dirty; --allow-dirty set (uncommitted changes will NOT be in the release)"
    else
      git status --short >&2
      fatal "working tree is not clean — commit what you want to release, then run with --stash to set the rest aside (or --allow-dirty at your own risk)"
    fi
  else
    ok "working tree is clean"
  fi

  # Up to date with origin
  info "fetching origin..."
  git fetch origin --tags --quiet
  local local_sha remote_sha
  local_sha="$(git rev-parse HEAD)"
  remote_sha="$(git rev-parse "origin/$DEFAULT_BRANCH")"
  if [[ "$local_sha" != "$remote_sha" ]]; then
    fatal "local $DEFAULT_BRANCH ($local_sha) is not in sync with origin/$DEFAULT_BRANCH ($remote_sha); pull/push first"
  fi
  ok "in sync with origin/$DEFAULT_BRANCH"

  # gh auth
  if ! gh auth status >/dev/null 2>&1; then
    fatal "gh CLI is not authenticated — run 'gh auth login'"
  fi
  ok "gh CLI is authenticated as $(gh api user --jq '.login' 2>/dev/null || echo '?')"

  # npm auth
  local npm_user
  if ! npm_user="$(npm whoami 2>/dev/null)"; then
    fatal "not logged in to npm — run 'npm login'"
  fi
  ok "npm logged in as $npm_user"

  # npm publish access (best-effort; real publish will still fail hard if wrong)
  if npm access list packages "$(cut -d/ -f1 <<<"$NPM_PACKAGE")" 2>/dev/null | grep -q "^$NPM_PACKAGE\b"; then
    ok "npm user has access to $NPM_PACKAGE"
  else
    warn "could not confirm publish access to $NPM_PACKAGE via 'npm access list'; continuing (publish will fail loudly if you don't have it)"
  fi

  # Binary tools for verification
  ok "go $(go version | awk '{print $3}')"
  ok "node $(node --version)"

  # Version files exist
  [[ -f "$GO_VERSION_FILE" ]] || fatal "missing $GO_VERSION_FILE"
  [[ -f "$NPM_PKG_FILE"    ]] || fatal "missing $NPM_PKG_FILE"
  ok "version files present"
}

check_version_mismatch() {
  local go_v npm_v
  go_v="$(current_version)"
  npm_v="$(node -e 'console.log(require("./'"$NPM_PKG_FILE"'").version)')"
  if [[ "$go_v" != "$npm_v" ]]; then
    warn "version mismatch: Go says $go_v, npm says $npm_v — using Go as source of truth"
  fi
}

check_tag_unused() {
  local tag="$1"
  if git rev-parse "$tag" >/dev/null 2>&1; then
    fatal "tag $tag already exists locally"
  fi
  if git ls-remote --exit-code --tags origin "$tag" >/dev/null 2>&1; then
    fatal "tag $tag already exists on origin"
  fi
  if gh release view "$tag" >/dev/null 2>&1; then
    fatal "GitHub release $tag already exists"
  fi
  ok "tag $tag is unused locally, on origin, and on GitHub releases"
}

# ─────────────────────────────────────────────────────────────────────────────
# Verification
# ─────────────────────────────────────────────────────────────────────────────

run_verification() {
  if [[ "$SKIP_TESTS" == "1" ]]; then
    warn "skipping verification (--skip-tests)"
    return
  fi

  step "Verification"

  info "go build ./... (host)"
  ( cd cli && go build ./... )
  ok "host build green"

  info "GOOS=windows go build ./..."
  ( cd cli && GOOS=windows GOARCH=amd64 go build ./... )
  ok "Windows cross-compile green"

  info "GOOS=linux go build ./..."
  ( cd cli && GOOS=linux GOARCH=amd64 go build ./... )
  ok "Linux cross-compile green"

  info "go test ./..."
  ( cd cli && go test ./... )
  ok "tests green"

  if [[ -f docs/package.json ]]; then
    info "docs: npx tsc --noEmit (no Next.js build, just type-check)"
    if ( cd docs && npx --no-install tsc --noEmit ) 2>/dev/null; then
      ok "docs type-check green"
    else
      warn "docs tsc failed or tsc not installed; skipping"
    fi
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Commit, tag, push
# ─────────────────────────────────────────────────────────────────────────────

commit_tag_push() {
  local new="$1"
  local tag="v$new"

  step "Committing, tagging, and pushing"

  git add "$GO_VERSION_FILE" "$NPM_PKG_FILE"
  git commit -m "chore: release $tag"
  ok "committed"

  git tag -a "$tag" -m "ccpm $tag"
  ok "tagged $tag"

  git push origin "$DEFAULT_BRANCH"
  ok "pushed $DEFAULT_BRANCH"

  git push origin "$tag"
  ok "pushed $tag"
}

# ─────────────────────────────────────────────────────────────────────────────
# Wait for GitHub Release
# ─────────────────────────────────────────────────────────────────────────────

wait_for_github_release() {
  local new="$1"
  local tag="v$new"

  step "Waiting for GoReleaser workflow + GitHub Release"
  info "watching workflow '$RELEASE_WORKFLOW' triggered by tag $tag"

  # Give the workflow a moment to appear.
  sleep 5

  # Find the most recent run for this workflow and watch it.
  local run_id
  run_id="$(gh run list --workflow="$RELEASE_WORKFLOW" --limit 1 --json databaseId,headBranch,event,status,name --jq '.[0].databaseId' 2>/dev/null || true)"
  if [[ -n "$run_id" ]]; then
    info "found workflow run $run_id — streaming..."
    if ! gh run watch "$run_id" --exit-status; then
      fatal "release workflow failed — inspect with 'gh run view $run_id --log-failed'"
    fi
    ok "workflow succeeded"
  else
    warn "couldn't locate the workflow run via gh; falling back to polling the release endpoint"
  fi

  # Verify the release + at least one asset actually landed.
  info "polling GitHub Release $tag (timeout: ${RELEASE_WAIT_TIMEOUT}s)..."
  local elapsed=0
  while (( elapsed < RELEASE_WAIT_TIMEOUT )); do
    if gh release view "$tag" --json assets --jq '.assets[].name' 2>/dev/null | grep -q '^ccpm_darwin_arm64'; then
      ok "GitHub Release $tag is published with binaries"
      return 0
    fi
    sleep 10
    elapsed=$((elapsed + 10))
    printf '.'
  done
  echo
  fatal "timed out waiting for $tag release assets on GitHub"
}

# ─────────────────────────────────────────────────────────────────────────────
# npm publish
# ─────────────────────────────────────────────────────────────────────────────

publish_npm() {
  local new="$1"
  if [[ "$SKIP_NPM" == "1" ]]; then
    warn "skipping npm publish (--skip-npm)"
    info "to publish later: cd npm && npm publish --access public"
    return
  fi

  step "Publishing to npm"
  ( cd npm && npm publish --access public )
  ok "published $NPM_PACKAGE@$new"

  # Verify it's actually pickup-able.
  info "verifying with 'npm view'..."
  sleep 3
  local shown
  shown="$(npm view "$NPM_PACKAGE@$new" version 2>/dev/null || true)"
  if [[ "$shown" == "$new" ]]; then
    ok "npm confirms $NPM_PACKAGE@$new is live"
  else
    warn "npm view did not return the new version yet (registry is probably catching up)"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

main() {
  preflight
  check_version_mismatch

  local cur new tag
  cur="$(current_version)"
  new="$(bump_semver "$cur" "$BUMP")"
  tag="v$new"

  step "Plan"
  info "current version : $C_BOLD$cur$C_RESET"
  info "bump            : $C_BOLD$BUMP$C_RESET"
  info "new version     : $C_BOLD$C_GREEN$new$C_RESET"
  info "tag             : $C_BOLD$tag$C_RESET"
  info "skip tests      : $( [[ $SKIP_TESTS == 1 ]] && echo yes || echo no )"
  info "skip npm publish: $( [[ $SKIP_NPM   == 1 ]] && echo yes || echo no )"
  info "dry run         : $( [[ $DRY_RUN    == 1 ]] && echo yes || echo no )"

  check_tag_unused "$tag"

  if [[ "$DRY_RUN" == "1" ]]; then
    warn "dry run — stopping before any mutation"
    exit 0
  fi

  confirm "proceed with release $tag?" || fatal "aborted by user"

  step "Bumping version"
  write_go_version "$new"
  write_npm_version "$new"
  ok "wrote $new to $GO_VERSION_FILE"
  ok "wrote $new to $NPM_PKG_FILE"

  run_verification

  commit_tag_push "$new"
  wait_for_github_release "$new"
  publish_npm "$new"

  step "Done"
  ok "ccpm $tag shipped — GitHub release: https://github.com/$REPO_SLUG/releases/tag/$tag"
  [[ "$SKIP_NPM" == "0" ]] && ok "npm: https://www.npmjs.com/package/$NPM_PACKAGE/v/$new"
}

main "$@"
