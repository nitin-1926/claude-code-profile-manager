import Link from "next/link";
import type { ComponentProps, ReactNode } from "react";

type Variant = "primary" | "secondary" | "ghost";
type Size = "sm" | "md";

const base =
  "inline-flex items-center justify-center gap-2 font-medium rounded-lg transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)] whitespace-nowrap select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:opacity-50 disabled:pointer-events-none";

const variants: Record<Variant, string> = {
  primary:
    "btn-primary bg-accent text-accent-fg hover:opacity-92 active:opacity-85 shadow-[0_1px_0_rgba(255,255,255,0.18)_inset,0_2px_6px_-1px_rgba(184,90,58,0.35),0_0_0_1px_var(--c-accent)]",
  secondary:
    "btn-secondary bg-surface text-fg border border-border hover:border-border-strong hover:bg-surface-hover shadow-[var(--shadow-card)]",
  ghost: "text-fg-muted hover:text-fg hover:bg-surface-hover",
};

const sizes: Record<Size, string> = {
  sm: "h-9 px-3.5 text-[0.8125rem] min-w-[44px]",
  md: "h-10 px-4 text-[0.875rem] min-w-[44px]",
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
