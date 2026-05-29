# Ask Me — Portkey + Gemini setup

The docs-site **Ask Me** assistant (`POST /api/ask`) routes through **Portkey** to a
**free Gemini** model. Portkey is used as an observability gateway (logs, analytics,
cost/latency) — not as a saved-prompt store.

## How it's wired

- **Provider:** Google Gemini via a Portkey **virtual key** (integration).
- **Model:** `gemini-2.5-flash-lite` (free tier).
- **Prompt:** owned in code (`docs/lib/ai/config.ts`), not a Portkey saved prompt.
  The request is a standard chat completion with a **system** message (rules) and a
  **user** message (`ccpm_docs_context` + `question`).
- **Grounding context:** `docs/lib/ai/ccpm-context.md`, injected into the user turn.

### Why the prompt lives in code

Gemini requires at least one user turn (`GenerateContentRequest.contents`). A Portkey
**saved prompt** that puts everything in a single `system` message renders to
system-only, so Gemini returns `400 INVALID_ARGUMENT: contents is not specified`.
Owning the system+user split in code guarantees a user turn and removes the dashboard
prompt template as a point of failure. Observability is unaffected — calls still flow
through Portkey via the virtual key.

## Environment variables

| Var                   | Required | Example          | Purpose                                  |
| --------------------- | -------- | ---------------- | ---------------------------------------- |
| `PORTKEY_API_KEY`     | yes      | `CByg…`          | Portkey workspace API key                |
| `PORTKEY_VIRTUAL_KEY` | yes      | `gemini-32cc9a`  | Portkey virtual key bound to the Gemini integration |
| `PORTKEY_MODEL`       | no       | `gemini-2.5-flash-lite` | Model override (defaults to flash-lite) |

If `PORTKEY_API_KEY` or `PORTKEY_VIRTUAL_KEY` is missing, `/api/ask` returns `503`
("Ask Me is not configured").

## Changing the system prompt

Edit `SYSTEM_PROMPT` in `docs/lib/ai/config.ts`. The user-facing facts the assistant
answers from live in `docs/lib/ai/ccpm-context.md` — keep that in sync with the README,
CLI flags, platform support, limits, and troubleshooting.

## Finding the virtual key in Portkey

Portkey dashboard → **Integrations / Virtual Keys** → the Gemini integration's slug
(e.g. `gemini-32cc9a`). Past requests are visible under **Logs**.
