import { MAX_QUESTION_CHARS } from "./config";

const INJECTION_MARKERS =
  /<\|(?:system|user|assistant)\|>|<\/(?:system|user|assistant)>|<ccpm_docs_context>|<\/ccpm_docs_context>|<user_question>|<\/user_question>/i;

const SCRIPT_MARKERS =
  /<\s*\/?\s*(?:script|iframe|object|embed|style|svg|math|link|meta)\b|javascript\s*:|data\s*:\s*text\/html|on(?:error|load|click|mouseover|focus)\s*=/i;

const PROMPT_INJECTION_PHRASES =
  /\b(?:ignore|disregard|forget|override)\s+(?:all\s+)?(?:previous|prior|above|system|developer)\s+(?:instructions?|messages?|prompts?|rules?)\b|\b(?:reveal|print|dump|show)\s+(?:the\s+)?(?:system\s+prompt|developer\s+message|hidden\s+instructions?|raw\s+context)\b/i;

const HTML_COMMENT_MARKERS = /<!--|-->/;

// Zero-width and BOM characters used for silent injection — stripped, not rejected.
// Covers U+200B..U+200D (zero-widths), U+2060 (word joiner), U+FEFF (BOM).
const ZERO_WIDTH = /[​-‍⁠﻿]/g;

export type SanitizeResult =
  | { ok: true; value: string }
  | { ok: false; reason: string };

/**
 * Trim, cap length, strip control chars + zero-widths, reject injection markers
 * and script framing.
 */
export function sanitizeQuestion(raw: unknown): SanitizeResult {
  if (typeof raw !== "string") {
    return { ok: false, reason: "Invalid input" };
  }
  let s = raw.trim();
  if (s.length === 0) {
    return { ok: false, reason: "Question is empty" };
  }
  if (s.length > MAX_QUESTION_CHARS) {
    return {
      ok: false,
      reason: `Question is too long (max ${MAX_QUESTION_CHARS} characters)`,
    };
  }
  s = s.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "");
  s = s.replace(ZERO_WIDTH, "");
  if (INJECTION_MARKERS.test(s)) {
    return { ok: false, reason: "Invalid characters in question" };
  }
  if (HTML_COMMENT_MARKERS.test(s)) {
    return { ok: false, reason: "Invalid characters in question" };
  }
  if (/```\s*system/i.test(s)) {
    return { ok: false, reason: "Invalid characters in question" };
  }
  if (SCRIPT_MARKERS.test(s)) {
    return { ok: false, reason: "Invalid markup in question" };
  }
  if (PROMPT_INJECTION_PHRASES.test(s)) {
    return {
      ok: false,
      reason: "Question appears to contain prompt injection",
    };
  }
  return { ok: true, value: s };
}
