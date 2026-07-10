import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { apiFetch, ApiError, setAccessToken } from './client';

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('ApiError', () => {
  it('exposes field-level validation errors from details.fields', () => {
    const err = new ApiError(422, {
      code: 'validation_failed',
      message: 'bad',
      details: { fields: { email: 'required' } },
    });
    expect(err.status).toBe(422);
    expect(err.fieldErrors()).toEqual({ email: 'required' });
  });

  it('returns an empty object when no field errors are present', () => {
    const err = new ApiError(500, { code: 'internal', message: 'boom' });
    expect(err.fieldErrors()).toEqual({});
  });
});

describe('apiFetch', () => {
  beforeEach(async () => {
    setAccessToken(null);
    // The client releases its single-flight refresh latch on a setTimeout(0)
    // macrotask; flush it so a prior test's resolved refresh can't leak in.
    await new Promise((r) => setTimeout(r, 0));
  });
  afterEach(() => vi.unstubAllGlobals());

  it('returns undefined for 204 No Content', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })));
    await expect(apiFetch('/x', { method: 'DELETE' })).resolves.toBeUndefined();
  });

  it('maps a non-2xx envelope to ApiError', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => json({ error: { code: 'not_found', message: 'gone' } }, 404)),
    );
    await expect(apiFetch('/x')).rejects.toMatchObject({ status: 404, code: 'not_found' });
  });

  it('on 401 refreshes once then retries the original request', async () => {
    const calls: string[] = [];
    const fetchMock = vi.fn(async (url: string) => {
      calls.push(url);
      if (url.endsWith('/auth/refresh')) return json({ access_token: 'fresh' });
      // first hit 401, second (post-refresh) succeeds
      const priorHits = calls.filter((u) => u.endsWith('/data')).length;
      return priorHits <= 1 ? json({ error: { code: 'unauthorized' } }, 401) : json({ ok: true });
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(apiFetch<{ ok: boolean }>('/data')).resolves.toEqual({ ok: true });
    expect(calls.filter((u) => u.endsWith('/auth/refresh'))).toHaveLength(1);
    expect(calls.filter((u) => u.endsWith('/data'))).toHaveLength(2);
  });

  it('shares a single in-flight refresh across concurrent 401s', async () => {
    let refreshes = 0;
    // Each data path returns 401 on its first hit, then succeeds after refresh.
    const firstHit: Record<string, boolean> = { '/a': true, '/b': true };
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/refresh')) {
          refreshes += 1;
          return json({ access_token: 'fresh' });
        }
        const path = url.endsWith('/a') ? '/a' : '/b';
        if (firstHit[path]) {
          firstHit[path] = false;
          return json({ error: { code: 'unauthorized' } }, 401);
        }
        return json({ ok: true });
      }),
    );

    await Promise.all([apiFetch('/a'), apiFetch('/b')]);
    expect(refreshes).toBe(1);
  });
});
