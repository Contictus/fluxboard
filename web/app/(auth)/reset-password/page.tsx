'use client';

import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { Suspense, useState } from 'react';
import { useMutation } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { resetPassword } from '@/lib/api/auth';
import { ApiError } from '@/lib/api/client';

function ResetForm() {
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get('token') ?? '';

  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [mismatch, setMismatch] = useState<string | undefined>();

  const mutation = useMutation({
    mutationFn: () => resetPassword({ token, password }),
    onSuccess: () => router.replace('/login'),
  });

  const err = mutation.error;
  const message =
    err instanceof ApiError
      ? err.status === 404 || err.status === 422
        ? 'This reset link is invalid or has expired.'
        : err.message
      : err
        ? 'Something went wrong.'
        : null;

  if (!token) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Invalid link</CardTitle>
          <CardDescription>This password reset link is missing its token.</CardDescription>
        </CardHeader>
        <CardContent>
          <Link href="/forgot-password">
            <Button variant="outline" className="w-full">
              Request a new link
            </Button>
          </Link>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Set a new password</CardTitle>
        <CardDescription>Choose a strong password you don&apos;t use elsewhere.</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (password !== confirm) {
              setMismatch('Passwords do not match.');
              return;
            }
            setMismatch(undefined);
            mutation.mutate();
          }}
        >
          <FormError message={message} />
          <Field id="password" label="New password">
            <Input
              id="password"
              type="password"
              autoComplete="new-password"
              required
              minLength={8}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <Field id="confirm" label="Confirm password" error={mismatch}>
            <Input
              id="confirm"
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </Field>
          <Button type="submit" className="w-full" disabled={mutation.isPending}>
            {mutation.isPending ? 'Saving…' : 'Reset password'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

export default function ResetPasswordPage() {
  return (
    <Suspense>
      <ResetForm />
    </Suspense>
  );
}
