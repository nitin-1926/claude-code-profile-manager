import { AlertTriangle, Info, Lightbulb, OctagonAlert } from "lucide-react";
import type { ReactNode } from "react";

type Type = "info" | "warn" | "tip" | "danger";

const config: Record<
  Type,
  { icon: typeof Info; label: string; tone: string; iconClass: string }
> = {
  info: {
    icon: Info,
    label: "Note",
    tone: "border-border bg-bg-subtle",
    iconClass: "text-fg-muted",
  },
  warn: {
    icon: AlertTriangle,
    label: "Warning",
    tone:
      "border-[color:var(--c-warning)]/30 bg-[color:var(--c-warning)]/[0.06]",
    iconClass: "text-[color:var(--c-warning)]",
  },
  tip: {
    icon: Lightbulb,
    label: "Tip",
    tone: "border-accent/30 bg-accent-soft",
    iconClass: "text-accent",
  },
  danger: {
    icon: OctagonAlert,
    label: "Danger",
    tone:
      "border-[color:var(--c-danger)]/30 bg-[color:var(--c-danger)]/[0.06]",
    iconClass: "text-[color:var(--c-danger)]",
  },
};

export function Callout({
  type = "info",
  title,
  children,
}: {
  type?: Type;
  title?: string;
  children: ReactNode;
}) {
  const { icon: Icon, label, tone, iconClass } = config[type];

  return (
    <div className={`not-prose my-5 rounded-lg border p-4 ${tone}`}>
      <div className="flex items-start gap-3">
        <div
          className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-current/20 bg-bg/30 ${iconClass}`}
        >
          <Icon size={14} strokeWidth={1.9} />
        </div>
        <div className="min-w-0 flex-1 pt-0.5">
          <div className="font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-muted">
            {title || label}
          </div>
          <div className="mt-1 text-[0.875rem] leading-relaxed text-fg-muted [&_strong]:text-fg [&_a]:text-accent [&_a]:underline [&_a]:underline-offset-2">
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}
