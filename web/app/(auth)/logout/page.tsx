'use client';

import { useRouter } from 'next/navigation';
import { useEffect, useRef } from 'react';
import { Loader2 } from 'lucide-react';

import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAuth } from '@/lib/auth/context';

// Logout runs client-side: the access token lives in memory (not reachable from a
// server route handler), so we call /auth/logout from the browser, clear local
// state, then return to /login.
export default function LogoutPage() {
  const router = useRouter();
  const { logout } = useAuth();
  const fired = useRef(false);

  useEffect(() => {
    if (fired.current) return;
    fired.current = true;
    void logout().finally(() => router.replace('/login'));
  }, [logout, router]);

  return (
    <Card>
      <CardHeader>
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        <CardTitle>Signing out…</CardTitle>
        <CardDescription>Clearing your session.</CardDescription>
      </CardHeader>
    </Card>
  );
}
