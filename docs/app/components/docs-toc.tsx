"use client";

import { useEffect, useState } from "react";

type Heading = { id: string; text: string; level: 2 | 3 };

export function DocsToc() {
  const [headings, setHeadings] = useState<Heading[]>([]);
  const [active, setActive] = useState<string>("");

  useEffect(() => {
    let observer: IntersectionObserver | null = null;

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

      if (found.length === 0) return;

      const visible = new Set<string>();
      observer = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) visible.add(entry.target.id);
            else visible.delete(entry.target.id);
          }
          const first = found.map((h) => h.id).find((id) => visible.has(id));
          if (first) setActive(first);
        },
        { rootMargin: "-80px 0px -75% 0px", threshold: 0 },
      );

      found.forEach((h) => {
        const el = document.getElementById(h.id);
        if (el && observer) observer.observe(el);
      });
    });

    return () => {
      cancelAnimationFrame(raf);
      observer?.disconnect();
    };
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
