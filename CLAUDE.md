<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **claude-code-profile-manager** (2518 symbols, 8098 relationships, 210 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/claude-code-profile-manager/context` | Codebase overview, check index freshness |
| `gitnexus://repo/claude-code-profile-manager/clusters` | All functional areas |
| `gitnexus://repo/claude-code-profile-manager/processes` | All execution flows |
| `gitnexus://repo/claude-code-profile-manager/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

# Devlog (`SUMMARY.md`) — non-negotiable

`SUMMARY.md` is the maintainer's **local-only devlog**. It is listed in `.gitignore` and is **never committed, staged, or pushed** — `git status` will not show it, and that is correct. Treat it as a personal append-only log that lives only on this machine.

Every substantive change to this repo (bug fix, feature, build/CI change, refactor with observable behavior, docs that change facts) MUST land with a new entry at the top of the `## Log` section in `SUMMARY.md`. The canonical rule and full template live in `AGENTS.md` (search "SUMMARY.md") and at the top of `SUMMARY.md` itself; this section exists so the rule is reliably loaded into every Claude Code session, since `CLAUDE.md` is the file Claude Code is guaranteed to read.

Hard rules:

- Write the entry **in the same session as the change**, before you declare the task done — not as a separate follow-up turn or "I'll log it next time."
- Do **not** `git add SUMMARY.md`, do not include it in any commit, do not mention it in PR descriptions. It is intentionally untracked.
- One entry per logically independent change. Do not batch unrelated work into one entry.
- Entries go at the **top** of `## Log` (reverse chronological).
- Use the entry template defined in `SUMMARY.md` (Type / Scope / Reasoning / Implementation summary, plus `Follow-ups deferred` when applicable). Don't invent a new shape.
- Only skip the log for purely cosmetic edits (whitespace, typo, doc link rename). When in doubt, write the entry.

Before you say a task is done, do this self-check: if any tracked file under `ccpm/**`, `scripts/**`, `docs/**`, or top-level docs has been modified in this session and you have not appended a `SUMMARY.md` entry covering it, you are not done. Append the entry first.

## Ask Me context (`docs/lib/ai/ccpm-context.md`)

The docs site’s **Ask Me** feature grounds answers in `docs/lib/ai/ccpm-context.md` (loaded server-side for `POST /api/ask`). Whenever you change **user-facing** behavior or facts — CLI commands/flags, platform support, limitations, troubleshooting, README or on-site docs that users rely on — update that file in the **same change** so the assistant does not drift. Portkey template variables and paste-instructions live in `docs/lib/ai/PORTKEY_PROMPT.md`.
