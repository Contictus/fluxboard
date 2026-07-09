// LexoRank string-ordering keys — a faithful TypeScript port of the Go
// `backend/internal/pkg/rank/rank.go` (ADR-009 / ADR-017). A rank is a non-empty
// string over the base-36 alphabet `0-9a-z` whose byte order equals lexicographic
// order, so a key minted here sorts identically to Postgres `ORDER BY rank`. That
// lets a kanban drag place a card at a new position WITHOUT a server round-trip:
// read the destination neighbours' ranks and mint a key strictly between them.
//
// The server (`MoveTask`) stores the supplied rank verbatim, so this port MUST
// match the Go algorithm digit-for-digit — do not "optimize" it independently.

const ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz';
const BASE = ALPHABET.length; // 36
const MID = 'i'; // ALPHABET[18], the canonical halfway key

/** Thrown by `between` when a >= b (no key can exist strictly between them). */
export class RankOutOfOrderError extends Error {
  constructor() {
    super('rank: lower bound not strictly below upper bound');
    this.name = 'RankOutOfOrderError';
  }
}

function idx(c: string): number {
  return ALPHABET.indexOf(c);
}

/**
 * Returns a key strictly between `a` and `b`. Empty `a` means "before all"
 * (negative infinity); empty `b` means "after all" (positive infinity); both
 * empty yields the canonical middle key. Throws `RankOutOfOrderError` when both
 * bounds are present and `a` is not strictly below `b`.
 */
export function between(a: string, b: string): string {
  if (a === '' && b === '') return MID;
  if (a === '') return before(b);
  if (b === '') return after(a);
  if (a >= b) throw new RankOutOfOrderError();
  return betweenBoth(a, b);
}

/** Returns a key ordering after `last` (the current maximum). */
export function append(last: string): string {
  return last === '' ? MID : after(last);
}

/** Returns a key ordering before `first` (the current minimum). */
export function prepend(first: string): string {
  return first === '' ? MID : before(first);
}

// betweenBoth returns a key strictly between a and b, both non-empty with a < b.
function betweenBoth(a: string, b: string): string {
  let i = 0;
  while (i < a.length && i < b.length && a[i] === b[i]) i++;
  const prefix = a.slice(0, i);
  if (i === a.length) {
    // a is a proper prefix of b: descend below b's remaining tail.
    return prefix + before(b.slice(i));
  }
  const da = idx(a[i] as string);
  const db = idx(b[i] as string);
  if (db - da >= 2) {
    return prefix + ALPHABET[(da + db) >> 1];
  }
  // Adjacent digits: keep a's digit and rise above its tail.
  return prefix + a[i] + after(a.slice(i + 1));
}

// before returns a key strictly below s (and above negative infinity).
function before(s: string): string {
  const d = idx(s[0] as string);
  if (d >= 2) return ALPHABET[Math.floor(d / 2)] as string;
  // Leading digit is 0 or 1: share the low path, then take the middle.
  return '0' + beforeTail(s.slice(1));
}

function beforeTail(s: string): string {
  return s === '' ? MID : before(s);
}

// after returns a key strictly above lo (and below positive infinity).
function after(lo: string): string {
  if (lo === '') return MID;
  const last = lo.length - 1;
  const d = idx(lo[last] as string);
  if (d < BASE - 1) return lo.slice(0, last) + ALPHABET[d + 1];
  return lo + MID;
}
