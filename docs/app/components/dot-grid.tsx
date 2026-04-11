export function DotGrid({ className = "" }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={`pointer-events-none absolute inset-0 dot-grid ${className}`}
    />
  );
}

export function AccentOrb({ className = "" }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={`pointer-events-none absolute accent-orb ${className}`}
    />
  );
}
