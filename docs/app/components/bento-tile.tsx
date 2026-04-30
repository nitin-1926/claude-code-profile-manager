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
