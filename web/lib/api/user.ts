import { apiFetch } from './client';
import type { Profile } from './types';

// Account self-service (docs/08 §3, backend §B).

export function getMe(): Promise<Profile> {
  return apiFetch('/me');
}

export function updateMe(input: { name: string }): Promise<Profile> {
  return apiFetch('/me', { method: 'PATCH', body: input });
}

export function avatarUploadURL(input: {
  content_type: string;
  size: number;
}): Promise<{ key: string; url: string }> {
  return apiFetch('/me/avatar/upload-url', { method: 'POST', body: input });
}

export function avatarConfirm(key: string): Promise<void> {
  return apiFetch('/me/avatar/confirm', { method: 'POST', body: { key } });
}

export function deleteMe(): Promise<void> {
  return apiFetch('/me', { method: 'DELETE' });
}

/** Upload the avatar bytes directly to MinIO via the presigned PUT URL. */
export async function putToPresignedUrl(url: string, file: File): Promise<void> {
  const res = await fetch(url, {
    method: 'PUT',
    body: file,
    headers: { 'Content-Type': file.type },
  });
  if (!res.ok) throw new Error(`Upload failed (${res.status})`);
}
