'use client';

import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { MailCheck } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { requestEmailVerification } from '@/lib/api/auth';
import { ApiError } from '@/lib/api/client';
import { RequireAuth } from '@/lib/auth/guards';
import { useAuth } from '@/lib/auth/context';

function VerifyEmailInner() {
  const { logout } = useAuth();
  const [email, setEmail] = useState('');

  const mutation = useMutation({ mutationFn: () => requestEmailVerification(email) });

  const err = mutation.error;
  const message =
    err instanceof ApiError
      ? err.status === 429
        ? 'Please wait a moment before requesting another email.'
        : err.message
      : err
        ? 'Something went wrong.'
        : null;

  return (
    <Card>
      <CardHeader>
        <MailCheck className="h-8 w-8 text-primary" />
        <CardTitle>Verify your email</CardTitle>
        <CardDescription>
          Your email isn&apos;t verified yet. Check your inbox for the verification link. Didn&apos;t
          get it? Resend below.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {mutation.isSuccess ? (
          <p className="rounded-md border border-primary/30 bg-primary/5 p-3 text-sm">
            If that address is unverified, a new link is on its way.
          </p>
        ) : (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              mutation.mutate();
            }}
          >
            <FormError message={message} />
            <Field id="email" label="Email">
              <Input
                id="email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </Field>
            <Button type="submit" className="w-full" disabled={mutation.isPending}>
              {mutation.isPending ? 'Sending…' : 'Resend verification email'}
            </Button>
          </form>
        )}
        <button
          onClick={() => void logout()}
          className="mt-6 w-full text-center text-sm text-muted-foreground hover:text-foreground"
        >
          Sign out
        </button>
      </CardContent>
    </Card>
  );
}

export default function VerifyEmailPage() {
  return (
    <RequireAuth>
      <VerifyEmailInner />
    </RequireAuth>
  );
}
