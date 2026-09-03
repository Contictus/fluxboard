'use client';

import { createContext, useContext, useEffect, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';

import { listMyOrgs, resolveOrgBySlug } from '@/lib/api/orgs';
import type { Org, Role } from '@/lib/api/types';

interface OrgContextValue {
  /** URL slug the shell was mounted for. */
  slug: string;
  /** Resolved org id (UUID) for every `/orgs/{orgId}/…` call. */
  orgId: string;
  /** Full org row (name, deleted_at, logo_key). */
  org: Org;
  /** Caller's role in this org (from the membership list). */
  role: Role;
  isAdmin: boolean;
  isOwner: boolean;
}

const OrgContext = createContext<OrgContextValue | null>(null);

/**
 * Resolves the `[orgSlug]` route param into an org context. The membership list
 * (`GET /orgs`) is the only source of the caller's role, and the slug resolver
 * carries full org data. Both requests run in parallel. A slug the caller
 * isn't a member of bounces to the org switcher.
 */
export function OrgProvider({ slug, children }: { slug: string; children: ReactNode }) {
  const router = useRouter();

  const memberships = useQuery({ queryKey: ['orgs'], queryFn: listMyOrgs });
  const membership = memberships.data?.find((m) => m.slug === slug);

  const org = useQuery({
    queryKey: ['org-by-slug', slug],
    queryFn: () => resolveOrgBySlug(slug),
  });

  // Not a member of this slug → back to the switcher (once the list has loaded).
  useEffect(() => {
    if (memberships.isSuccess && !membership) router.replace('/app');
  }, [memberships.isSuccess, membership, router]);

  if (memberships.isLoading || org.isLoading) {
    return <ShellLoading />;
  }

  if (memberships.isError) {
    return <ShellError message="Couldn't load your organizations." />;
  }

  if (!membership) {
    // Redirect effect is in flight; render nothing to avoid a flash.
    return null;
  }

  if (org.isError || !org.data) {
    return <ShellError message="Couldn't load this organization." />;
  }

  // Membership data remains authority for caller role and access to slug.
  if (org.data.id !== membership.org_id) {
    return <ShellError message="Organization membership changed. Please reload." />;
  }

  const role = membership.role;
  const value: OrgContextValue = {
    slug,
    orgId: membership.org_id,
    org: org.data,
    role,
    isAdmin: role === 'OWNER' || role === 'ADMIN',
    isOwner: role === 'OWNER',
  };

  return <OrgContext.Provider value={value}>{children}</OrgContext.Provider>;
}

export function useOrg(): OrgContextValue {
  const ctx = useContext(OrgContext);
  if (!ctx) throw new Error('useOrg must be used within <OrgProvider>');
  return ctx;
}

function ShellLoading() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-sm text-muted-foreground">Loading workspace…</p>
    </div>
  );
}

function ShellError({ message }: { message: string }) {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-sm text-destructive">{message}</p>
    </div>
  );
}
