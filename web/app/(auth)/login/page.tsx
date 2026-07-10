'use client';

import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { Suspense, useState } from 'react';
import { useMutation } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { login } from '@/lib/api/auth';
import { API_BASE, ApiError } from '@/lib/api/client';
import { isTwoFactorRequired } from '@/lib/api/types';
import { useAuth } from '@/lib/auth/context';
import { PENDING_2FA_KEY } from '@/lib/auth/pending';
import { sanitizeNext } from '@/lib/nav/safe-next';

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const next = sanitizeNext(params.get('next'));
  const { adopt } = useAuth();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const mutation = useMutation({
    mutationFn: () => login({ email, password }),
    onSuccess: (res) => {
      if (isTwoFactorRequired(res)) {
        sessionStorage.setItem(PENDING_2FA_KEY, res.pending_token);
        router.push(`/login/2fa?next=${encodeURIComponent(next)}`);
        return;
      }
      adopt(res);
      router.replace(next);
    },
  });

  const err = mutation.error;
  const message =
    err instanceof ApiError
      ? err.status === 401
        ? 'Incorrect email or password.'
        : err.status === 429
          ? 'Too many attempts. Try again shortly.'
          : err.message
      : err
        ? 'Something went wrong. Try again.'
        : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Sign in</CardTitle>
        <CardDescription>Welcome back to Fluxboard.</CardDescription>
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
          <Field id="password" label="Password">
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <div className="text-right">
            <Link href="/forgot-password" className="text-sm text-primary hover:underline">
              Forgot password?
            </Link>
          </div>
          <Button type="submit" className="w-full" disabled={mutation.isPending}>
            {mutation.isPending ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>

        <div className="my-4 flex items-center gap-3 text-xs text-muted-foreground">
          <span className="h-px flex-1 bg-border" />
          OR
          <span className="h-px flex-1 bg-border" />
        </div>

        <a
          href={`${API_BASE}/auth/oauth/google/start?redirect=${encodeURIComponent(
            `/oauth/google/callback?next=${encodeURIComponent(next)}`,
          )}`}
        >
          <Button variant="outline" className="w-full">
            Continue with Google
          </Button>
        </a>

        <p className="mt-6 text-center text-sm text-muted-foreground">
          No account?{' '}
          <Link href="/register" className="text-primary hover:underline">
            Create one
          </Link>
        </p>
      </CardContent>
    </Card>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
