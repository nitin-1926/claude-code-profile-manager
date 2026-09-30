"use client";

import { useEffect, useState } from "react";

/**
 * Returns the id of the first element in `ids` (document order as given) that
 * is inside the viewport band `rootMargin` describes, or "" before any is.
 * `ids` must be referentially stable between renders.
 */
export function useScrollspy(ids: string[], rootMargin: string): string {
  const [active, setActive] = useState("");

  useEffect(() => {
    if (ids.length === 0) return;
    const visible = new Set<string>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target.id);
          else visible.delete(entry.target.id);
        }
        const first = ids.find((id) => visible.has(id));
        if (first) setActive(first);
      },
      { rootMargin, threshold: 0 },
    );
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, [ids, rootMargin]);

  return active;
}
