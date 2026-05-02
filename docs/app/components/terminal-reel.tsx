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
