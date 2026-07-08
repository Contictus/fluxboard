import type { ApiErrorBody, TokenResponse } from './types';

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? 'http://localhost:8080/api/v1';

// ---- In-memory access token -------------------------------------------------
// The access token lives ONLY in module memory (never localStorage/cookie) per
// docs/CLAUDE.md §TypeScript. The refresh token is an httpOnly cookie the browser
// sends automatically to /api/v1/auth/*. The auth context is the single writer.

let accessToken: string | null = null;
const tokenListeners = new Set<(t: string | null) => void>();

export function setAccessToken(token: string | null): void {
  accessToken = token;
  for (const fn of tokenListeners) fn(token);
}

export function getAccessToken(): string | null {
  return accessToken;
}

/** Subscribe to token changes (used by the auth context to mirror into React state). */
export function onAccessTokenChange(fn: (t: string | null) => void): () => void {
  tokenListeners.add(fn);
  return () => tokenListeners.delete(fn);
}

// ---- Error type -------------------------------------------------------------

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, unknown>;
  readonly requestId?: string;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message || body.code || `HTTP ${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.code = body.code;
    this.details = body.details;
    this.requestId = body.request_id;
  }

  /** Field-level validation errors (422 details.fields), when present. */
  fieldErrors(): Record<string, string> {
    const f = this.details?.fields;
    if (f && typeof f === 'object') return f as Record<string, string>;
    return {};
  }
}

async function toApiError(res: Response): Promise<ApiError> {
  let body: ApiErrorBody = { code: 'internal', message: `HTTP ${res.status}` };
  try {
    const json = (await res.json()) as { error?: ApiErrorBody };
    if (json?.error) body = json.error;
  } catch {
    // non-JSON error body — keep the default envelope
  }
  return new ApiError(res.status, body);
}

// ---- Single-flight refresh --------------------------------------------------

let refreshInFlight: Promise<boolean> | null = null;

/** Try to mint a fresh access token from the refresh cookie. Returns success. */
export async function refreshSession(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      try {
        const res = await fetch(`${API_BASE}/auth/refresh`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'X-Requested-With': 'fetch' },
        });
        if (!res.ok) {
          setAccessToken(null);
          return false;
        }
        const tok = (await res.json()) as TokenResponse;
        setAccessToken(tok.access_token);
        return true;
      } catch {
        setAccessToken(null);
        return false;
      } finally {
        // released on the next tick so concurrent callers share this attempt
        setTimeout(() => {
          refreshInFlight = null;
        }, 0);
      }
    })();
  }
  return refreshInFlight;
}

// ---- Core fetch -------------------------------------------------------------

export interface ApiFetchOptions extends Omit<RequestInit, 'body'> {
  body?: unknown; // JSON-serialized unless already a string/FormData
  /** Skip the automatic 401 → refresh → retry (used by the refresh call itself). */
  skipAuthRetry?: boolean;
}

function buildInit(opts: ApiFetchOptions): RequestInit {
  const headers = new Headers(opts.headers);
  headers.set('X-Requested-With', 'fetch');
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`);

  let body = opts.body as BodyInit | undefined;
  if (opts.body !== undefined && !(opts.body instanceof FormData) && typeof opts.body !== 'string') {
    headers.set('Content-Type', 'application/json');
    body = JSON.stringify(opts.body);
  }

  return { ...opts, headers, body, credentials: 'include' };
}

/**
 * Typed API call. Injects the bearer token, sends cookies, maps the error
 * envelope to ApiError, and on a 401 performs a single silent refresh + one retry.
 */
export async function apiFetch<T>(path: string, opts: ApiFetchOptions = {}): Promise<T> {
  const url = path.startsWith('http') ? path : `${API_BASE}${path}`;

  let res = await fetch(url, buildInit(opts));

  if (res.status === 401 && !opts.skipAuthRetry) {
    const ok = await refreshSession();
    if (ok) res = await fetch(url, buildInit(opts));
  }

  if (!res.ok) throw await toApiError(res);

  if (res.status === 204) return undefined as T;
  const ct = res.headers.get('content-type') ?? '';
  if (!ct.includes('application/json')) return (await res.text()) as unknown as T;
  return (await res.json()) as T;
}

export { API_BASE };
