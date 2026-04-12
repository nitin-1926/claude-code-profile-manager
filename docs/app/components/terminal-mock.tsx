type Line = {
  text: string;
  prompt?: boolean;
  color?: "fg" | "muted" | "accent" | "warning";
  delay: number;
  duration: number;
};

const lines: Line[] = [
  { text: "ccpm add work", prompt: true, delay: 0.2, duration: 1.4 },
  {
    text: "✓ profile \"work\" created",
    color: "accent",
    delay: 1.8,
    duration: 1.0,
  },
  { text: "ccpm switch work", prompt: true, delay: 3.2, duration: 1.4 },
  {
    text: "→ activated work (anthropic key, mcp: github)",
    color: "muted",
    delay: 4.8,
    duration: 1.6,
  },
  { text: "claude", prompt: true, delay: 6.6, duration: 0.8 },
  {
    text: "Welcome to Claude Code  ▍",
    color: "fg",
    delay: 7.6,
    duration: 1.4,
    cursor: true,
  } as Line & { cursor?: boolean },
];

const colorMap = {
  fg: "text-zinc-100",
  muted: "text-zinc-400",
  accent: "text-[#d77757]",
  warning: "text-amber-300",
};

export function TerminalMock() {
  return (
    <div className="relative w-full">
      <div className="rounded-xl overflow-hidden border border-[color:var(--c-code-border)] bg-[color:var(--c-code-bg)] shadow-2xl shadow-black/40">
        <div className="flex items-center gap-1.5 px-4 py-2.5 border-b border-[color:var(--c-code-border)] bg-black/40">
          <span className="w-3 h-3 rounded-full bg-[#FF5F56]" />
          <span className="w-3 h-3 rounded-full bg-[#FFBD2E]" />
          <span className="w-3 h-3 rounded-full bg-[#27C93F]" />
          <span className="ml-3 text-[11px] text-zinc-500 font-mono">
            ~/dev/app — ccpm
          </span>
        </div>
        <div className="px-5 py-4 font-mono text-[13px] leading-7 min-h-[260px]">
          {lines.map((line, i) => {
            const colorClass = colorMap[line.color ?? "fg"];
            return (
              <span
                key={i}
                className={`terminal-line ${colorClass}`}
                style={{
                  animationName: "typing",
                  animationDuration: `${line.duration}s`,
                  animationDelay: `${line.delay}s`,
                  animationTimingFunction: `steps(${Math.max(line.text.length, 8)}, end)`,
                  animationFillMode: "forwards",
                }}
              >
                {line.prompt && (
                  <span className="text-[#d77757] select-none">$ </span>
                )}
                {line.text}
                {(line as Line & { cursor?: boolean }).cursor && (
                  <span className="terminal-cursor align-middle ml-0.5" />
                )}
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
}
