// Formatting helpers for billing/usage display. Money is always integer minor
// units on the wire (invariant #5); we localize only at render.

/** Format integer minor units + ISO currency as a localized amount, e.g. 1999,"usd" → "$19.99". */
export function formatMoney(minor: number, currency = 'usd'): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: currency.toUpperCase(),
    }).format(minor / 100);
  } catch {
    // Unknown currency code — fall back to a plain decimal + code.
    return `${(minor / 100).toFixed(2)} ${currency.toUpperCase()}`;
  }
}

/** Human-readable byte size (binary units). */
export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** i;
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
