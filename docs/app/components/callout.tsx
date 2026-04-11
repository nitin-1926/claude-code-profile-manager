import { AlertTriangle, Info, Lightbulb, OctagonAlert } from "lucide-react";
import type { ReactNode } from "react";

type Type = "info" | "warn" | "tip" | "danger";

const config: Record<
  Type,
  { icon: typeof Info; label: string; tone: string }
> = {
  info: {
    icon: Info,
    label: "Note",
    tone: "border-border bg-bg-subtle text-fg",
  },
  warn: {
    icon: AlertTriangle,
    label: "Warning",
    tone:
      "border-[color:var(--c-warning)]/40 bg-[color:var(--c-warning)]/[0.06] text-fg",
  },
  tip: {
    icon: Lightbulb,
    label: "Tip",
    tone: "border-accent/40 bg-accent-muted text-fg",
  },
  danger: {
    icon: OctagonAlert,
    label: "Danger",
    tone:
      "border-[color:var(--c-danger)]/40 bg-[color:var(--c-danger)]/[0.06] text-fg",
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
  const { icon: Icon, label, tone } = config[type];

  return (
    <div className={`my-5 rounded-xl border p-4 ${tone}`}>
      <div className="flex items-start gap-3">
        <Icon
          size={18}
          strokeWidth={1.75}
          className={
            type === "info"
              ? "text-fg-muted mt-0.5 shrink-0"
              : type === "warn"
                ? "text-[color:var(--c-warning)] mt-0.5 shrink-0"
                : type === "tip"
                  ? "text-accent mt-0.5 shrink-0"
                  : "text-[color:var(--c-danger)] mt-0.5 shrink-0"
          }
        />
        <div className="min-w-0 flex-1">
          <div className="font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-muted">
            {title || label}
          </div>
          <div className="mt-1 text-sm leading-relaxed text-fg-muted [&_strong]:text-fg [&_a]:text-accent [&_a]:underline">
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}
