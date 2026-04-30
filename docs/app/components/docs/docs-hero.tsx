import { ArrowUpRight } from "lucide-react";
import { GithubIcon } from "../brand-icons";
import { VERSION } from "@/lib/version";
import { QuickLinksGrid } from "./quick-links";

// Docs page hero: version pill + GitHub pill + headline + quick-links grid.
// Kept as a standalone component so the docs page's top of file stays
// navigable and so the hero can be reused on any future sibling docs routes.
export function DocsHero() {
  return (
    <div className="mb-10 not-prose">
      <div className="flex items-center gap-2 mb-3">
        <span className="pill pill--accent">
          <span className="pulse-dot" />
          <span>docs · v{VERSION}</span>
        </span>
        <a
          href="https://github.com/nitin-1926/claude-code-profile-manager"
          target="_blank"
          rel="noopener noreferrer"
          className="pill hover:text-fg transition-colors"
        >
          <GithubIcon size={11} />
          <span>source</span>
          <ArrowUpRight size={10} strokeWidth={2.25} />
        </a>
      </div>
      <h1
        className="font-semibold tracking-[-0.02em] text-fg leading-[1.1]"
        style={{ fontSize: "var(--t-h1)" }}
      >
        Everything you need to manage multiple Claude Code accounts.
      </h1>
      <p className="mt-3 text-fg-muted leading-relaxed text-[0.9375rem] max-w-2xl">
        ccpm is a single static binary that creates fully isolated Claude Code
        profiles — each with its own credentials, settings, memory, and MCP
        servers.
      </p>

      <QuickLinksGrid />

      <div className="mt-10 h-px bg-border" />
    </div>
  );
}
