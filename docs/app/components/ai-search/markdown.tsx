"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";

/** Allow safe `className` on code for styling; default sanitize strips it. */
const schema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    code: [...(defaultSchema.attributes?.code ?? []), "className"],
    pre: [...(defaultSchema.attributes?.pre ?? []), "className"],
  },
};

type MarkdownProps = {
  /** Markdown source (streaming partials are OK; incomplete syntax may flicker). */
  children: string;
  className?: string;
};

const STYLES = [
  "prose-doc prose-sm max-w-none text-fg",
  // Paragraphs
  "[&_p]:my-2 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0",
  // Headings (rare in short answers, but make sure they don't overpower)
  "[&_h1]:text-base [&_h1]:font-semibold [&_h1]:mt-3 [&_h1]:mb-1",
  "[&_h2]:text-base [&_h2]:font-semibold [&_h2]:mt-3 [&_h2]:mb-1",
  "[&_h3]:text-sm [&_h3]:font-semibold [&_h3]:mt-3 [&_h3]:mb-1",
  // Lists
  "[&_ul]:list-disc [&_ul]:pl-5 [&_ul]:my-2 [&_ul]:space-y-1 [&_ul]:marker:text-accent/70",
  "[&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:my-2 [&_ol]:space-y-1",
  "[&_li]:leading-relaxed",
  // Links
  "[&_a]:text-accent [&_a]:underline [&_a]:underline-offset-2 hover:[&_a]:opacity-90",
  // Inline code (code not inside pre)
  "[&_:not(pre)>code]:bg-white/[0.07] [&_:not(pre)>code]:border [&_:not(pre)>code]:border-white/10 [&_:not(pre)>code]:rounded [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-[0.85em] [&_:not(pre)>code]:font-mono [&_:not(pre)>code]:text-fg",
  // Fenced code blocks
  "[&_pre]:rounded-lg [&_pre]:border [&_pre]:border-white/10 [&_pre]:bg-black/40 [&_pre]:p-3 [&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:text-[0.82rem] [&_pre]:leading-[1.55]",
  "[&_pre_code]:bg-transparent [&_pre_code]:border-0 [&_pre_code]:p-0 [&_pre_code]:font-mono",
  // Blockquotes / hr
  "[&_blockquote]:border-l-2 [&_blockquote]:border-accent/40 [&_blockquote]:pl-3 [&_blockquote]:text-fg-muted [&_blockquote]:my-2",
  "[&_hr]:border-white/10 [&_hr]:my-3",
  // Strong / em
  "[&_strong]:text-fg [&_strong]:font-semibold",
].join(" ");

/**
 * Sanitized markdown for Ask Me answers. Uses GFM + rehype-sanitize defaults.
 */
export function Markdown({ children, className }: MarkdownProps) {
  if (!children.trim()) {
    return null;
  }
  return (
    <div className={className ?? STYLES}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, schema]]}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
