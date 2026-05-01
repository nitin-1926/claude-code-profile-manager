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
