"use client";

import { useEffect, useState } from "react";

type Heading = { id: string; text: string; level: 2 | 3 };

export function DocsToc() {
  const [headings, setHeadings] = useState<Heading[]>([]);
  const [active, setActive] = useState<string>("");

  useEffect(() => {
    let observer: IntersectionObserver | null = null;

    const raf = requestAnimationFrame(() => {
      const found: Heading[] = [];
      document
        .querySelectorAll<HTMLHeadingElement>("main h2[id], main h3[id]")
        .forEach((el) => {
          found.push({
            id: el.id,
            text: el.textContent?.replace(/#$/, "").trim() ?? "",
            level: el.tagName === "H2" ? 2 : 3,
          });
        });
      setHeadings(found);

      if (found.length === 0) return;

      const visible = new Set<string>();
      observer = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) visible.add(entry.target.id);
            else visible.delete(entry.target.id);
          }
          const first = found.map((h) => h.id).find((id) => visible.has(id));
          if (first) setActive(first);
        },
        { rootMargin: "-80px 0px -75% 0px", threshold: 0 },
      );

      found.forEach((h) => {
        const el = document.getElementById(h.id);
