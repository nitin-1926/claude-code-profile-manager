import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

type Props = {
  title: string;
  description: string;
  icon: LucideIcon;
  eyebrow?: string;
  className?: string;
  children?: ReactNode;
};

export function BentoTile({
  title,
  description,
  icon: Icon,
  eyebrow,
  className = "",
  children,
}: Props) {
  return (
    <div
      className={`surface-card group flex flex-col p-6 overflow-hidden ${className}`}
    >
      <div className="flex items-start justify-between gap-4 mb-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <span className="icon-chip inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-accent-muted border border-accent/20">
            <Icon
              size={16}
              strokeWidth={1.75}
              className="text-accent"
            />
          </span>
          {eyebrow && (
            <span className="font-mono text-[0.68rem] uppercase tracking-[0.12em] text-fg-subtle truncate">
              {eyebrow}
            </span>
          )}
        </div>
      </div>

      <h3 className="text-[1rem] font-semibold tracking-tight text-fg leading-snug">
        {title}
      </h3>
      <p className="mt-1.5 text-[0.875rem] text-fg-muted leading-relaxed">
        {description}
      </p>
      {children && <div className="mt-5 flex-1 min-h-0">{children}</div>}
    </div>
  );
}
