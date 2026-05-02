"use client";

import { useEffect, useRef, useState } from "react";

export type ReelStep = {
  text: string;
  /**
   * typed  - renders character-by-character with a cursor (simulates typing)
   * instant - appears in one frame (simulates program output)
   */
  kind?: "typed" | "instant";
  color?: "fg" | "muted" | "accent" | "success" | "warn" | "dim";
  /** Prefix character rendered in accent (e.g. "$", ">", "?") */
  prompt?: string;
  /** Pause (ms) after this step before moving to the next */
  afterMs?: number;
};

const colorClass: Record<string, string> = {
  fg: "term-text-fg",
  muted: "term-text-muted",
  accent: "term-text-accent",
  success: "term-text-success",
  warn: "text-amber-400",
  dim: "term-text-muted opacity-60",
};

/**
 * Animates a sequence of terminal lines. Command lines type character by
 * character; output lines appear instantly. Loops with a pause between runs.
 *
 * Performance: the animation is paused whenever the reel is off-screen
 * (IntersectionObserver) or the document is hidden (visibilitychange). This
 * keeps background tabs and below-the-fold instances from burning CPU on
 * low-power devices — noticeable when multiple terminals are stacked on the
 * hero.
 */
export function TerminalReel({
  script,
  startDelay = 0,
  loop = true,
  loopPauseMs = 3500,
  minLines,
}: {
  script: ReelStep[];
  startDelay?: number;
  loop?: boolean;
  loopPauseMs?: number;
  /** Reserve this many lines of vertical space so the terminal body does not
   *  jump as lines appear. Defaults to `script.length`. */
  minLines?: number;
}) {
  const [visibleCount, setVisibleCount] = useState(0);
  const [typedText, setTypedText] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    let activeTimer: ReturnType<typeof setTimeout> | null = null;
    // Pause state is mutated by IntersectionObserver + visibilitychange; the
    // animation loop consults pausedRef between steps and during the sleep
    // helper so ongoing animations naturally stall instead of racing on.
    const pausedRef = { current: false };
    const resumeSignal = { current: null as (() => void) | null };

    const sleep = (ms: number) =>
      new Promise<void>((resolve) => {
        activeTimer = setTimeout(() => {
          activeTimer = null;
          resolve();
        }, ms);
      });

    const waitWhilePaused = () =>
      new Promise<void>((resolve) => {
        if (!pausedRef.current || cancelled) {
          resolve();
          return;
        }
        resumeSignal.current = () => {
          resumeSignal.current = null;
          resolve();
        };
      });

    // Reduced motion: render the final frame and stop. Matches the rest of
    // the site's motion policy (globals.css also gates animation via this
    // media query).
    if (typeof window !== "undefined") {
      const reduced = window.matchMedia(
        "(prefers-reduced-motion: reduce)",
      ).matches;
      if (reduced) {
        queueMicrotask(() => {
          if (cancelled) return;
          setVisibleCount(script.length);
          setTypedText(script[script.length - 1]?.text ?? "");
        });
        return () => {
          cancelled = true;
        };
      }
    }

    // IntersectionObserver flips pausedRef when the reel scrolls off-screen.
    // `threshold: 0.01` keeps it running so long as a sliver is visible.
    let io: IntersectionObserver | null = null;
    const node = containerRef.current;
    if (node && typeof IntersectionObserver !== "undefined") {
      io = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting && document.visibilityState !== "hidden") {
              pausedRef.current = false;
              resumeSignal.current?.();
            } else {
              pausedRef.current = true;
            }
          }
        },
        { threshold: 0.01 },
      );
      io.observe(node);
    }

    const onVisibility = () => {
      if (document.visibilityState === "hidden") {
        pausedRef.current = true;
      } else if (node) {
        // Only resume when the reel is still on-screen.
        const rect = node.getBoundingClientRect();
        const onScreen =
          rect.bottom > 0 && rect.top < window.innerHeight;
        if (onScreen) {
          pausedRef.current = false;
          resumeSignal.current?.();
        }
      }
    };
    document.addEventListener("visibilitychange", onVisibility);

    async function playOnce() {
      if (cancelled) return;
      setVisibleCount(0);
      setTypedText("");
      await sleep(startDelay);
      if (cancelled) return;

      for (let i = 0; i < script.length; i++) {
        if (cancelled) return;
        await waitWhilePaused();
        if (cancelled) return;

        const step = script[i];

        if (step.kind === "typed") {
          setTypedText("");
        } else {
          setTypedText(step.text);
        }
        setVisibleCount(i + 1);

        if (step.kind === "typed") {
          for (let j = 1; j <= step.text.length; j++) {
            if (cancelled) return;
            await waitWhilePaused();
            if (cancelled) return;
            setTypedText(step.text.slice(0, j));
            await sleep(38 + Math.random() * 32);
          }
        }

        await sleep(step.afterMs ?? 280);
      }
    }

    (async () => {
      while (!cancelled) {
        await playOnce();
        if (!loop || cancelled) break;
        await sleep(loopPauseMs);
      }
    })();

    return () => {
      cancelled = true;
      if (activeTimer) {
        clearTimeout(activeTimer);
        activeTimer = null;
      }
      io?.disconnect();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [script, startDelay, loop, loopPauseMs]);

  const reserved = Math.max(minLines ?? script.length, script.length);
  const rows: React.ReactNode[] = [];

  for (let i = 0; i < reserved; i++) {
    const step = script[i];
    const isCurrent = i === visibleCount - 1;

    if (!step || i >= visibleCount) {
      rows.push(
        <div key={i} aria-hidden="true">
          &nbsp;
        </div>,
      );
      continue;
    }

    const text =
      isCurrent && step.kind === "typed" ? typedText : step.text;
    const showCursor =
      isCurrent && step.kind === "typed" && text.length < step.text.length;
    const cls = colorClass[step.color ?? "fg"];

    rows.push(
      <div key={i} className={cls}>
        {step.prompt && (
          <span className="term-text-accent select-none">
            {step.prompt}{" "}
          </span>
        )}
        {text}
        {showCursor && (
          <span className="terminal-cursor align-middle ml-0.5" />
        )}
      </div>,
    );
  }

  // The outer wrapper exists only for the IntersectionObserver target. It's
  // decorative, so it inherits its parent's semantic role. Callers attach
  // aria-label / role="img" at the surrounding chrome (HeroTerminal) so screen
  // readers hear one cohesive "demo terminal" description.
  return <div ref={containerRef}>{rows}</div>;
}
