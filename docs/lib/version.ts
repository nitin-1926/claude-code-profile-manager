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
