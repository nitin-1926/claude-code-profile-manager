import type { Metadata } from "next";
import Link from "next/link";
import { Nav } from "../components/nav";
import { Footer } from "../components/footer";
import { DotGrid, AccentOrb } from "../components/dot-grid";
import { Button } from "../components/button";
import { CHANGELOG, type ChangelogCategory } from "@/lib/changelog";
import { VERSION_TAG } from "@/lib/version";

export const metadata: Metadata = {
  title: "Changelog",
  description:
    "Release history and notable improvements for ccpm (Claude Code Profile Manager).",
};

const categoryTone: Record<
  ChangelogCategory,
  { border: string; bg: string; text: string }
> = {
  Added: {
    border: "border-success/25",
    bg: "bg-success/[0.06]",
    text: "text-success",
  },
  Improved: {
    border: "border-accent/25",
    bg: "bg-accent-soft",
    text: "text-accent",
  },
  Fixed: {
    border: "border-warning/25",
    bg: "bg-[color:var(--c-warning)]/[0.06]",
    text: "text-[color:var(--c-warning)]",
  },
  Security: {
    border: "border-danger/25",
    bg: "bg-[color:var(--c-danger)]/[0.06]",
    text: "text-[color:var(--c-danger)]",
  },
};

function formatDateRange(dates: string[]): string {
  if (dates.length === 0) return "";
  const sorted = [...dates].sort();
  const first = sorted[0];
  const last = sorted[sorted.length - 1];
  return first === last ? first : `${first} → ${last}`;
}

export default function ChangelogPage() {
  return (
    <>
      <Nav />
      <div className="relative overflow-hidden">
        <DotGrid />
        <AccentOrb className="top-[-120px] right-[-20%] w-[560px] h-[560px]" />
        <div className="relative max-w-3xl mx-auto px-6 pt-12 pb-20">
          <div className="not-prose mb-10">
            <div className="flex flex-wrap items-center gap-2 mb-3">
              <span className="pill pill--accent">
                <span className="pulse-dot" />
                <span>{VERSION_TAG}</span>
              </span>
              <span className="pill">history</span>
            </div>
            <h1
              className="font-semibold tracking-[-0.02em] text-fg leading-[1.1]"
              style={{ fontSize: "var(--t-h1)" }}
            >
              Changelog
            </h1>
            <p className="mt-3 text-fg-muted leading-relaxed max-w-2xl text-[0.9375rem]">
              How ccpm has grown, release by release. Each chapter below is one
              minor series, with the themed changes and patch fixes that
              shipped under it. For every tagged build, see{" "}
              <a
                href="https://github.com/nitin-1926/claude-code-profile-manager/releases"
                target="_blank"
                rel="noopener noreferrer"
                className="text-accent underline underline-offset-2 hover:opacity-90"
              >
                GitHub Releases
              </a>
              .
            </p>
            <div className="mt-6">
              <Button href="/docs" variant="secondary" size="md">
                Back to documentation
              </Button>
            </div>
          </div>

          <ol className="not-prose relative space-y-0 border-l border-border ml-3 sm:ml-4 pl-6 sm:pl-8 pb-2">
            {CHANGELOG.map((series, sIdx) => {
              const dateRange = formatDateRange(
                series.releases.map((r) => r.date),
              );
              const isCurrent = sIdx === 0;
              return (
                <li
                  key={series.series}
                  className="mb-12 last:mb-0"
                >
                  <span
                    className="absolute -left-[5px] sm:-left-[6px] mt-1.5 h-2.5 w-2.5 rounded-full border-2 border-bg bg-accent shadow-[0_0_0_3px_var(--c-bg)]"
                    aria-hidden
                  />
                  <article className="surface-card p-5 sm:p-6 shadow-[var(--shadow-card)]">
                    <header className="mb-4">
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <h2 className="text-xl font-semibold tracking-tight text-fg">
                          v{series.series}
                        </h2>
                        {isCurrent && (
                          <span className="inline-flex items-center rounded-md border border-accent/25 bg-accent-soft px-2 py-0.5 font-mono text-[0.65rem] font-semibold uppercase tracking-wide text-accent">
                            current
                          </span>
                        )}
                        {dateRange && (
                          <time className="font-mono text-[0.75rem] text-fg-subtle tabular-nums">
                            {dateRange}
                          </time>
                        )}
                      </div>
                      <p className="mt-1.5 text-sm text-fg-muted leading-relaxed">
                        {series.summary}
                      </p>
                    </header>

                    <ul className="space-y-4 border-l border-border/60 pl-4">
                      {series.releases.map((r, rIdx) => (
                        <li
                          key={`${r.date}-${r.title}-${rIdx}`}
                          className="relative"
                        >
                          <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
                            <time
                              dateTime={r.date}
                              className="font-mono text-[0.72rem] text-fg-subtle tabular-nums"
                            >
                              {r.date}
                            </time>
                            {r.version && (
                              <span className="font-mono text-[0.72rem] font-semibold text-accent">
                                v{r.version}
                              </span>
                            )}
                            <div className="flex flex-wrap gap-1">
                              {r.categories.map((c) => {
                                const t = categoryTone[c];
                                return (
                                  <span
                                    key={c}
                                    className={`inline-flex items-center rounded-md border px-1.5 py-0.5 font-mono text-[0.6rem] font-semibold uppercase tracking-wide ${t.border} ${t.bg} ${t.text}`}
                                  >
                                    {c}
                                  </span>
                                );
                              })}
                            </div>
                          </div>
                          <h3 className="mt-1 text-[0.95rem] font-semibold tracking-tight text-fg">
                            {r.title}
                          </h3>
                          <ul className="mt-2 space-y-1.5 text-[0.875rem] text-fg-muted leading-relaxed [&_strong]:text-fg">
                            {r.bullets.map((b, bIdx) => (
                              <li
                                key={`${bIdx}-${b.slice(0, 24)}`}
                                className="flex gap-2"
                              >
                                <span
                                  className="text-accent shrink-0 mt-0.5"
                                  aria-hidden
                                >
                                  ·
                                </span>
                                <span>{b}</span>
                              </li>
                            ))}
                          </ul>
                        </li>
                      ))}
                    </ul>
                  </article>
                </li>
              );
            })}
          </ol>

          <p className="not-prose mt-12 text-sm text-fg-muted text-center max-w-md mx-auto">
            Have a question about a change?{" "}
            <Link
              href="/docs"
              className="text-accent underline underline-offset-2"
            >
              Open the docs
            </Link>{" "}
            and use <strong className="text-fg">Ask Me</strong> (⌘K / Ctrl+K).
          </p>
        </div>
      </div>
      <Footer />
    </>
  );
}
