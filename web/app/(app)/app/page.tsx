'use client';

import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { Building2, Plus, ChevronRight, LogOut } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
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
    <div className="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-6 p-6 animate-fade-in">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Your organizations</h1>
          <p className="text-sm text-muted-foreground">Select a workspace or create a new one.</p>
        </div>
        <Link href="/account/profile" className="text-sm text-muted-foreground hover:text-foreground transition-colors">
          Account
        </Link>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-[72px] w-full rounded-lg" />
          ))}
        </div>
      ) : isError ? (
        <Card className="p-8 text-center">
          <p className="text-sm text-destructive">Couldn&apos;t load your organizations.</p>
        </Card>
      ) : orgs && orgs.length > 0 ? (
        <div className="stagger-children space-y-2">
          {orgs.map((o) => (
            <Link key={o.org_id} href={`/app/${o.slug}`}>
              <Card className="flex items-center justify-between p-4 transition-all duration-200 hover:bg-secondary/50 hover:-translate-y-0.5">
                <div className="flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Building2 className="h-5 w-5" />
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
        <Card className="p-10 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
            <Building2 className="h-6 w-6 text-muted-foreground" />
          </div>
          <p className="font-medium">No organizations yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first organization to get started.
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
        className="flex items-center justify-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <LogOut className="h-3.5 w-3.5" />
        Sign out
      </Link>
    </div>
  );
}
