"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import {
  Copy,
  CornerDownLeft,
  Loader2,
  RotateCcw,
  Search,
  Sparkles,
  Square,
  X,
} from "lucide-react";
import { useAiSearch } from "./ai-search-context";
import { Markdown } from "./markdown";
import { MAX_QUESTION_CHARS } from "@/lib/ai/config";

const EXAMPLES = [
  "Run two profiles at once",
  "Set a default for VS Code",
  "Share an MCP server across profiles",
  "Back up & restore credentials",
  "Import my existing ~/.claude setup",
];

function errorForStatus(status: number, fallback?: string): string {
  if (status === 429)
    return "You're asking too fast — try again in a few seconds.";
  if (status === 503) return "Ask Me isn't configured on this server yet.";
  if (status === 400) return fallback || "That question can't be processed.";
  if (status === 403)
    return "This request was blocked. Please retry from the docs site.";
  return fallback || "Something went wrong. Please try again.";
}

export function AiSearchDialog() {
  const { open, closeDialog } = useAiSearch();
  const titleId = useId();
  const descId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const streamRunRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const reset = useCallback(() => {
    streamRunRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    setQuestion("");
    setAnswer("");
    setError(null);
    setLoading(false);
    setCopied(false);
  }, []);

  const clearConversation = useCallback(() => {
    reset();
    inputRef.current?.focus();
  }, [reset]);

  const stop = useCallback(() => {
    streamRunRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    setLoading(false);
  }, []);

  const hasResult = Boolean(answer || error || loading);

  useEffect(() => {
    if (!open) {
      reset();
      return;
    }
    const t = window.setTimeout(() => inputRef.current?.focus(), 50);
    return () => window.clearTimeout(t);
  }, [open, reset]);

  useEffect(() => {
    if (!open) return;

    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        closeDialog();
        return;
      }

      if (e.key !== "Tab") return;

      const panel = panelRef.current;
      const items = Array.from(
        panel?.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      );
      if (items.length === 0) return;

      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement as HTMLElement | null;

      if (e.shiftKey) {
        if (active === first || !panel?.contains(active)) {
          e.preventDefault();
          last.focus();
        }
      } else if (active === last) {
        e.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, closeDialog]);

  const submit = useCallback(async () => {
    const q = question.trim();
    if (!q || loading) return;

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const runId = ++streamRunRef.current;

    setLoading(true);
    setError(null);
    setAnswer("");
    setCopied(false);

    try {
      const res = await fetch("/api/ask", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ question: q }),
        signal: controller.signal,
      });

      if (!res.ok) {
        let serverMsg: string | undefined;
        try {
          const data = (await res.json()) as { error?: string };
          serverMsg = data?.error;
        } catch {
          /* ignore */
        }
        if (streamRunRef.current === runId) {
          setError(errorForStatus(res.status, serverMsg));
        }
        return;
      }

      if (!res.body) {
        if (streamRunRef.current === runId) {
          setError("No response body.");
        }
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        if (streamRunRef.current !== runId) return;
        const text = decoder.decode(value, { stream: true });
        if (!text) continue;
        buf += text;
        setAnswer(buf);
      }
    } catch (e) {
      if ((e as { name?: string })?.name === "AbortError") return;
      if (streamRunRef.current === runId) {
        setError("Network error. Please try again.");
      }
    } finally {
      if (streamRunRef.current === runId) {
        setLoading(false);
        abortRef.current = null;
      }
    }
  }, [question, loading]);

  const onBackdrop = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) closeDialog();
    },
    [closeDialog],
  );

  const copyAnswer = useCallback(async () => {
    if (!answer) return;
    try {
      await navigator.clipboard.writeText(answer);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }, [answer]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[160] flex items-start justify-center bg-black/55 px-3 pb-8 pt-[min(10vh,5rem)] backdrop-blur-[6px] motion-reduce:backdrop-blur-none"
      role="presentation"
      onMouseDown={onBackdrop}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descId}
        className="relative w-full max-w-2xl overflow-hidden rounded-[24px] border border-white/10 bg-bg/[0.92] shadow-[0_28px_90px_-28px_rgba(0,0,0,0.85),0_1px_0_rgba(255,255,255,0.14)_inset] backdrop-blur-md motion-reduce:shadow-lg"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_0%,rgba(217,119,87,0.16),transparent_36%),linear-gradient(180deg,rgba(255,255,255,0.06),transparent_42%)]" />
        <div className="pointer-events-none absolute inset-x-8 top-0 h-px bg-gradient-to-r from-transparent via-white/30 to-transparent" />

        <div className="relative flex items-center justify-between gap-3 px-4 py-3">
          <div className="flex min-w-0 items-center gap-2.5">
            <span className="inline-flex h-8 w-8 items-center justify-center rounded-xl border border-white/10 bg-white/[0.06]">
              <Sparkles size={15} strokeWidth={2.2} className="text-accent" />
            </span>
            <div className="min-w-0">
              <h2 id={titleId} className="text-sm font-semibold text-fg">
                Ask ccpm
              </h2>
              <p id={descId} className="text-xs text-fg-subtle">
                Grounded in the ccpm docs.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={closeDialog}
            aria-label="Close"
            className="inline-flex h-9 w-9 items-center justify-center rounded-full text-fg-muted transition-colors hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
          >
            <X size={17} strokeWidth={2} />
          </button>
        </div>

        <div className="relative px-4 pb-4">
          <form
            className="group relative flex items-center gap-3 rounded-2xl bg-white/[0.04] px-4 py-2.5 shadow-[0_0_0_1px_rgba(255,255,255,0.08)] transition-[background-color,box-shadow] duration-[var(--dur-base)] focus-within:bg-white/[0.06] focus-within:shadow-[0_0_0_1px_rgba(255,255,255,0.12),0_0_0_4px_rgba(217,119,87,0.12)] motion-reduce:transition-none"
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            <Search
              size={18}
              strokeWidth={2}
              className="shrink-0 text-fg-subtle"
              aria-hidden
            />
            <input
              ref={inputRef}
              type="text"
              value={question}
              maxLength={MAX_QUESTION_CHARS}
              onChange={(e) => setQuestion(e.target.value)}
              placeholder="Ask about profiles, MCP, OAuth, settings…"
              aria-label="Your question"
              disabled={loading}
              style={{ outline: "none", outlineOffset: 0, borderRadius: 0 }}
              className="min-w-0 flex-1 border-none bg-transparent py-1 text-[1rem] leading-6 text-fg placeholder:text-fg-subtle disabled:opacity-70"
            />
            <button
              type="submit"
              disabled={loading || !question.trim()}
              aria-label="Ask"
              className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-accent text-accent-fg transition-opacity hover:opacity-95 disabled:opacity-35 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg motion-reduce:transition-none"
            >
              {loading ? (
                <Loader2 className="animate-spin" size={16} aria-hidden />
              ) : (
                <CornerDownLeft size={16} strokeWidth={2.3} aria-hidden />
              )}
            </button>
          </form>

          {!answer && !loading && !error && (
            <div className="mt-4">
              <p className="mb-2 text-[0.72rem] uppercase tracking-wide text-fg-subtle">
                Try asking about
              </p>
              <div className="flex flex-wrap gap-2">
                {EXAMPLES.map((ex) => (
                  <button
                    key={ex}
                    type="button"
                    onClick={() => {
                      setQuestion(ex);
                      setError(null);
                      inputRef.current?.focus();
                    }}
                    className="inline-flex h-7 items-center rounded-full border border-white/10 bg-white/[0.04] px-3 text-[0.78rem] text-fg-muted transition-colors hover:border-white/15 hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
                  >
                    {ex}
                  </button>
                ))}
              </div>
            </div>
          )}

          {hasResult && (
            <div className="mt-4 max-h-[46vh] overflow-y-auto rounded-2xl border border-white/10 bg-surface/70 p-4">
              {error && (
                <div className="space-y-3">
                  <p
                    className="text-sm text-[color:var(--c-danger)]"
                    role="alert"
                  >
                    {error}
                  </p>
                  <div className="flex justify-end border-t border-white/10 pt-3">
                    <button
                      type="button"
                      onClick={clearConversation}
                      className="inline-flex min-h-8 items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-fg-muted transition-colors hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
                    >
                      <RotateCcw size={13} />
                      Clear
                    </button>
                  </div>
                </div>
              )}
              {loading && !answer && !error && (
                <div className="flex items-center justify-between gap-2">
                  <div
                    className="flex items-center gap-2 text-sm text-fg-muted"
                    aria-live="polite"
                  >
                    <span className="relative flex h-2.5 w-2.5">
                      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent opacity-50" />
                      <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-accent" />
                    </span>
                    Thinking…
                  </div>
                  <button
                    type="button"
                    onClick={stop}
                    className="inline-flex min-h-8 items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-fg-muted transition-colors hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
                  >
                    <Square size={12} />
                    Stop
                  </button>
                </div>
              )}
              {answer && (
                <div className="space-y-3">
                  <div aria-live="polite">
                    <Markdown>{answer}</Markdown>
                    {loading && (
                      <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse rounded-full bg-accent align-[-2px]" />
                    )}
                  </div>
                  <div className="flex items-center justify-between border-t border-white/10 pt-3">
                    <button
                      type="button"
                      onClick={clearConversation}
                      className="inline-flex min-h-8 items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-fg-muted transition-colors hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
                    >
                      <RotateCcw size={13} />
                      Clear
                    </button>
                    <div className="flex items-center gap-1">
                      {loading && (
                        <button
                          type="button"
                          onClick={stop}
                          className="inline-flex min-h-8 items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-fg-muted transition-colors hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
                        >
                          <Square size={12} />
                          Stop
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => void copyAnswer()}
                        className="inline-flex min-h-8 items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-fg-muted transition-colors hover:bg-white/[0.08] hover:text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent motion-reduce:transition-none"
                      >
                        <Copy size={13} />
                        {copied ? "Copied" : "Copy answer"}
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
