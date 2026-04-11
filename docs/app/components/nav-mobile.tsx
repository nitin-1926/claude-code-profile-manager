"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Menu, X } from "lucide-react";
import { GithubIcon } from "./brand-icons";

const links = [
  { href: "/docs", label: "Docs" },
  { href: "/#features", label: "Features" },
  { href: "/#privacy", label: "Privacy" },
];

export function NavMobile() {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;

    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        setOpen(false);
        triggerRef.current?.focus();
      }
    }

    document.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";

    // Focus first link inside panel
    const firstLink = panelRef.current?.querySelector<HTMLElement>("a");
    firstLink?.focus();

    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen(true)}
        aria-label="Open navigation"
        aria-expanded={open}
        className="md:hidden inline-flex h-11 w-11 items-center justify-center rounded-lg text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors"
      >
        <Menu size={20} strokeWidth={1.75} />
      </button>

      {open && (
        <div
          className="md:hidden fixed inset-0 z-[100] bg-bg/95 backdrop-blur-md"
          role="dialog"
          aria-modal="true"
          aria-label="Navigation"
        >
          <div className="flex items-center justify-between h-14 px-6 border-b border-border">
            <span className="font-mono font-semibold tracking-tight">ccpm</span>
            <button
              type="button"
              onClick={() => setOpen(false)}
              aria-label="Close navigation"
              className="inline-flex h-11 w-11 items-center justify-center rounded-lg text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors"
            >
              <X size={20} strokeWidth={1.75} />
            </button>
          </div>

          <div ref={panelRef} className="flex flex-col p-6 gap-1">
            {links.map((l) => (
              <Link
                key={l.href}
                href={l.href}
                onClick={() => setOpen(false)}
                className="block py-3 text-lg text-fg hover:text-accent transition-colors"
              >
                {l.label}
              </Link>
            ))}
            <a
              href="https://github.com/nitin-1926/claude-code-profile-manager"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 py-3 text-lg text-fg hover:text-accent transition-colors"
            >
              <GithubIcon size={18} />
              GitHub
            </a>
          </div>
        </div>
      )}
    </>
  );
}
