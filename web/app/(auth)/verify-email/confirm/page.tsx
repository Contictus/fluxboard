'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { Suspense, useEffect, useRef } from 'react';
import { useMutation } from '@tanstack/react-query';
import { CheckCircle2, XCircle, Loader2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { confirmEmail } from '@/lib/api/auth';

function ConfirmInner() {
  const params = useSearchParams();
  const token = params.get('token') ?? '';
  const fired = useRef(false);

  const mutation = useMutation({ mutationFn: () => confirmEmail(token) });

  // Consume the token exactly once on mount.
  useEffect(() => {
    if (fired.current || !token) return;
    fired.current = true;
    mutation.mutate();
  }, [token, mutation]);

  let icon = <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />;
  let title = 'Verifying…';
  let desc = 'Confirming your email address.';

  if (!token) {
    icon = <XCircle className="h-8 w-8 text-destructive" />;
    title = 'Invalid link';
    desc = 'This verification link is missing its token.';
  } else if (mutation.isSuccess) {
    icon = <CheckCircle2 className="h-8 w-8 text-primary" />;
    title = 'Email verified';
    desc = 'Your email is confirmed. You can now sign in.';
  } else if (mutation.isError) {
    icon = <XCircle className="h-8 w-8 text-destructive" />;
    title = 'Link expired';
    desc = 'This verification link is invalid or has already been used.';
  }

  return (
    <Card>
      <CardHeader>
        {icon}
        <CardTitle>{title}</CardTitle>
        <CardDescription>{desc}</CardDescription>
      </CardHeader>
      {(mutation.isSuccess || mutation.isError || !token) && (
        <CardContent>
          <Link href="/login">
            <Button className="w-full" variant={mutation.isSuccess ? 'default' : 'outline'}>
              Go to sign in
            </Button>
          </Link>
        </CardContent>
      )}
    </Card>
  );
}

export default function ConfirmEmailPage() {
  return (
    <Suspense>
      <ConfirmInner />
    </Suspense>
  );
}
