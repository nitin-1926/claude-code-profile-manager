"use client";

import { usePathname } from "next/navigation";
import { Sparkles } from "lucide-react";
import { useAiSearch } from "./ai-search-context";

const SHOW_PATH_PREFIXES = ["/docs", "/changelog"];

export function FloatingAskButton() {
  const pathname = usePathname() ?? "";
  const { openDialog } = useAiSearch();

  const show = SHOW_PATH_PREFIXES.some(
    (p) => pathname === p || pathname.startsWith(`${p}/`),
  );
  if (!show) return null;

  return (
    <button
      type="button"
      onClick={openDialog}
      aria-label="Open Ask Me"
      title="Ask about ccpm"
      className="fixed bottom-6 right-6 z-[120] inline-flex h-12 w-12 items-center justify-center rounded-full bg-accent text-accent-fg shadow-[var(--shadow-raised)] border border-white/10 hover:opacity-95 transition-opacity duration-[var(--dur-base)] motion-reduce:transition-none focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg md:bottom-8 md:right-8"
    >
      <Sparkles size={22} strokeWidth={2} aria-hidden />
    </button>
  );
}
