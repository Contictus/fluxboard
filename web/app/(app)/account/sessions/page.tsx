'use client';

import { useRouter } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Monitor } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useToast } from '@/components/ui/toast';
import { useAuth } from '@/lib/auth/context';
import { listSessions, revokeSession, logoutAll } from '@/lib/api/auth';

function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString();
}

export default function SessionsPage() {
  const queryClient = useQueryClient();
  const router = useRouter();
  const { logout } = useAuth();
  const { toast } = useToast();

  const { data, isLoading } = useQuery({
    queryKey: ['sessions'],
    queryFn: listSessions,
  });

  const revoke = useMutation({
    mutationFn: (id: string) => revokeSession(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['sessions'] });
      toast({ title: 'Session revoked', variant: 'success' });
    },
  });

  const logoutEverywhere = useMutation({
    mutationFn: logoutAll,
    onSuccess: async () => {
      await logout();
      router.replace('/login');
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>Active sessions</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {data?.items.map((s) => (
              <li key={s.id} className="flex items-center justify-between gap-4 p-4">
                <div className="flex items-center gap-3">
                  <Monitor className="h-5 w-5 text-muted-foreground" />
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">
                      {s.user_agent || 'Unknown device'}
                      {s.current ? (
                        <span className="ml-2 rounded-full bg-secondary px-2 py-0.5 text-xs">
                          This device
                        </span>
                      ) : null}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {s.ip ? `${s.ip} · ` : ''}last used {formatWhen(s.last_used_at)}
                    </p>
                  </div>
                </div>
                {!s.current ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => revoke.mutate(s.id)}
                    disabled={revoke.isPending}
                  >
                    Revoke
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}

        <Button
          variant="destructive"
          onClick={() => logoutEverywhere.mutate()}
          disabled={logoutEverywhere.isPending}
        >
          Log out of all sessions
        </Button>
      </CardContent>
    </Card>
  );
}
