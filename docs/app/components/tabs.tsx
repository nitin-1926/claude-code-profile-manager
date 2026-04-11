"use client";

import { useId, useRef, useState, KeyboardEvent } from "react";
import type { ReactNode } from "react";

export type Tab = {
  id: string;
  label: string;
  content: ReactNode;
};

export function Tabs({ tabs }: { tabs: Tab[] }) {
  const [active, setActive] = useState(0);
  const baseId = useId();
  const tablistRef = useRef<HTMLDivElement>(null);

  function focusTab(index: number) {
    const buttons = tablistRef.current?.querySelectorAll<HTMLButtonElement>(
      '[role="tab"]'
    );
    buttons?.[index]?.focus();
    setActive(index);
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "ArrowRight") {
      e.preventDefault();
      focusTab((active + 1) % tabs.length);
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      focusTab((active - 1 + tabs.length) % tabs.length);
    } else if (e.key === "Home") {
      e.preventDefault();
      focusTab(0);
    } else if (e.key === "End") {
      e.preventDefault();
      focusTab(tabs.length - 1);
    }
  }

  return (
    <div className="w-full">
      <div
        ref={tablistRef}
        role="tablist"
        aria-label="Install command"
        onKeyDown={onKeyDown}
        className="inline-flex items-center gap-1 p-1 rounded-lg bg-surface border border-border"
      >
        {tabs.map((tab, i) => {
          const selected = active === i;
          return (
            <button
              key={tab.id}
              role="tab"
              id={`${baseId}-tab-${i}`}
              aria-selected={selected}
              aria-controls={`${baseId}-panel-${i}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => setActive(i)}
              className={`px-3 py-1.5 text-xs font-mono rounded-md transition-colors min-w-[44px] ${
                selected
                  ? "bg-bg text-fg shadow-sm"
                  : "text-fg-muted hover:text-fg"
              }`}
            >
              {tab.label}
            </button>
          );
        })}
      </div>
      {tabs.map((tab, i) => (
        <div
          key={tab.id}
          role="tabpanel"
          id={`${baseId}-panel-${i}`}
          aria-labelledby={`${baseId}-tab-${i}`}
          hidden={active !== i}
          className="mt-3"
        >
          {tab.content}
        </div>
      ))}
    </div>
  );
}
