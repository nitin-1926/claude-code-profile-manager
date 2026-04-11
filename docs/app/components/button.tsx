import Link from "next/link";
import type { ComponentProps, ReactNode } from "react";

type Variant = "primary" | "secondary" | "ghost";
type Size = "sm" | "md";

const base =
  "inline-flex items-center justify-center gap-2 font-medium rounded-lg transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)] whitespace-nowrap select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:opacity-50 disabled:pointer-events-none";

const variants: Record<Variant, string> = {
  primary:
    "bg-accent text-accent-fg hover:opacity-90 active:opacity-80 shadow-[0_0_0_1px_var(--c-accent)]",
  secondary:
    "bg-surface text-fg border border-border hover:border-border-strong hover:bg-surface-hover",
  ghost: "text-fg-muted hover:text-fg hover:bg-surface-hover",
};

const sizes: Record<Size, string> = {
  sm: "h-9 px-3.5 text-sm min-w-[44px]",
  md: "h-11 px-5 text-sm min-w-[44px]",
};

type ButtonProps = {
  variant?: Variant;
  size?: Size;
  href?: string;
  external?: boolean;
  children: ReactNode;
} & Omit<ComponentProps<"button">, "ref">;

export function Button({
  variant = "primary",
  size = "md",
  href,
  external,
  className = "",
  children,
  ...rest
}: ButtonProps) {
  const cls = `${base} ${variants[variant]} ${sizes[size]} ${className}`;

  if (href) {
    if (external) {
      return (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className={cls}
        >
          {children}
        </a>
      );
    }
    return (
      <Link href={href} className={cls}>
        {children}
      </Link>
    );
  }

  return (
    <button className={cls} {...rest}>
      {children}
    </button>
  );
}
