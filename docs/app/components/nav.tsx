"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { ArrowUpRight, Terminal } from "lucide-react";
import { ThemeToggle } from "./theme-toggle";
import { NavMobile } from "./nav-mobile";
import { GithubIcon } from "./brand-icons";

const links = [
  { href: "/docs", label: "Docs" },
  { href: "/#features", label: "Features" },
  { href: "/#privacy", label: "Privacy" },
];

export function Nav() {
  const pathname = usePathname();
  const [scrolled, setScrolled] = useState(false);

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
      className={`sticky top-0 z-50 transition-colors duration-[var(--dur-base)] ease-[var(--ease-out)] ${
        scrolled
          ? "bg-bg/75 backdrop-blur-md border-b border-border"
          : "bg-transparent border-b border-transparent"
      }`}
    >
      <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between gap-6">
        <Link
          href="/"
          className="flex items-center gap-2 group"
          aria-label="ccpm home"
        >
          <Terminal
            size={18}
            strokeWidth={2}
            className="text-accent transition-transform group-hover:rotate-12 duration-[var(--dur-base)]"
          />
          <span className="font-mono font-semibold tracking-tight text-fg">
            ccpm
          </span>
        </Link>

        <div className="hidden md:flex items-center gap-1 text-sm">
          {links.map((l) => {
            const active =
              l.href === "/docs"
                ? pathname?.startsWith("/docs")
                : pathname === l.href;
            return (
              <Link
                key={l.href}
                href={l.href}
                aria-current={active ? "page" : undefined}
                className={`px-3 py-2 rounded-md transition-colors ${
                  active
                    ? "text-fg"
                    : "text-fg-muted hover:text-fg hover:bg-surface-hover"
                }`}
              >
                {l.label}
              </Link>
            );
          })}
        </div>

        <div className="flex items-center gap-2">
          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="GitHub repository"
            className="hidden sm:inline-flex h-11 w-11 items-center justify-center rounded-lg text-fg-muted hover:text-fg hover:bg-surface-hover transition-colors"
          >
            <GithubIcon size={18} />
          </a>
          <ThemeToggle />
          <Link
            href="/docs"
            className="hidden sm:inline-flex items-center gap-1 h-11 px-4 rounded-lg bg-accent text-accent-fg text-sm font-medium hover:opacity-90 transition-opacity"
          >
            Get started
            <ArrowUpRight size={14} strokeWidth={2} />
          </Link>
          <NavMobile />
        </div>
      </div>
    </nav>
  );
}
