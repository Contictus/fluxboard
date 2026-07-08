// Hand-authored API types mirroring docs/08-API-SPEC.md. The served OpenAPI spec
// only covers Phase-6 handlers, so the auth/org/account contracts are typed here
// directly and grown per section. Keep field names in sync with the Go handlers.

/** Error codes from the single error envelope (docs/08 §1). */
export type ApiErrorCode =
  | 'validation_failed'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'plan_limit_exceeded'
  | 'rate_limited'
  | 'internal';

/** The `error` object every non-2xx response carries. */
export interface ApiErrorBody {
  code: ApiErrorCode | string;
  message: string;
  details?: Record<string, unknown>;
  request_id?: string;
}

/** Session tokens returned by login / 2fa-verify / refresh (docs/04 §5). */
export interface TokenResponse {
  access_token: string;
  token_type: 'Bearer';
  expires_at: string; // RFC3339
  user_id: string;
}

/** Login may short-circuit into the TOTP challenge instead of a session. */
export interface TwoFactorRequired {
  status: '2fa_required';
  pending_token: string;
}

export type LoginResponse = TokenResponse | TwoFactorResponse;
export type TwoFactorResponse = TwoFactorRequired;

export function isTwoFactorRequired(r: LoginResponse): r is TwoFactorRequired {
  return 'status' in r && r.status === '2fa_required';
}

/** A live session row (GET /auth/sessions). */
export interface SessionInfo {
  id: string;
  user_agent: string;
  ip?: string;
  current: boolean;
  created_at: string;
  last_used_at: string;
  expires_at: string;
}

/** 2FA enrollment payload (POST /auth/2fa/enroll). */
export interface Enroll2FAResponse {
  secret: string;
  provisioning_uri: string;
}

/** Org membership summary row (GET /orgs). */
export interface OrgSummary {
  id: string;
  name: string;
  slug: string;
  role: string;
  plan?: string;
  status?: string;
}
