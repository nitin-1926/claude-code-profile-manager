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
