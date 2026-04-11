export function Eyebrow({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-block font-mono text-[0.7rem] font-semibold tracking-[0.12em] uppercase text-accent">
      {children}
    </span>
  );
}
