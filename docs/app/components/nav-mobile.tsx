"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Menu, X } from "lucide-react";
import { GithubIcon } from "./brand-icons";
import { navLinks } from "./nav-links";

export function NavMobile() {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  // Close + restore focus to the trigger so keyboard users don't land on
  // <body>.
  const close = useCallback(() => {
    setOpen(false);
    triggerRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!open) return;

    const panel = panelRef.current;
    const focusables = () =>
      Array.from(
        panel?.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      );

    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
        return;
      }
      if (e.key !== "Tab") return;

      // Focus trap: keep Tab/Shift+Tab cycling inside the dialog so the user
      // can't land on the sticky nav behind the overlay.
      const items = focusables();
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
    document.body.style.overflow = "hidden";

    // Focus first link inside panel
    focusables()[0]?.focus();

    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open, close]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen(true)}
        aria-label="Open navigation"
        aria-expanded={open}
        aria-haspopup="dialog"
        className="md:hidden inline-flex h-9 w-9 items-center justify-center rounded-md text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors"
      >
        <Menu size={18} strokeWidth={1.75} />
      </button>

      {open && (
        <div
          className="md:hidden fixed inset-0 z-[100] bg-bg/95 backdrop-blur-md"
          role="dialog"
          aria-modal="true"
          aria-label="Navigation"
          // Click anywhere on the backdrop to close. Inner panel stops
          // propagation so clicking a link or the close button doesn't
          // re-fire this.
          onClick={close}
        >
          <div
            ref={panelRef}
            className="flex flex-col h-full"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between h-14 px-6 border-b border-border">
              <span className="font-mono font-semibold tracking-tight">ccpm</span>
              <button
                type="button"
                onClick={close}
                aria-label="Close navigation"
                className="inline-flex h-11 w-11 items-center justify-center rounded-lg text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors"
              >
                <X size={20} strokeWidth={1.75} />
              </button>
            </div>

            <div className="flex flex-col p-6 gap-1">
              {navLinks.map((l) => (
                <Link
                  key={l.href}
                  href={l.href}
                  onClick={close}
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
        </div>
      )}
    </>
  );
}
