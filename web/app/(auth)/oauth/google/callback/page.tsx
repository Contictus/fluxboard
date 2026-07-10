'use client';

import { useRouter } from 'next/navigation';
import { useEffect, useState } from 'react';
import { Loader2, XCircle } from 'lucide-react';

import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { setAccessToken } from '@/lib/api/client';
import { sanitizeNext } from '@/lib/nav/safe-next';

// The backend OAuth callback redirects here as
//   /oauth/google/callback?next=<path>#access_token=<jwt>
// The access token arrives in the URL fragment (kept out of server logs/Referer);
// we lift it into memory and continue to the app. The refresh cookie is already set.
export default function OAuthCallbackPage() {
  const router = useRouter();
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const hash = window.location.hash.startsWith('#')
      ? window.location.hash.slice(1)
      : window.location.hash;
    const token = new URLSearchParams(hash).get('access_token');
    const next = sanitizeNext(new URLSearchParams(window.location.search).get('next'));

    if (!token) {
      setFailed(true);
      const t = setTimeout(() => router.replace('/login'), 2500);
      return () => clearTimeout(t);
    }
    setAccessToken(token);
    // Strip the fragment before navigating so the token doesn't linger in history.
    router.replace(next);
  }, [router]);

  return (
    <Card>
      <CardHeader>
        {failed ? (
          <XCircle className="h-8 w-8 text-destructive" />
        ) : (
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        )}
        <CardTitle>{failed ? 'Sign-in failed' : 'Signing you in…'}</CardTitle>
        <CardDescription>
          {failed ? 'Redirecting you back to sign in.' : 'Completing Google authentication.'}
        </CardDescription>
      </CardHeader>
    </Card>
  );
}
