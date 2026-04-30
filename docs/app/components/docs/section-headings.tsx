import { Hash } from "lucide-react";

// Shared H2 / H3 renderers for the docs page. Each heading exposes an in-page
// anchor link that appears on hover AND on keyboard focus so tab-navigation
// users can discover deep links the same way mouse users do.

type Props = {
  id: string;
  children: React.ReactNode;
};

// accessibleLabel extracts a readable label for aria-label. Plain strings are
// used verbatim; anything richer (JSX, arrays) falls back to the slug-cased
// id. The callers today pass strings, but typed JSX is legal so we handle it.
function accessibleLabel(id: string, children: React.ReactNode): string {
  if (typeof children === "string") return `Link to ${children}`;
  if (typeof children === "number") return `Link to ${children}`;
  return `Link to ${id.replace(/-/g, " ")}`;
}

export function H2({ id, children }: Props) {
  return (
    <h2 id={id}>
      {children}
      <a
        href={`#${id}`}
        aria-label={accessibleLabel(id, children)}
        className="heading-anchor inline-flex"
      >
        <Hash size={15} strokeWidth={1.75} className="inline" />
      </a>
    </h2>
  );
}

export function H3({ id, children }: Props) {
  return (
    <h3 id={id}>
      {children}
      <a
        href={`#${id}`}
        aria-label={accessibleLabel(id, children)}
        className="heading-anchor inline-flex"
      >
        <Hash size={13} strokeWidth={1.75} className="inline" />
      </a>
    </h3>
  );
}
