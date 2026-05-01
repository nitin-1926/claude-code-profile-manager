"use client";

import { useEffect, useState } from "react";
import type { LucideIcon } from "lucide-react";
import {
  Rocket,
  Users,
  Share2,
  Wrench,
  BookOpen,
} from "lucide-react";

type Section = { id: string; label: string };
type Group = { title: string; icon: LucideIcon; items: Section[] };

const groups: Group[] = [
  {
    title: "Getting started",
    icon: Rocket,
    items: [
      { id: "installation", label: "Installation" },
      { id: "quick-start", label: "Quick start" },
    ],
  },
  {
    title: "Profiles",
    icon: Users,
    items: [
      { id: "profiles", label: "Profile management" },
      { id: "running", label: "Running Claude" },
      { id: "auth", label: "Authentication" },
    ],
  },
  {
    title: "Sharing & sync",
    icon: Share2,
    items: [
      { id: "import", label: "Import & wizard" },
      { id: "skills", label: "Skills, MCP, settings" },
      { id: "mcp-auth", label: "MCP auth model" },
      { id: "settings-precedence", label: "Settings precedence" },
    ],
  },
  {
    title: "Operations",
    icon: Wrench,
    items: [
      { id: "doctor", label: "Doctor" },
      { id: "drift", label: "Drift detection" },
      { id: "vault", label: "Vault backup" },
      { id: "uninstall", label: "Uninstall" },
    ],
  },
  {
    title: "Reference",
    icon: BookOpen,
    items: [
      { id: "shell", label: "Shell integration" },
      { id: "ide", label: "IDE / VS Code" },
      { id: "privacy", label: "Privacy & security" },
      { id: "platforms", label: "Platform support" },
      { id: "limitations", label: "Known limitations" },
    ],
  },
];

const allIds = groups.flatMap((g) => g.items.map((i) => i.id));

export function DocsSidebar() {
  const [active, setActive] = useState<string>(allIds[0]);
