'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useOrg } from '@/lib/org/context';
import { useAuth } from '@/lib/auth/context';
import { useToast } from '@/components/ui/toast';
import { useOrgMembers } from '@/lib/org/use-members';
import { deleteOrg, transferOwnership } from '@/lib/api/orgs';

export default function DangerSettingsPage() {
  const { isOwner } = useOrg();

  if (!isOwner) {
    return (
      <p className="max-w-2xl rounded-md border bg-secondary/40 p-4 text-sm text-muted-foreground">
        Only the organization owner can transfer ownership or delete the organization.
      </p>
    );
  }

  return (
    <div className="max-w-2xl space-y-10">
      <TransferSection />
      <DeleteSection />
    </div>
  );
}

function TransferSection() {
  const { orgId } = useOrg();
  const { userId } = useAuth();
  const { members, isLoading } = useOrgMembers();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const candidates = members.filter((m) => m.user_id !== userId);
  const [target, setTarget] = useState('');
  const [confirm, setConfirm] = useState('');

  const selected = candidates.find((m) => m.user_id === target);
  const confirmWord = selected ? (selected.name || selected.email) : '';

  const transfer = useMutation({
    mutationFn: () => transferOwnership(orgId, target),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['orgs'] });
      queryClient.invalidateQueries({ queryKey: ['org-members', orgId] });
      setTarget('');
      setConfirm('');
      toast({ title: 'Ownership transferred', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t transfer ownership', variant: 'error' }),
  });

  return (
    <section>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
        Transfer ownership
      </h2>
      <div className="space-y-3 rounded-md border border-destructive/30 p-4">
        <p className="text-sm text-muted-foreground">
          Hand the OWNER role to another member. You become an admin. This can’t be undone by you
          afterwards — only the new owner can transfer it back.
        </p>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading members…</p>
        ) : candidates.length === 0 ? (
          <p className="text-sm text-muted-foreground">No other members to transfer to.</p>
        ) : (
          <>
            <select
              value={target}
              onChange={(e) => {
                setTarget(e.target.value);
                setConfirm('');
              }}
              className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
            >
              <option value="">Select a member…</option>
              {candidates.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  {m.name || m.email}
                </option>
              ))}
            </select>
            {target ? (
              <div className="space-y-2">
                <p className="text-sm">
                  Type <span className="font-mono font-medium">{confirmWord}</span> to confirm.
                </p>
                <Input value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder={confirmWord} />
                <Button
                  variant="destructive"
                  onClick={() => transfer.mutate()}
                  disabled={transfer.isPending || confirm !== confirmWord}
                >
                  {transfer.isPending ? 'Transferring…' : 'Transfer ownership'}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}

function DeleteSection() {
  const { orgId, org, slug } = useOrg();
  const router = useRouter();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [confirm, setConfirm] = useState('');

  const del = useMutation({
    mutationFn: () => deleteOrg(orgId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['org', orgId] });
      queryClient.invalidateQueries({ queryKey: ['orgs'] });
      toast({ title: 'Organization deleted', variant: 'success' });
      // The org shell reads deleted_at and shows the restore CTA within grace.
      router.push(`/app/${slug}`);
    },
    onError: () => toast({ title: 'Couldn’t delete the organization', variant: 'error' }),
  });

  return (
    <section>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
        Delete organization
      </h2>
      <div className="space-y-3 rounded-md border border-destructive/40 bg-destructive/5 p-4">
        <p className="text-sm text-muted-foreground">
          Soft-deletes the organization and everything in it. You can restore it during a short
          grace window; after that it’s permanently purged.
        </p>
        <p className="text-sm">
          Type the slug <span className="font-mono font-medium">{org.slug}</span> to confirm.
        </p>
        <Input value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder={org.slug} />
        <Button
          variant="destructive"
          onClick={() => del.mutate()}
          disabled={del.isPending || confirm !== org.slug}
        >
          {del.isPending ? 'Deleting…' : 'Delete organization'}
        </Button>
      </div>
    </section>
  );
}
