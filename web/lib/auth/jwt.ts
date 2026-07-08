// Access-token claim decoding (docs/04-AUTH.md §2, backend jwtx.Claims).
// NOTE: this only base64url-decodes the payload — it does NOT verify the ES256
// signature. Use it for display and client-side guard hints only; the server is
// always the real authority. Fields: sub (user id), sid, ver (email verified),
// imp (impersonated org id), scope, exp. No platform_role — that comes from /me.

export interface AccessClaims {
  sub: string; // user id
  sid?: string; // session id
  ver?: boolean; // email verified at issue time
  imp?: string; // impersonated org id (read-only session active)
  scope?: string;
  exp?: number; // unix seconds
  iat?: number;
}

function base64UrlDecode(input: string): string {
  const pad = input.length % 4 === 0 ? '' : '='.repeat(4 - (input.length % 4));
  const b64 = input.replace(/-/g, '+').replace(/_/g, '/') + pad;
  if (typeof atob === 'function') return atob(b64);
  return Buffer.from(b64, 'base64').toString('binary');
}

/** Decode the payload of a JWT. Returns null on any malformed input. */
export function decodeAccessClaims(token: string | null): AccessClaims | null {
  if (!token) return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  try {
    const payload = parts[1];
    if (!payload) return null;
    const json = decodeURIComponent(
      base64UrlDecode(payload)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join(''),
    );
    return JSON.parse(json) as AccessClaims;
  } catch {
    return null;
  }
}

/** True when the token is absent or its exp is in the past (with a small skew). */
export function isExpired(claims: AccessClaims | null, skewSeconds = 30): boolean {
  if (!claims?.exp) return true;
  return claims.exp * 1000 <= Date.now() + skewSeconds * 1000;
}
