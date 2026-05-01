"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { ArrowUpRight, Terminal } from "lucide-react";
import { ThemeToggle } from "./theme-toggle";
import { NavMobile } from "./nav-mobile";
import { GithubIcon } from "./brand-icons";
import { navLinks } from "./nav-links";

export function Nav() {
  const pathname = usePathname();
  // Seed the state from the current scrollY so the first paint already
  // matches what the user sees on a mid-page refresh — otherwise the nav
  // renders transparent for one frame before the effect flips it.
  const [scrolled, setScrolled] = useState(() =>
    typeof window !== "undefined" && window.scrollY > 8,
  );

  useEffect(() => {
    function onScroll() {
      setScrolled(window.scrollY > 8);
    }
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <nav
      aria-label="Primary"
      className={`sticky top-0 z-50 transition-all duration-[var(--dur-base)] ease-[var(--ease-out)] ${
        scrolled
          ? "bg-bg/70 backdrop-blur-xl border-b border-border shadow-[0_1px_0_0_rgba(255,255,255,0.02)]"
          : "bg-transparent border-b border-transparent"
      }`}
    >
      <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between gap-6">
        <Link
          href="/"
          className="flex items-center gap-2 group"
          aria-label="ccpm home"
        >
          <span className="inline-flex h-7 w-7 items-center justify-center rounded-md bg-accent-muted border border-accent/20 transition-transform duration-[var(--dur-base)] group-hover:rotate-[8deg]">
            <Terminal
              size={14}
              strokeWidth={2.25}
              className="text-accent"
            />
          </span>
          <span className="font-mono font-semibold tracking-tight text-fg">
            ccpm
          </span>
        </Link>

        <div className="hidden md:flex items-center gap-0.5 text-[0.8125rem]">
          {navLinks.map((l) => {
            const active =
              l.href === "/docs"
                ? pathname?.startsWith("/docs")
                : pathname === l.href;
            return (
              <Link
                key={l.href}
                href={l.href}
                aria-current={active ? "page" : undefined}
                className={`px-3 py-1.5 rounded-md transition-colors ${
                  active
                    ? "text-fg bg-surface-hover"
                    : "text-fg-muted hover:text-fg hover:bg-surface-hover"
                }`}
              >
                {l.label}
              </Link>
            );
          })}
        </div>

        <div className="flex items-center gap-1.5">
          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="GitHub repository"
            className="hidden sm:inline-flex h-9 w-9 items-center justify-center rounded-md text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors"
          >
            <GithubIcon size={16} />
          </a>
          <ThemeToggle />
          <Link
            href="/docs"
            className="hidden sm:inline-flex items-center gap-1 h-9 px-3.5 rounded-md bg-accent text-accent-fg text-[0.8125rem] font-medium hover:opacity-90 transition-opacity shadow-[0_1px_0_rgba(255,255,255,0.15)_inset,0_1px_2px_rgba(0,0,0,0.1)]"
          >
            Get started
            <ArrowUpRight size={13} strokeWidth={2.25} />
          </Link>
          <NavMobile />
        </div>
      </div>
    </nav>
  );
}
