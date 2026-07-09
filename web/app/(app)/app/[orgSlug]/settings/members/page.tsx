'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Loader2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { useOrg } from '@/lib/org/context';
import { useAuth } from '@/lib/auth/context';
import { useToast } from '@/components/ui/toast';
import { useOrgMembers } from '@/lib/org/use-members';
import {
  changeMemberRole,
  listInvitations,
  removeMember,
  resendInvitation,
  revokeInvitation,
} from '@/lib/api/orgs';
import type { OrgMember, Role } from '@/lib/api/types';

// Roles assignable via the dropdown. OWNER is set only through transfer-ownership
// (Danger zone), never here (ADR-019 §3).
const ASSIGNABLE_ROLES: Role[] = ['ADMIN', 'MEMBER', 'GUEST'];

export default function MembersSettingsPage() {
  const { slug } = useOrg();

  return (
    <div className="max-w-3xl space-y-10">
      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Members</h2>
          <Link href={`/app/${slug}/settings/members/invite`}>
            <Button size="sm">Invite people</Button>
          </Link>
        </div>
        <MembersTable />
      </section>
      <PendingInvites />
    </div>
  );
}

function MembersTable() {
  const { orgId } = useOrg();
  const { userId } = useAuth();
  const { members, isLoading } = useOrgMembers();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['org-members', orgId] });

  const setRole = useMutation({
    mutationFn: (v: { userId: string; role: Role }) => changeMemberRole(orgId, v.userId, v.role),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Role updated', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t update role', variant: 'error' }),
  });

  const remove = useMutation({
    mutationFn: (uid: string) => removeMember(orgId, uid),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Member removed', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t remove member', variant: 'error' }),
  });

  if (isLoading) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading members…
      </p>
    );
  }

  return (
    <div className="divide-y rounded-md border">
      {members.map((m) => (
        <MemberRow
          key={m.user_id}
          member={m}
          isSelf={m.user_id === userId}
          onRole={(role) => setRole.mutate({ userId: m.user_id, role })}
          onRemove={() => remove.mutate(m.user_id)}
          busy={setRole.isPending || remove.isPending}
        />
      ))}
    </div>
  );
}

function MemberRow({
  member,
  isSelf,
  onRole,
  onRemove,
  busy,
}: {
  member: OrgMember;
  isSelf: boolean;
  onRole: (role: Role) => void;
  onRemove: () => void;
  busy: boolean;
}) {
  const [confirming, setConfirming] = useState(false);
  const isOwner = member.role === 'OWNER';
  // Owner and your own row are locked: owner changes via transfer; you can't
  // demote/remove yourself here (use Leave / Danger zone).
  const locked = isOwner || isSelf;

  return (
    <div className="flex flex-wrap items-center gap-3 p-3">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          {member.name || member.email}
          {isSelf ? <span className="ml-2 text-xs text-muted-foreground">(you)</span> : null}
        </p>
        <p className="truncate text-xs text-muted-foreground">{member.email}</p>
      </div>

      {isOwner ? (
        <span className="rounded-full bg-secondary px-2.5 py-1 text-xs font-medium">Owner</span>
      ) : (
        <select
          value={member.role}
          disabled={locked || busy}
          onChange={(e) => onRole(e.target.value as Role)}
          className="h-9 rounded-md border border-input bg-background px-2 text-sm disabled:opacity-50"
          aria-label="Member role"
        >
          {ASSIGNABLE_ROLES.map((r) => (
            <option key={r} value={r}>
              {r.charAt(0) + r.slice(1).toLowerCase()}
            </option>
          ))}
        </select>
      )}

      {locked ? (
        <span className="w-20" />
      ) : confirming ? (
        <span className="flex items-center gap-1 text-xs">
          <button
            onClick={() => {
              onRemove();
              setConfirming(false);
            }}
            className="rounded bg-destructive px-2 py-1 font-medium text-destructive-foreground"
          >
            Remove
          </button>
          <button onClick={() => setConfirming(false)} className="px-1 text-muted-foreground hover:text-foreground">
            Cancel
          </button>
        </span>
      ) : (
        <button
          onClick={() => setConfirming(true)}
          disabled={busy}
          className="text-sm text-destructive hover:underline disabled:opacity-50"
        >
          Remove
        </button>
      )}
    </div>
  );
}

function PendingInvites() {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const invites = useQuery({ queryKey: ['invitations', orgId], queryFn: () => listInvitations(orgId) });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['invitations', orgId] });

  const revoke = useMutation({
    mutationFn: (id: string) => revokeInvitation(orgId, id),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Invitation revoked', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t revoke', variant: 'error' }),
  });

  const resend = useMutation({
    mutationFn: (id: string) => resendInvitation(orgId, id),
    onSuccess: () => toast({ title: 'Invitation re-sent', variant: 'success' }),
    onError: () => toast({ title: 'Couldn’t re-send', variant: 'error' }),
  });

  const items = invites.data ?? [];

  return (
    <section>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Pending invitations</h2>
      {invites.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : items.length === 0 ? (
        <p className="rounded-md border bg-secondary/30 p-4 text-sm text-muted-foreground">
          No pending invitations.
        </p>
      ) : (
        <div className="divide-y rounded-md border">
          {items.map((inv) => (
            <div key={inv.id} className="flex flex-wrap items-center gap-3 p-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{inv.email}</p>
                <p className="text-xs text-muted-foreground">
                  {inv.role.charAt(0) + inv.role.slice(1).toLowerCase()} · expires{' '}
                  {new Date(inv.expires_at).toLocaleDateString()}
                </p>
              </div>
              <button
                onClick={() => resend.mutate(inv.id)}
                disabled={resend.isPending}
                className="text-sm text-primary hover:underline disabled:opacity-50"
              >
                Resend
              </button>
              <button
                onClick={() => revoke.mutate(inv.id)}
                disabled={revoke.isPending}
                className="text-sm text-destructive hover:underline disabled:opacity-50"
              >
                Revoke
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
