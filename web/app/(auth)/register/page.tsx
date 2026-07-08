'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { MailCheck } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { register } from '@/lib/api/auth';
import { ApiError } from '@/lib/api/client';

export default function RegisterPage() {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const mutation = useMutation({
    mutationFn: () => register({ name, email, password }),
  });

  const err = mutation.error;
  const fieldErrors = err instanceof ApiError ? err.fieldErrors() : {};
  const banner =
    err instanceof ApiError && err.status !== 422
      ? err.message
      : err && !(err instanceof ApiError)
        ? 'Something went wrong. Try again.'
        : null;

  if (mutation.isSuccess) {
    return (
      <Card>
        <CardHeader>
          <MailCheck className="h-8 w-8 text-primary" />
          <CardTitle>Check your inbox</CardTitle>
          <CardDescription>
            If that email is valid, we&apos;ve sent a verification link. Click it to activate your
            account, then sign in.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Link href="/login">
            <Button variant="outline" className="w-full">
              Back to sign in
            </Button>
          </Link>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Create your account</CardTitle>
        <CardDescription>Start managing projects in minutes.</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          <FormError message={banner} />
          <Field id="name" label="Name" error={fieldErrors.name}>
            <Input
              id="name"
              autoComplete="name"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field id="email" label="Email" error={fieldErrors.email}>
            <Input
              id="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </Field>
          <Field id="password" label="Password" error={fieldErrors.password}>
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
          <Button type="submit" className="w-full" disabled={mutation.isPending}>
            {mutation.isPending ? 'Creating…' : 'Create account'}
          </Button>
        </form>
        <p className="mt-6 text-center text-sm text-muted-foreground">
          Already have an account?{' '}
          <Link href="/login" className="text-primary hover:underline">
            Sign in
          </Link>
        </p>
      </CardContent>
    </Card>
  );
}
