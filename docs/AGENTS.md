<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.

<!-- END:nextjs-agent-rules -->

## ccpm docs site — maintainer rules

- **Ask Me context:** User-visible facts for the site’s Ask Me assistant are curated in `docs/lib/ai/ccpm-context.md`. Any change to commands, platform behavior, limits, troubleshooting, or user-facing docs that alters what a reasonable user would ask about MUST update that file in the same change. See `docs/lib/ai/PORTKEY_PROMPT.md` for Portkey variable names and paste-instructions.
- **Changelog:** Curated public entries live in `docs/lib/changelog.ts` (site `/changelog` and README teasers). Add or adjust entries when shipping user-visible fixes or features; keep tone user-facing (no internal postmortem framing).

For repository-wide ccpm invariants and `SUMMARY.md` rules, see **`../AGENTS.md`** (repo root).
