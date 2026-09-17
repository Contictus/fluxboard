'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { CheckCircle2 } from 'lucide-react';

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
      <Card className="text-center">
        <CardHeader>
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-success/10 animate-scale-in">
            <CheckCircle2 className="h-7 w-7 text-success" />
          </div>
          <CardTitle className="mt-2">Check your inbox</CardTitle>
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
            {/* Password strength hint */}
            {password.length > 0 ? (
              <div className="mt-2 flex gap-1">
                {[1, 2, 3, 4].map((level) => {
                  const strength =
                    password.length >= 12 && /[A-Z]/.test(password) && /\d/.test(password) ? 4 :
                    password.length >= 10 ? 3 :
                    password.length >= 8 ? 2 : 1;
                  return (
                    <div
                      key={level}
                      className={`h-1 flex-1 rounded-full transition-colors ${
                        level <= strength
                          ? strength <= 1
                            ? 'bg-destructive'
                            : strength <= 2
                              ? 'bg-warning'
                              : 'bg-success'
                          : 'bg-muted'
                      }`}
                    />
                  );
                })}
              </div>
            ) : null}
          </Field>
          <Button type="submit" className="w-full" isLoading={mutation.isPending}>
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
