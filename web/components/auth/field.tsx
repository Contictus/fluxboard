import type { ReactNode } from 'react';

import { Label } from '@/components/ui/label';

// A labelled form field with an optional inline error, shared by the auth forms.
export function Field({
  id,
  label,
  error,
  children,
}: {
  id: string;
  label: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  );
}

// A top-of-form error banner for non-field errors (bad credentials, rate limits).
export function FormError({ message }: { message?: string | null }) {
  if (!message) return null;
  return (
    <div className="rounded-xl bg-destructive/10 p-3 text-sm text-destructive">
      {message}
    </div>
  );
}
