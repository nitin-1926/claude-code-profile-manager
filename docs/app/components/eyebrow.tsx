// Eyebrow is a small uppercase mono label above a section heading. Content is
// decorative (e.g. "// features") — the real semantic label always lives on
// the H2/H3 that follows. aria-hidden="true" keeps screen readers from
// announcing "slash slash features" on every section.
export function Eyebrow({ children }: { children: React.ReactNode }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block font-mono text-[0.7rem] font-semibold tracking-[0.12em] uppercase text-accent"
    >
      {children}
    </span>
  );
}
