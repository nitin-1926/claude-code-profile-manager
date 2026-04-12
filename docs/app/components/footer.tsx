import Link from "next/link";
import { Heart, Terminal } from "lucide-react";
import { GithubIcon } from "./brand-icons";

export function Footer() {
  return (
    <footer className="mt-auto border-t border-border bg-bg-subtle">
      <div className="max-w-6xl mx-auto px-6 py-12 grid grid-cols-1 sm:grid-cols-3 gap-10">
        <div>
          <Link
            href="/"
            className="inline-flex items-center gap-2"
            aria-label="ccpm home"
          >
            <Terminal size={18} strokeWidth={2} className="text-accent" />
            <span className="font-mono font-semibold tracking-tight text-fg">
              ccpm
            </span>
          </Link>
          <p className="mt-3 text-sm text-fg-muted leading-relaxed max-w-xs">
            Run multiple Claude Code accounts in parallel — each fully isolated.
          </p>
        </div>

        <div>
          <h3 className="font-mono text-[0.7rem] font-semibold uppercase tracking-[0.12em] text-fg-subtle mb-4">
            Product
          </h3>
          <ul className="space-y-2.5 text-sm">
            <li>
              <Link
                href="/docs"
                className="text-fg-muted hover:text-fg transition-colors"
              >
                Documentation
              </Link>
            </li>
            <li>
              <Link
                href="/#features"
                className="text-fg-muted hover:text-fg transition-colors"
              >
                Features
              </Link>
            </li>
            <li>
              <a
                href="https://github.com/nitin-1926/claude-code-profile-manager/releases"
                target="_blank"
                rel="noopener noreferrer"
                className="text-fg-muted hover:text-fg transition-colors"
              >
                Changelog
              </a>
            </li>
          </ul>
        </div>

        <div>
          <h3 className="font-mono text-[0.7rem] font-semibold uppercase tracking-[0.12em] text-fg-subtle mb-4">
            Resources
          </h3>
          <ul className="space-y-2.5 text-sm">
            <li>
              <a
                href="https://github.com/nitin-1926/claude-code-profile-manager"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-fg-muted hover:text-fg transition-colors"
              >
                <GithubIcon size={14} />
                GitHub
              </a>
            </li>
            <li>
              <a
                href="https://www.npmjs.com/package/@ngcodes/ccpm"
                target="_blank"
                rel="noopener noreferrer"
                className="text-fg-muted hover:text-fg transition-colors"
              >
                npm
              </a>
            </li>
            <li>
              <a
                href="https://github.com/nitin-1926/claude-code-profile-manager/issues"
                target="_blank"
                rel="noopener noreferrer"
                className="text-fg-muted hover:text-fg transition-colors"
              >
                Report an issue
              </a>
            </li>
          </ul>
        </div>
      </div>

      <div className="border-t border-border">
        <div className="max-w-6xl mx-auto px-6 py-5 flex flex-col sm:flex-row items-center justify-between gap-3 text-xs text-fg-subtle">
          <div className="flex items-center gap-1.5">
            <span>MIT License</span>
            <span className="text-fg-subtle/50">•</span>
            <span className="inline-flex items-center gap-1">
              Built with <Heart size={11} strokeWidth={2.25} className="text-accent" /> by Nitin Gupta
            </span>
          </div>
          <span className="font-mono">v0.1.0</span>
        </div>
      </div>
    </footer>
  );
}
