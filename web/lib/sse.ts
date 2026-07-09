import { API_BASE, getAccessToken, refreshSession } from '@/lib/api/client';

// Realtime SSE client (docs/09 §1, FR-NTF-001).
//
// NOTE: the browser's native EventSource cannot send an Authorization header, and
// our access token lives ONLY in memory (never a cookie or URL — invariant #4 /
// docs/CLAUDE.md). The SSE endpoint authenticates with `Authorization: Bearer`
// like every other API route. So instead of EventSource we consume the
// text/event-stream with `fetch` + a ReadableStream reader, which lets us attach
// the bearer and the `Last-Event-ID` header. See ADR-021.

export interface SseEvent {
  id?: string;
  name: string;
  data: Record<string, unknown>;
}

export interface SseHandlers {
  /** A parsed event frame (not the heartbeat, not `resync`). */
  onEvent: (e: SseEvent) => void;
  /** Server asked the client to drop caches and refetch (backlog expired). */
  onResync?: () => void;
  onStatus?: (s: 'connecting' | 'open' | 'closed') => void;
}

// Reconnect backoff (docs/09 §1): 1 → 2 → 5 → 10s, then hold at 10s, plus jitter.
const BACKOFF_MS = [1000, 2000, 5000, 10000];

function jitter(ms: number): number {
  return ms + Math.floor(Math.random() * 400);
}

/**
 * Open a resilient SSE connection to `path` (relative to API_BASE). Returns a
 * disposer that permanently closes the stream. Reconnects with backoff on any
 * drop; tracks Last-Event-ID so a reconnect replays only missed events.
 */
export function connectSse(path: string, handlers: SseHandlers): () => void {
  let closed = false;
  let attempt = 0;
  let lastEventId: string | null = null;
  let controller: AbortController | null = null;
  let retryTimer: ReturnType<typeof setTimeout> | null = null;

  const url = path.startsWith('http') ? path : `${API_BASE}${path}`;

  async function run(): Promise<void> {
    if (closed) return;
    handlers.onStatus?.('connecting');

    let token = getAccessToken();
    if (!token) {
      // No token yet — try a silent refresh before giving up this attempt.
      await refreshSession();
      token = getAccessToken();
    }
    if (!token) {
      scheduleRetry();
      return;
    }

    controller = new AbortController();
    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
      Accept: 'text/event-stream',
    };
    if (lastEventId) headers['Last-Event-ID'] = lastEventId;

    try {
      const res = await fetch(url, {
        method: 'GET',
        headers,
        credentials: 'include',
        signal: controller.signal,
        cache: 'no-store',
      });

      if (res.status === 401) {
        // Token likely expired mid-life — refresh once, then reconnect.
        await refreshSession();
        scheduleRetry();
        return;
      }
      if (!res.ok || !res.body) {
        scheduleRetry();
        return;
      }

      handlers.onStatus?.('open');
      attempt = 0; // a real connection resets the backoff

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        // SSE frames are separated by a blank line.
        let sep: number;
        while ((sep = buffer.indexOf('\n\n')) !== -1) {
          const raw = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          dispatchFrame(raw);
        }
      }
      // Stream ended cleanly (server closed) — reconnect unless disposed.
      scheduleRetry();
    } catch {
      // Network error / abort — reconnect unless disposed.
      if (!closed) scheduleRetry();
    }
  }

  function dispatchFrame(raw: string): void {
    let name = 'message';
    const dataLines: string[] = [];
    let id: string | undefined;

    for (const line of raw.split('\n')) {
      if (line === '' || line.startsWith(':')) continue; // heartbeat / comment
      const idx = line.indexOf(':');
      const field = idx === -1 ? line : line.slice(0, idx);
      const val = idx === -1 ? '' : line.slice(idx + 1).replace(/^ /, '');
      if (field === 'event') name = val;
      else if (field === 'data') dataLines.push(val);
      else if (field === 'id') id = val;
    }

    if (id) lastEventId = id;

    if (name === 'resync') {
      handlers.onResync?.();
      return;
    }

    let data: Record<string, unknown> = {};
    if (dataLines.length > 0) {
      try {
        data = JSON.parse(dataLines.join('\n')) as Record<string, unknown>;
      } catch {
        data = {};
      }
    }
    handlers.onEvent({ id, name, data });
  }

  function scheduleRetry(): void {
    if (closed) return;
    const base = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)] ?? 10000;
    attempt += 1;
    retryTimer = setTimeout(() => void run(), jitter(base));
  }

  void run();

  return () => {
    closed = true;
    handlers.onStatus?.('closed');
    if (retryTimer) clearTimeout(retryTimer);
    controller?.abort();
  };
}
