'use client';

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useToast } from '@/components/ui/toast';
import { listMyOrgs } from '@/lib/api/orgs';
import { getPrefs, setPref, NOTIFICATION_CATEGORIES } from '@/lib/api/notifications';
import type { NotificationPref } from '@/lib/api/types';

function Toggle({
  checked,
  onChange,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <input
      type="checkbox"
      className="h-4 w-4 accent-[hsl(var(--primary))]"
      checked={checked}
      disabled={disabled}
      onChange={(e) => onChange(e.target.checked)}
    />
  );
}

function OrgPrefs({ orgId }: { orgId: string }) {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const { data: stored, isLoading } = useQuery({
    queryKey: ['notif-prefs', orgId],
    queryFn: () => getPrefs(orgId),
  });

  // Merge stored rows over the default (both true) for every category.
  const byCat = new Map<string, NotificationPref>();
  for (const p of stored ?? []) byCat.set(p.category, p);

  const mutate = useMutation({
    mutationFn: (pref: NotificationPref) => setPref(orgId, pref),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['notif-prefs', orgId] });
    },
    onError: () => toast({ title: 'Could not save preference', variant: 'error' }),
  });

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading…</p>;

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-muted-foreground">
            <th className="py-2 font-medium">Category</th>
            <th className="w-20 py-2 text-center font-medium">Email</th>
            <th className="w-20 py-2 text-center font-medium">In-app</th>
          </tr>
        </thead>
        <tbody>
          {NOTIFICATION_CATEGORIES.map((cat) => {
            const cur = byCat.get(cat.key) ?? { category: cat.key, email: true, in_app: true };
            return (
              <tr key={cat.key} className="border-b last:border-0">
                <td className="py-3">{cat.label}</td>
                <td className="py-3 text-center">
                  <Toggle
                    checked={cur.email}
                    disabled={mutate.isPending}
                    onChange={(v) => mutate.mutate({ ...cur, email: v })}
                  />
                </td>
                <td className="py-3 text-center">
                  <Toggle
                    checked={cur.in_app}
                    disabled={mutate.isPending}
                    onChange={(v) => mutate.mutate({ ...cur, in_app: v })}
                  />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export default function NotificationsPage() {
  const { data: orgs, isLoading } = useQuery({ queryKey: ['orgs'], queryFn: listMyOrgs });
  const [orgId, setOrgId] = useState<string>('');

  useEffect(() => {
    if (!orgId && orgs && orgs.length > 0) setOrgId(orgs[0]!.org_id);
  }, [orgs, orgId]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Notifications</CardTitle>
        <CardDescription>
          Preferences are set per organization. Transactional security emails are always sent.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : orgs && orgs.length > 0 ? (
          <>
            <div className="flex items-center gap-2">
              <label htmlFor="org" className="text-sm text-muted-foreground">
                Organization
              </label>
              <select
                id="org"
                className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                value={orgId}
                onChange={(e) => setOrgId(e.target.value)}
              >
                {orgs.map((o) => (
                  <option key={o.org_id} value={o.org_id}>
                    {o.name}
                  </option>
                ))}
              </select>
            </div>
            {orgId ? <OrgPrefs orgId={orgId} /> : null}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            Join or create an organization to configure notifications.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
