'use client';

import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { Building2, Plus, ChevronRight } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { listMyOrgs } from '@/lib/api/orgs';

// Org switcher hub (docs/02 §3, FR-TEN-001). NOTE: the sitemap's "auto-redirect
// when exactly one org" lands the user in the org shell (/app/{slug}), which is
// built in a later Phase-7 section; until then we always render the picker so the
// user is never bounced into a not-yet-existing route.
export default function OrgSwitcherPage() {
  const { data: orgs, isLoading, isError } = useQuery({
    queryKey: ['orgs'],
    queryFn: listMyOrgs,
  });

  return (
    <div className="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Your organizations</h1>
          <p className="text-sm text-muted-foreground">Select a workspace or create a new one.</p>
        </div>
        <Link href="/account/profile" className="text-sm text-muted-foreground hover:text-foreground">
          Account
        </Link>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : isError ? (
        <p className="text-sm text-destructive">Couldn&apos;t load your organizations.</p>
      ) : orgs && orgs.length > 0 ? (
        <div className="space-y-2">
          {orgs.map((o) => (
            <Link key={o.org_id} href={`/app/${o.slug}`}>
              <Card className="flex items-center justify-between p-4 transition-colors hover:bg-secondary">
                <div className="flex items-center gap-3">
                  <div className="flex h-9 w-9 items-center justify-center rounded-md bg-secondary">
                    <Building2 className="h-4 w-4" />
                  </div>
                  <div>
                    <p className="font-medium">{o.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {o.slug} · {o.role.toLowerCase()}
                    </p>
                  </div>
                </div>
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              </Card>
            </Link>
          ))}
        </div>
      ) : (
        <Card className="p-8 text-center">
          <p className="text-sm text-muted-foreground">
            You&apos;re not part of any organization yet.
          </p>
        </Card>
      )}

      <Link href="/app/new-organization">
        <Button className="w-full" variant="outline">
          <Plus className="h-4 w-4" /> New organization
        </Button>
      </Link>

      <Link
        href="/logout"
        className="text-center text-sm text-muted-foreground hover:text-foreground"
      >
        Sign out
      </Link>
    </div>
  );
}
