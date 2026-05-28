import { RATE_LIMIT } from "./config";

type Bucket = { count: number; resetAt: number };

const buckets = new Map<string, Bucket>();

function clientKey(req: Request): string {
  const xf = req.headers.get("x-forwarded-for");
  if (xf) {
    return xf.split(",")[0]?.trim() || "unknown";
  }
  return req.headers.get("x-real-ip") || "unknown";
}

export type RateLimitResult =
  | { ok: true }
  | { ok: false; retryAfterSec: number };

// Drop buckets whose window has fully expired so the Map doesn't grow without
// bound across distinct client keys. Opportunistic — runs on each check.
function pruneExpired(now: number): void {
  for (const [k, b] of buckets) {
    if (now >= b.resetAt) buckets.delete(k);
  }
}

export function checkRateLimit(req: Request): RateLimitResult {
  const key = clientKey(req);
  const now = Date.now();
  pruneExpired(now);
  let b = buckets.get(key);
  if (!b || now >= b.resetAt) {
    b = { count: 0, resetAt: now + RATE_LIMIT.windowMs };
    buckets.set(key, b);
  }
  if (b.count >= RATE_LIMIT.maxRequests) {
    return {
      ok: false,
      retryAfterSec: Math.max(1, Math.ceil((b.resetAt - now) / 1000)),
    };
  }
  b.count += 1;
  return { ok: true };
}
