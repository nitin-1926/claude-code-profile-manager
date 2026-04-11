"use client";

import { useEffect, useState } from "react";

type Section = { id: string; label: string };
type Group = { title: string; items: Section[] };

const groups: Group[] = [
  {
    title: "Getting started",
    items: [
      { id: "installation", label: "Installation" },
      { id: "quick-start", label: "Quick start" },
    ],
  },
  {
    title: "Profiles",
    items: [
      { id: "profiles", label: "Profile management" },
      { id: "running", label: "Running Claude" },
      { id: "auth", label: "Authentication" },
    ],
  },
  {
    title: "Security",
    items: [
      { id: "vault", label: "Vault backup" },
      { id: "privacy", label: "Privacy & security" },
    ],
  },
  {
    title: "Reference",
    items: [
      { id: "shell", label: "Shell integration" },
      { id: "ide", label: "IDE / VS Code" },
      { id: "platforms", label: "Platform support" },
      { id: "limitations", label: "Known limitations" },
    ],
  },
];

const allIds = groups.flatMap((g) => g.items.map((i) => i.id));

export function DocsSidebar() {
  const [active, setActive] = useState<string>(allIds[0]);

  useEffect(() => {
    const observers: IntersectionObserver[] = [];
    const visible = new Set<string>();

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            visible.add(entry.target.id);
          } else {
            visible.delete(entry.target.id);
          }
        }
        // Pick the first id (in document order) that's currently visible
        const firstVisible = allIds.find((id) => visible.has(id));
        if (firstVisible) setActive(firstVisible);
      },
      { rootMargin: "-72px 0px -70% 0px", threshold: 0 }
    );

    for (const id of allIds) {
      const el = document.getElementById(id);
      if (el) observer.observe(el);
    }
    observers.push(observer);

    return () => observers.forEach((o) => o.disconnect());
  }, []);

  return (
    <aside className="hidden lg:block w-60 shrink-0">
      <nav
        aria-label="Documentation"
        className="sticky top-20 max-h-[calc(100vh-6rem)] overflow-y-auto pr-4"
      >
        {groups.map((group) => (
          <div key={group.title} className="mb-6">
            <h3 className="font-mono text-[0.7rem] font-semibold uppercase tracking-[0.12em] text-fg-subtle mb-2 px-3">
              {group.title}
            </h3>
            <ul className="space-y-0.5">
              {group.items.map((item) => {
                const isActive = active === item.id;
                return (
                  <li key={item.id}>
                    <a
                      href={`#${item.id}`}
                      aria-current={isActive ? "true" : undefined}
                      className={`block px-3 py-1.5 text-sm rounded-md border-l-2 transition-colors ${
                        isActive
                          ? "border-accent bg-accent-muted text-fg font-medium"
                          : "border-transparent text-fg-muted hover:text-fg hover:bg-surface-hover"
                      }`}
                    >
                      {item.label}
                    </a>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>
    </aside>
  );
}
