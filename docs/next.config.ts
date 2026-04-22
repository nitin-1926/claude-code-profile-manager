import type { NextConfig } from "next";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

// Read the authoritative ccpm version from the monorepo's npm/package.json —
// the same file `scripts/release.sh` bumps. Exposed as a build-time constant
// via `env`, so all `lib/version.ts` has to do is read process.env. This keeps
// Turbopack happy (no imports outside the docs project root) and means a
// version bump in the CLI release propagates into every UI surface on the
// next docs build with zero hand-edits.
const pkgPath = resolve(__dirname, "..", "npm", "package.json");
const pkg = JSON.parse(readFileSync(pkgPath, "utf-8")) as { version: string };

const nextConfig: NextConfig = {
  env: {
    NEXT_PUBLIC_CCPM_VERSION: pkg.version,
  },
};

export default nextConfig;
