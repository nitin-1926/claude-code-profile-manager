"use client";

import { useEffect, type ReactNode } from "react";
import { AiSearchProvider, useAiSearch } from "./ai-search-context";
import { AiSearchDialog } from "./ai-search-dialog";
import { FloatingAskButton } from "./floating-ask-button";

function GlobalAskShortcutsInner({ children }: { children: ReactNode }) {
  const { openDialog, open } = useAiSearch();

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const meta = e.metaKey || e.ctrlKey;
      if (meta && e.key === "k") {
        e.preventDefault();
        openDialog();
        return;
      }
      if (e.key === "/" && !open) {
        const t = e.target as HTMLElement | null;
        const tag = t?.tagName?.toLowerCase();
        const editable =
          tag === "input" ||
          tag === "textarea" ||
          tag === "select" ||
          t?.isContentEditable;
        if (!editable) {
          e.preventDefault();
          openDialog();
        }
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [openDialog, open]);

  return <>{children}</>;
}

export function AiSearchRoot({ children }: { children: ReactNode }) {
  return (
    <AiSearchProvider>
      <GlobalAskShortcutsInner>
        {children}
        <AiSearchDialog />
        <FloatingAskButton />
      </GlobalAskShortcutsInner>
    </AiSearchProvider>
  );
}
