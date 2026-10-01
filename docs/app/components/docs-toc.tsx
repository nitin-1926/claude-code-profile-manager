"use client";

import { useEffect, useMemo, useState } from "react";
import { useScrollspy } from "./use-scrollspy";

type Heading = { id: string; text: string; level: 2 | 3 };

export function DocsToc() {
  const [headings, setHeadings] = useState<Heading[]>([]);
  const ids = useMemo(() => headings.map((h) => h.id), [headings]);
  const active = useScrollspy(ids, "-80px 0px -75% 0px");

  useEffect(() => {
    const raf = requestAnimationFrame(() => {
      const found: Heading[] = [];
      document
        .querySelectorAll<HTMLHeadingElement>("main h2[id], main h3[id]")
        .forEach((el) => {
          found.push({
            id: el.id,
            text: el.textContent?.replace(/#$/, "").trim() ?? "",
            level: el.tagName === "H2" ? 2 : 3,
          });
        });
      setHeadings(found);
    });

    return () => cancelAnimationFrame(raf);
  }, []);

  if (headings.length === 0) return null;

  return (
    <aside className="hidden xl:block w-52 shrink-0">
      <div className="sticky top-20 max-h-[calc(100vh-6rem)] overflow-y-auto">
        <h3 className="font-mono text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-fg-subtle mb-3">
          On this page
        </h3>
        <ul className="space-y-0.5">
          {headings.map((h) => {
            const isActive = active === h.id;
            return (
              <li key={h.id}>
                <a
                  href={`#${h.id}`}
                  className={`block py-1 text-[0.75rem] leading-snug border-l-2 transition-colors ${
                    h.level === 3 ? "pl-5" : "pl-3"
                  } ${
                    isActive
                      ? "border-accent text-fg"
                      : "border-transparent text-fg-muted hover:text-fg"
                  }`}
                >
                  {h.text}
                </a>
              </li>
            );
          })}
        </ul>
      </div>
    </aside>
  );
}
