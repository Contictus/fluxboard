// sessionStorage key carrying the short-lived pending-2FA token between the
// /login and /login/2fa steps. Kept out of the URL (it's a bearer-ish token).
export const PENDING_2FA_KEY = 'fluxboard.pending_2fa';
