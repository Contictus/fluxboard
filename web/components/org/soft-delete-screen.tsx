'use client';

import { useRouter } from 'next/navigation';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useOrg } from '@/lib/org/context';
import { restoreOrg } from '@/lib/api/orgs';

// Full-screen grace state for a soft-deleted org (02 §9, 7.4.3). The API exposes
// `deleted_at` but not `purge_after`, so we cannot show a live countdown
// (ADR-016) — copy stays qualitative. Restore is OWNER-only.
export function SoftDeleteScreen() {
  const { orgId, org, isOwner } = useOrg();
  const router = useRouter();
  const queryClient = useQueryClient();

  const restore = useMutation({
    mutationFn: () => restoreOrg(orgId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['org', orgId] });
      router.refresh();
    },
  });

  const deletedAt = org.deleted_at ? new Date(org.deleted_at) : null;

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="max-w-md border-destructive/40">
        <CardHeader>
          <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-md bg-destructive/10">
            <Trash2 className="h-5 w-5 text-destructive" />
          </div>
          <CardTitle>{org.name} is scheduled for deletion</CardTitle>
          <CardDescription>
            This organization was deleted
            {deletedAt ? ` on ${deletedAt.toLocaleDateString()}` : ''} and is in a grace period
            before it is permanently removed. Restore it to regain access.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {isOwner ? (
            <>
              <Button
                onClick={() => restore.mutate()}
                disabled={restore.isPending}
                className="w-full"
              >
                {restore.isPending ? 'Restoring…' : 'Restore organization'}
              </Button>
              {restore.isError ? (
                <p className="text-sm text-destructive">
                  Couldn&apos;t restore the organization. Try again.
                </p>
              ) : null}
            </>
          ) : (
            <p className="text-sm text-muted-foreground">
              Only an owner can restore this organization. Ask an owner to restore it.
            </p>
          )}
          <Button variant="outline" className="w-full" onClick={() => router.replace('/app')}>
            Back to organizations
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
