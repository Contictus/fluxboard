'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { FormError } from '@/components/auth/field';
import { ApiError } from '@/lib/api/client';
import { useAuth } from '@/lib/auth/context';
import { deleteMe } from '@/lib/api/user';

interface OrgRef {
  id: string;
  slug: string;
  name: string;
}

export default function DangerPage() {
  const router = useRouter();
  const { logout } = useAuth();
  const [confirm, setConfirm] = useState('');
  const [blockers, setBlockers] = useState<OrgRef[] | null>(null);

  const mutation = useMutation({
    mutationFn: deleteMe,
    onSuccess: async () => {
      await logout();
      router.replace('/');
    },
    onError: (err) => {
      // 409 sole_owner_of carries the orgs that must first get a new owner.
      if (err instanceof ApiError && err.status === 409) {
        const list = err.details?.sole_owner_of;
        setBlockers(Array.isArray(list) ? (list as OrgRef[]) : []);
      }
    },
  });

  const genericError =
    mutation.error instanceof ApiError && mutation.error.status !== 409
      ? mutation.error.message
      : mutation.error && !(mutation.error instanceof ApiError)
        ? 'Something went wrong.'
        : null;

  return (
    <Card className="border-destructive/40">
      <CardHeader>
        <CardTitle className="text-destructive">Delete account</CardTitle>
        <CardDescription>
          Permanently deletes your account and signs out everywhere. This cannot be undone.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <FormError message={genericError} />

        {blockers && blockers.length > 0 ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm">
            <p className="font-medium text-destructive">
              You&apos;re the sole owner of these organizations:
            </p>
            <ul className="mt-2 list-disc pl-5">
              {blockers.map((o) => (
                <li key={o.id}>{o.name}</li>
              ))}
            </ul>
            <p className="mt-2 text-muted-foreground">
              Transfer ownership or delete them before deleting your account.
            </p>
          </div>
        ) : null}

        <div className="space-y-2">
          <p className="text-sm">
            Type <span className="font-mono font-medium">DELETE</span> to confirm.
          </p>
          <Input value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder="DELETE" />
        </div>

        <Button
          variant="destructive"
          disabled={confirm !== 'DELETE' || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? 'Deleting…' : 'Delete my account'}
        </Button>
      </CardContent>
    </Card>
  );
}
