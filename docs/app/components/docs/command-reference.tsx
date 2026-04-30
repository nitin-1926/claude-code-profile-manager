import type { ReactNode } from "react";
import { CodeBlock } from "../code-block";
import { H3 } from "./section-headings";

// CommandReference is the canonical shape for a "here is one ccpm subcommand"
// block inside the docs page: H3 + description + code example + optional
// trailing note. Several dozen subcommands share this exact structure; keeping
// one component means future tweaks (copy buttons, flag tables, etc.) land in
// one file instead of 20 duplicated JSX literals.

export type CommandReferenceProps = {
  id: string;
  name: string;
  description: ReactNode;
  example?: string;
  lang?: string;
  note?: ReactNode;
  children?: ReactNode;
};

export function CommandReference({
  id,
  name,
  description,
  example,
  lang = "bash",
  note,
  children,
}: CommandReferenceProps) {
  return (
    <>
      <H3 id={id}>{name}</H3>
      <p>{description}</p>
      {children}
      {example && <CodeBlock code={example} lang={lang} />}
      {note && <p className="text-sm text-fg-muted">{note}</p>}
    </>
  );
}
