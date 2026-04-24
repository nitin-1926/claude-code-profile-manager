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

// Security headers. CSP is the main line of defence against a future XSS
// regression; the others close off clickjacking / MIME-sniffing / referrer
// leaks / cross-origin probing that Next.js does not set by default. The
// site ships one inline script (theme init in app/layout.tsx) and uses
// shiki-rendered inline styles, so 'unsafe-inline' is kept for script-src
// and style-src — tightening to nonces would require refactoring layout.tsx
// to a nonce pattern, which is out of scope for the security review pass.
// Development only: React uses eval() for dev tooling; production does not.
const isDev = process.env.NODE_ENV === "development";
const contentSecurityPolicy = [
  "default-src 'self'",
  isDev
    ? "script-src 'self' 'unsafe-inline' 'unsafe-eval'"
    : "script-src 'self' 'unsafe-inline'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: https:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "form-action 'self'",
