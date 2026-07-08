'use client';

import { useRouter, useSearchParams } from 'next/navigation';
import { Suspense, useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { verify2FA } from '@/lib/api/auth';
import { ApiError } from '@/lib/api/client';
import { useAuth } from '@/lib/auth/context';
import { PENDING_2FA_KEY } from '@/lib/auth/pending';

function TwoFactorForm() {
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get('next') ?? '/app';
  const { adopt } = useAuth();

  const [pending, setPending] = useState<string | null>(null);
  const [code, setCode] = useState('');

  // The pending token was stashed by /login. Without it there's nothing to verify.
  useEffect(() => {
    const t = sessionStorage.getItem(PENDING_2FA_KEY);
    if (!t) {
      router.replace('/login');
      return;
    }
    setPending(t);
  }, [router]);

  const mutation = useMutation({
    mutationFn: () => verify2FA({ pending_token: pending ?? '', code }),
    onSuccess: (tok) => {
      sessionStorage.removeItem(PENDING_2FA_KEY);
      adopt(tok);
      router.replace(next);
    },
  });

  const err = mutation.error;
  const message =
    err instanceof ApiError
      ? err.status === 401 || err.status === 422
        ? 'Invalid or expired code.'
        : err.message
      : err
        ? 'Something went wrong.'
        : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Two-factor authentication</CardTitle>
        <CardDescription>Enter the 6-digit code from your authenticator app.</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          <FormError message={message} />
          <Field id="code" label="Authentication code">
            <Input
              id="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]*"
              maxLength={10}
              required
              autoFocus
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
          <Button type="submit" className="w-full" disabled={mutation.isPending || !pending}>
            {mutation.isPending ? 'Verifying…' : 'Verify'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

export default function TwoFactorPage() {
  return (
    <Suspense>
      <TwoFactorForm />
    </Suspense>
  );
}
