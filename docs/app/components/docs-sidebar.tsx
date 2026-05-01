"use client";

import { useEffect, useState } from "react";
import type { LucideIcon } from "lucide-react";
import {
  Rocket,
  Users,
  Share2,
  Wrench,
  BookOpen,
} from "lucide-react";

type Section = { id: string; label: string };
type Group = { title: string; icon: LucideIcon; items: Section[] };

const groups: Group[] = [
  {
    title: "Getting started",
    icon: Rocket,
    items: [
      { id: "installation", label: "Installation" },
      { id: "quick-start", label: "Quick start" },
    ],
  },
  {
    title: "Profiles",
    icon: Users,
    items: [
      { id: "profiles", label: "Profile management" },
      { id: "running", label: "Running Claude" },
      { id: "auth", label: "Authentication" },
    ],
  },
  {
    title: "Sharing & sync",
    icon: Share2,
    items: [
      { id: "import", label: "Import & wizard" },
      { id: "skills", label: "Skills, MCP, settings" },
      { id: "mcp-auth", label: "MCP auth model" },
      { id: "settings-precedence", label: "Settings precedence" },
    ],
  },
  {
    title: "Operations",
    icon: Wrench,
    items: [
      { id: "doctor", label: "Doctor" },
      { id: "drift", label: "Drift detection" },
      { id: "vault", label: "Vault backup" },
      { id: "uninstall", label: "Uninstall" },
    ],
  },
  {
    title: "Reference",
    icon: BookOpen,
    items: [
      { id: "shell", label: "Shell integration" },
      { id: "ide", label: "IDE / VS Code" },
      { id: "privacy", label: "Privacy & security" },
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
        const firstVisible = allIds.find((id) => visible.has(id));
        if (firstVisible) setActive(firstVisible);
      },
      { rootMargin: "-72px 0px -70% 0px", threshold: 0 },
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
        className="sticky top-20 max-h-[calc(100vh-6rem)] overflow-y-auto pr-3"
      >
        {groups.map((group) => {
          const GroupIcon = group.icon;
          return (
            <div key={group.title} className="mb-5">
              <h3 className="flex items-center gap-2 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-fg-subtle mb-2 px-2.5">
                <GroupIcon size={12} strokeWidth={2} className="text-accent/70" />
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
                        className={`block px-2.5 py-1.5 text-[0.8125rem] rounded-md border-l-2 transition-all duration-[var(--dur-fast)] ${
                          isActive
                            ? "border-accent bg-accent-soft text-fg font-medium"
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
          );
        })}
      </nav>
    </aside>
  );
}
