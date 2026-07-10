// Guards the post-auth `?next=` redirect against open-redirect abuse. A crafted
// `next=https://evil.com` (or protocol-relative `//evil.com`) must never send the
// user off-origin after login/2FA/OAuth. We only ever honor a same-origin path.

const FALLBACK = '/app';

/**
 * Return `next` only when it is a safe, same-origin absolute path; otherwise the
 * fallback. Accepts `/app`, `/app/foo?x=1`. Rejects absolute URLs (`https://…`,
 * any `scheme:` — they don't start with `/`) and protocol-relative variants
 * (`//host`, `/\host`, which browsers normalize to `//`).
 */
export function sanitizeNext(next: string | null | undefined, fallback = FALLBACK): string {
  if (!next) return fallback;
  // Must be a single-slash-rooted path (rules out https://, mailto:, etc.).
  if (!next.startsWith('/')) return fallback;
  // Reject protocol-relative + backslash-normalized variants (//host, /\host).
  const normalized = next.replace(/\\/g, '/');
  if (normalized.startsWith('//')) return fallback;
  return next;
}
