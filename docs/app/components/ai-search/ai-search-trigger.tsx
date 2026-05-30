"use client";

import { MessageCircleQuestion, Sparkles } from "lucide-react";
import { useAiSearch } from "./ai-search-context";

type Variant = "nav" | "hero";

type AiSearchTriggerProps = {
  variant: Variant;
  className?: string;
};

export function AiSearchTrigger({ variant, className }: AiSearchTriggerProps) {
  const { openDialog } = useAiSearch();

  if (variant === "nav") {
    return (
      <button
        type="button"
        onClick={openDialog}
        aria-label="Open Ask Me (keyboard shortcut Command K or Control K)"
        title="Ask about ccpm"
        className={
          className ??
          "hidden sm:inline-flex h-9 items-center gap-1.5 px-2.5 rounded-md text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors text-[0.8125rem] font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg motion-reduce:transition-none"
        }
      >
        <Sparkles size={15} strokeWidth={2} className="text-accent shrink-0" />
        <span>Ask</span>
        <kbd className="hidden lg:inline font-mono text-[0.65rem] px-1 py-0.5 rounded border border-border bg-bg-subtle text-fg-subtle">
          ⌘K
        </kbd>
      </button>
    );
  }

  return (
    <button
      type="button"
      onClick={openDialog}
      className={
        className ??
        "group w-full text-left rounded-xl border border-border bg-surface shadow-[var(--shadow-card)] p-4 hover:border-accent/30 hover:bg-surface-hover transition-[border-color,background-color] duration-[var(--dur-base)] motion-reduce:transition-none focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
      }
    >
      <div className="flex items-start gap-3">
        <span className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-accent-muted border border-accent/20">
          <MessageCircleQuestion
            size={20}
            strokeWidth={2}
            className="text-accent"
            aria-hidden
          />
        </span>
        <div className="min-w-0 flex-1">
          <p className="font-medium text-fg text-[0.9375rem]">Ask about ccpm</p>
          <p className="mt-0.5 text-sm text-fg-muted leading-relaxed">
            Grounded answers from our docs context — open with{" "}
            <kbd className="font-mono text-[0.7rem] px-1 py-0.5 rounded border border-border bg-bg-subtle">
              ⌘K
            </kbd>{" "}
            /{" "}
            <kbd className="font-mono text-[0.7rem] px-1 py-0.5 rounded border border-border bg-bg-subtle">
              Ctrl+K
            </kbd>
            .
          </p>
          <span className="mt-2 inline-flex items-center gap-1 text-sm font-medium text-accent group-hover:underline underline-offset-2">
            <Sparkles size={14} strokeWidth={2} />
            Open Ask Me
          </span>
        </div>
      </div>
    </button>
  );
}
