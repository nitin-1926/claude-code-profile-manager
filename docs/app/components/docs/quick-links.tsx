import Link from "next/link";
import {
  ArrowUpRight,
  BookOpen,
  Rocket,
  Share2,
  Terminal,
  Users,
  Wrench,
  type LucideIcon,
} from "lucide-react";

type QuickLink = {
  icon: LucideIcon;
  title: string;
  desc: string;
  href: string;
};

export const docsQuickLinks: QuickLink[] = [
  {
    icon: Rocket,
    title: "Getting started",
    desc: "Install ccpm, create your first profile, run two sessions.",
    href: "#installation",
  },
  {
    icon: Users,
    title: "Profiles & auth",
    desc: "Create, list, remove profiles. OAuth or API key, per profile.",
    href: "#profiles",
  },
  {
    icon: Share2,
    title: "Sharing & sync",
    desc: "Skills, MCP servers, settings — global or per-profile.",
    href: "#skills",
  },
  {
    icon: Wrench,
    title: "Operations",
    desc: "Doctor, drift detection, vault backups, uninstall.",
    href: "#doctor",
  },
  {
    icon: BookOpen,
    title: "Reference",
    desc: "Shell hook, IDE integration, platform notes, limitations.",
    href: "#shell",
  },
  {
    icon: Terminal,
    title: "Full command reference",
    desc: "Every subcommand, flag, and example in one place.",
    href: "#profiles",
  },
];

export function QuickLinksGrid({ links = docsQuickLinks }: { links?: QuickLink[] }) {
  return (
    <div className="mt-7 grid grid-cols-1 sm:grid-cols-2 gap-2.5">
      {links.map((q) => {
        const Icon = q.icon;
        return (
          <Link
            key={q.title}
            href={q.href}
            className="surface-card group p-4 flex items-start gap-3"
          >
            <span className="icon-chip inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-accent-muted border border-accent/20">
              <Icon size={14} strokeWidth={1.9} className="text-accent" />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center justify-between gap-2">
                <div className="font-semibold text-[0.875rem] text-fg leading-tight">
                  {q.title}
                </div>
                <ArrowUpRight
                  size={13}
                  strokeWidth={2}
                  className="text-fg-subtle group-hover:text-accent transition-colors shrink-0"
                />
              </div>
              <div className="mt-0.5 text-[0.8125rem] text-fg-muted leading-snug">
                {q.desc}
              </div>
            </div>
          </Link>
        );
      })}
    </div>
  );
}
