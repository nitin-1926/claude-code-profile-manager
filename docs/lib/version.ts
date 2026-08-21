// Single source of truth for the version string displayed in the docs site.
// The authoritative value lives in the monorepo's npm/package.json (bumped by
// scripts/release.sh). next.config.ts reads that file at build time and
// exposes it as NEXT_PUBLIC_CCPM_VERSION so this module only needs to read
// process.env — which keeps Turbopack happy (no imports outside docs/).
//
// The fallback is a safety net for environments that don't run our
// next.config.ts (e.g. isolated unit-test of a component). Production builds
// always have the env populated.
export const VERSION: string = process.env.NEXT_PUBLIC_CCPM_VERSION ?? "0.0.0";

// `v0.3.0` — ready to drop into UI contexts that show the full release tag.
export const VERSION_TAG = `v${VERSION}`;

const REPO_URL = "https://github.com/nitin-1926/claude-code-profile-manager";

// The desktop app (CCPM Desktop) ships on its own `desktop-v*` release tags,
// independent of the CLI's `v*` tags. Surfaced by next.config.ts from
// ccpm/desktop/wails.json. Do NOT link the desktop app via /releases/latest —
// that resolves to whichever release is newest by date, almost always a CLI
// release with no .dmg.
export const DESKTOP_VERSION: string =
  process.env.NEXT_PUBLIC_CCPM_DESKTOP_VERSION ?? "0.0.0";
export const DESKTOP_TAG = `desktop-v${DESKTOP_VERSION}`;

// Direct per-architecture .dmg download URLs for the current desktop release.
// File naming mirrors scripts/build-desktop-dmg.sh: `CCPM-<ver>-<arch>.dmg`.
export const DESKTOP_DMG = {
  appleSilicon: `${REPO_URL}/releases/download/${DESKTOP_TAG}/CCPM-${DESKTOP_VERSION}-arm64.dmg`,
  intel: `${REPO_URL}/releases/download/${DESKTOP_TAG}/CCPM-${DESKTOP_VERSION}-amd64.dmg`,
} as const;

// Desktop-scoped releases list — always resolves to desktop releases (unlike
// /releases/latest). Use as the "all downloads / older versions" link and as a
// fallback if a direct asset 404s mid-release.
export const DESKTOP_RELEASES_URL = `${REPO_URL}/releases?q=desktop-v&expanded=true`;
