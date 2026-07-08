'use client';

import { useParams, useRouter } from 'next/navigation';
import { useEffect, useRef } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Loader2, XCircle } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { acceptInvitation } from '@/lib/api/auth';
import { ApiError } from '@/lib/api/client';
import { useAuth } from '@/lib/auth/context';

// Invitation landing (docs/02 §2, FR-TEN-004): logged-in users accept immediately;
// logged-out users are bounced to login with a post-login return to this page.
export default function InvitePage() {
  const router = useRouter();
  const { token } = useParams<{ token: string }>();
  const { loading, isAuthenticated } = useAuth();
  const fired = useRef(false);

  const mutation = useMutation({
    mutationFn: () => acceptInvitation(token),
    onSuccess: () => router.replace('/app'),
  });

  useEffect(() => {
    if (loading || fired.current) return;
    if (!isAuthenticated) {
      const next = encodeURIComponent(`/invite/${token}`);
      router.replace(`/login?next=${next}`);
      return;
    }
    fired.current = true;
    mutation.mutate();
  }, [loading, isAuthenticated, token, router, mutation]);

  const err = mutation.error;
  const message =
    err instanceof ApiError
      ? err.status === 404
        ? 'This invitation is invalid or has expired.'
        : err.status === 409
          ? 'You have already accepted this invitation.'
          : err.message
      : err
        ? 'Something went wrong accepting the invitation.'
        : null;

  if (mutation.isError) {
    return (
      <Card>
        <CardHeader>
          <XCircle className="h-8 w-8 text-destructive" />
          <CardTitle>Couldn&apos;t accept invitation</CardTitle>
          <CardDescription>{message}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" className="w-full" onClick={() => router.replace('/app')}>
            Go to your workspace
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        <CardTitle>Accepting invitation…</CardTitle>
        <CardDescription>Adding you to the organization.</CardDescription>
      </CardHeader>
    </Card>
  );
}
