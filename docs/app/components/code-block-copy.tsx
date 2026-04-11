"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";

export function CodeBlockCopy({ code }: { code: string }) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      /* clipboard unavailable — silently fail */
    }
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={copied ? "Copied" : "Copy code"}
      className="code-block__copy"
    >
      {copied ? (
        <Check size={16} strokeWidth={2.25} className="text-accent" />
      ) : (
        <Copy size={15} strokeWidth={1.75} />
      )}
    </button>
  );
}
