import { describe, expect, it } from 'vitest';

import { sanitizeNext } from './safe-next';

describe('sanitizeNext', () => {
  it('accepts same-origin absolute paths', () => {
    expect(sanitizeNext('/app')).toBe('/app');
    expect(sanitizeNext('/app/projects/WEB?tab=board')).toBe('/app/projects/WEB?tab=board');
  });

  it('rejects absolute URLs', () => {
    expect(sanitizeNext('https://evil.com')).toBe('/app');
    expect(sanitizeNext('http://evil.com/app')).toBe('/app');
    expect(sanitizeNext('mailto:x@y.z')).toBe('/app');
  });

  it('rejects protocol-relative and backslash tricks', () => {
    expect(sanitizeNext('//evil.com')).toBe('/app');
    expect(sanitizeNext('/\\evil.com')).toBe('/app');
    expect(sanitizeNext('/\\/evil.com')).toBe('/app');
  });

  it('rejects empty / nullish and honors a custom fallback', () => {
    expect(sanitizeNext(null)).toBe('/app');
    expect(sanitizeNext(undefined)).toBe('/app');
    expect(sanitizeNext('')).toBe('/app');
    expect(sanitizeNext('relative/no/slash')).toBe('/app');
    expect(sanitizeNext('https://evil.com', '/login')).toBe('/login');
  });
});
