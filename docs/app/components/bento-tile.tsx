import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

type Props = {
  title: string;
  description: string;
  icon: LucideIcon;
  className?: string;
  children?: ReactNode;
};

export function BentoTile({
  title,
  description,
  icon: Icon,
  className = "",
  children,
}: Props) {
  return (
    <div
      className={`group relative flex flex-col p-6 rounded-xl border border-border bg-surface overflow-hidden transition-all duration-[var(--dur-base)] ease-[var(--ease-out)] hover:border-border-strong hover:-translate-y-0.5 hover:bg-surface-hover ${className}`}
    >
      <div className="flex items-start justify-between gap-4 mb-4">
        <h3 className="text-[1.0625rem] font-semibold tracking-tight text-fg leading-snug">
          {title}
        </h3>
        <Icon
          size={20}
          strokeWidth={1.75}
          className="text-accent shrink-0 mt-0.5"
        />
      </div>
      <p className="text-sm text-fg-muted leading-relaxed">{description}</p>
      {children && <div className="mt-5 flex-1 min-h-0">{children}</div>}
    </div>
  );
}
