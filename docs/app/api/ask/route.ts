import { NextRequest } from "next/server";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import Portkey from "portkey-ai";
import {
  ASK_MODEL,
  SYSTEM_PROMPT,
  buildUserMessage,
} from "@/lib/ai/config";
import { sanitizeQuestion } from "@/lib/ai/sanitize";
import { checkRateLimit } from "@/lib/ai/rate-limit";

export const runtime = "nodejs";

const MAX_CONTEXT_BYTES = 64 * 1024;

let cachedContext: string | null = null;

function loadContext(): string {
  if (cachedContext) return cachedContext;
  const path = join(process.cwd(), "lib", "ai", "ccpm-context.md");
  let raw = readFileSync(path, "utf-8");
  if (Buffer.byteLength(raw, "utf-8") > MAX_CONTEXT_BYTES) {
    console.warn(
      `[ask] ccpm-context.md exceeds ${MAX_CONTEXT_BYTES} bytes; truncating.`,
    );
    raw = Buffer.from(raw, "utf-8")
      .subarray(0, MAX_CONTEXT_BYTES)
      .toString("utf-8");
  }
  cachedContext = raw;
  return cachedContext;
}

function isSameOrigin(req: NextRequest): boolean {
  const origin = req.headers.get("origin");
  const referer = req.headers.get("referer");
  const self = req.nextUrl.origin;
  if (origin) return origin === self;
  if (referer) {
    try {
      return new URL(referer).origin === self;
    } catch {
      return false;
    }
  }
  return false;
}

// Cap output so a single completion can't run away on tokens (and cost).
const MAX_OUTPUT_TOKENS = 1024;

// Cap the request body explicitly (the question is ≤500 chars; 8 KiB leaves
// generous JSON overhead) instead of relying on framework defaults.
const MAX_BODY_BYTES = 8 * 1024;

export async function POST(req: NextRequest) {
  // The same-origin and rate-limit checks below are best-effort
  // defense-in-depth — both are spoofable/evadable. The real backstop against
  // cost abuse is a provider-side spend cap on the Portkey virtual key.
  if (!isSameOrigin(req)) {
    return new Response(JSON.stringify({ error: "Forbidden" }), {
      status: 403,
      headers: { "Content-Type": "application/json" },
    });
  }

  const contentLength = Number(req.headers.get("content-length") ?? 0);
  if (contentLength > MAX_BODY_BYTES) {
    return new Response(JSON.stringify({ error: "Request too large" }), {
      status: 413,
      headers: { "Content-Type": "application/json" },
    });
  }

  let body: unknown;
  try {
    const text = await req.text();
    // content-length can lie (or be absent on chunked bodies) — enforce on
    // the actual bytes read too.
    if (Buffer.byteLength(text, "utf-8") > MAX_BODY_BYTES) {
      return new Response(JSON.stringify({ error: "Request too large" }), {
        status: 413,
        headers: { "Content-Type": "application/json" },
      });
    }
    body = JSON.parse(text);
  } catch {
    return new Response(JSON.stringify({ error: "Invalid JSON" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  const rawQ =
    typeof body === "object" &&
    body !== null &&
    "question" in body &&
    typeof (body as { question: unknown }).question === "string"
      ? (body as { question: string }).question
      : undefined;

  const sanitized = sanitizeQuestion(rawQ);
  if (!sanitized.ok) {
    return new Response(JSON.stringify({ error: sanitized.reason }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  const limited = checkRateLimit(req);
  if (!limited.ok) {
    return new Response(JSON.stringify({ error: "Too many requests" }), {
      status: 429,
      headers: {
        "Content-Type": "application/json",
        "Retry-After": String(limited.retryAfterSec),
      },
    });
  }

  const apiKey = process.env.PORTKEY_API_KEY;
  const virtualKey = process.env.PORTKEY_VIRTUAL_KEY;
  if (!apiKey?.trim() || !virtualKey?.trim()) {
    return new Response(JSON.stringify({ error: "Ask Me is not configured" }), {
      status: 503,
      headers: { "Content-Type": "application/json" },
    });
  }

  const contextMarkdown = loadContext();
  const portkey = new Portkey({
    baseURL: "https://api.portkey.ai/v1",
    apiKey,
    virtualKey,
  });
  const encoder = new TextEncoder();
  const signal = req.signal;

  const stream = new ReadableStream<Uint8Array>({
    async start(controller) {
      try {
        // Own the prompt here (system + user) rather than a Portkey saved
        // prompt: Gemini rejects system-only requests with "contents is not
        // specified", so the user turn must always be present.
        const completion = await portkey.chat.completions.create(
          {
            model: ASK_MODEL,
            stream: true,
            max_tokens: MAX_OUTPUT_TOKENS,
            messages: [
              { role: "system", content: SYSTEM_PROMPT },
              {
                role: "user",
                content: buildUserMessage(contextMarkdown, sanitized.value),
              },
            ],
          },
          { signal },
        );

        for await (const chunk of completion) {
          if (signal.aborted) break;
          const delta = chunk.choices?.[0]?.delta?.content;
          if (typeof delta === "string" && delta) {
            controller.enqueue(encoder.encode(delta));
          }
        }
        controller.close();
      } catch (e) {
        if (signal.aborted) {
          controller.close();
          return;
        }
        console.error("[ask] upstream error:", e);
        controller.enqueue(
          encoder.encode(
            "\n\n[Ask Me ran into an upstream error. Please try again.]\n",
          ),
        );
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
