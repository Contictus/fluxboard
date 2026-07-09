'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Field } from '@/components/auth/field';
import { useOrg } from '@/lib/org/context';
import { createInvitation } from '@/lib/api/orgs';
import { ApiError } from '@/lib/api/client';
import type { Role } from '@/lib/api/types';

const INVITE_ROLES: Role[] = ['ADMIN', 'MEMBER', 'GUEST'];

interface Outcome {
  email: string;
  status: 'sent' | 'exists' | 'invalid' | 'error';
  message: string;
}

// Split on commas / whitespace / newlines; drop blanks; de-dupe (case-insensitive).
function parseEmails(raw: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const tok of raw.split(/[\s,]+/)) {
    const e = tok.trim();
    if (!e) continue;
    const key = e.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(e);
  }
  return out;
}

export default function InviteMembersPage() {
  const { orgId, slug } = useOrg();
  const queryClient = useQueryClient();

  const [raw, setRaw] = useState('');
  const [role, setRole] = useState<Role>('MEMBER');
  const [sending, setSending] = useState(false);
  const [outcomes, setOutcomes] = useState<Outcome[]>([]);
  const [seatLimit, setSeatLimit] = useState(false);

  const emails = parseEmails(raw);

  async function submit() {
    if (emails.length === 0 || sending) return;
    setSending(true);
    setSeatLimit(false);
    const results: Outcome[] = [];
    for (const email of emails) {
      try {
        await createInvitation(orgId, { email, role });
        results.push({ email, status: 'sent', message: 'Invitation sent' });
      } catch (e) {
        if (e instanceof ApiError && e.status === 402) {
          // Plan seat cap hit — stop the run and surface the upgrade CTA.
          setSeatLimit(true);
          break;
        }
        if (e instanceof ApiError && e.status === 409) {
          results.push({ email, status: 'exists', message: 'Already a member or invited' });
        } else if (e instanceof ApiError && e.status === 422) {
          results.push({ email, status: 'invalid', message: 'Invalid email' });
        } else {
          results.push({ email, status: 'error', message: 'Failed to send' });
        }
      }
    }
    setOutcomes(results);
    setSending(false);
    if (results.some((r) => r.status === 'sent')) {
      queryClient.invalidateQueries({ queryKey: ['invitations', orgId] });
    }
  }

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Invite people</h2>
        <Link href={`/app/${slug}/settings/members`} className="text-sm text-muted-foreground hover:text-foreground">
          Back to members
        </Link>
      </div>

      {seatLimit ? (
        <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-4 text-sm">
          <p className="font-medium text-amber-700 dark:text-amber-400">Seat limit reached</p>
          <p className="mt-1 text-muted-foreground">
            Your plan is at its member limit. Upgrade to invite more people.
          </p>
          <Link href={`/app/${slug}/billing`} className="mt-2 inline-block">
            <Button size="sm">Go to billing</Button>
          </Link>
        </div>
      ) : null}

      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Field id="emails" label="Email addresses">
          <textarea
            id="emails"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            rows={4}
            placeholder="alice@acme.com, bob@acme.com"
            className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <p className="text-xs text-muted-foreground">
            Separate multiple addresses with commas, spaces or new lines.
            {emails.length > 0 ? ` ${emails.length} recipient${emails.length === 1 ? '' : 's'}.` : ''}
          </p>
        </Field>

        <Field id="invite-role" label="Role">
          <select
            id="invite-role"
            value={role}
            onChange={(e) => setRole(e.target.value as Role)}
            className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
          >
            {INVITE_ROLES.map((r) => (
              <option key={r} value={r}>
                {r.charAt(0) + r.slice(1).toLowerCase()}
              </option>
            ))}
          </select>
        </Field>

        <Button type="submit" disabled={sending || emails.length === 0}>
          {sending ? 'Sending…' : `Send ${emails.length || ''} invitation${emails.length === 1 ? '' : 's'}`.trim()}
        </Button>
      </form>

      {outcomes.length > 0 ? (
        <div className="divide-y rounded-md border text-sm">
          {outcomes.map((o) => (
            <div key={o.email} className="flex items-center justify-between p-3">
              <span className="truncate">{o.email}</span>
              <span
                className={
                  o.status === 'sent'
                    ? 'text-green-600'
                    : o.status === 'exists'
                      ? 'text-muted-foreground'
                      : 'text-destructive'
                }
              >
                {o.message}
              </span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
