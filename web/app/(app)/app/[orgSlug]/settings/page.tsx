'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, Check, Loader2, X } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Field } from '@/components/auth/field';
import { useOrg } from '@/lib/org/context';
import { useToast } from '@/components/ui/toast';
import { isSlugAvailable, updateOrg } from '@/lib/api/orgs';
import { ApiError } from '@/lib/api/client';

function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40);
}

type SlugState = 'idle' | 'checking' | 'available' | 'taken' | 'invalid';

export default function OrgGeneralSettingsPage() {
  const { orgId, org, slug: currentSlug, isOwner } = useOrg();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const [name, setName] = useState(org.name);
  const [slug, setSlug] = useState(org.slug);
  const [slugState, setSlugState] = useState<SlugState>('idle');

  // Keep drafts in sync if the org changes underneath (e.g. another tab edits it).
  useEffect(() => {
    setName(org.name);
    setSlug(org.slug);
  }, [org.name, org.slug]);

  // Debounced availability check — only when the slug actually changed.
  useEffect(() => {
    if (slug === org.slug) {
      setSlugState('idle');
      return;
    }
    if (slug.length < 3) {
      setSlugState('invalid');
      return;
    }
    setSlugState('checking');
    const t = setTimeout(() => {
      isSlugAvailable(slug)
        .then((ok) => setSlugState(ok ? 'available' : 'taken'))
        .catch(() => setSlugState('idle'));
    }, 400);
    return () => clearTimeout(t);
  }, [slug, org.slug]);

  const saveName = useMutation({
    mutationFn: () => updateOrg(orgId, { name: name.trim() }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['org', orgId] });
      queryClient.invalidateQueries({ queryKey: ['orgs'] });
      toast({ title: 'Organization updated', variant: 'success' });
    },
    onError: () => toast({ title: 'Update failed', variant: 'error' }),
  });

  const saveSlug = useMutation({
    mutationFn: () => updateOrg(orgId, { slug }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['orgs'] });
      queryClient.invalidateQueries({ queryKey: ['org', orgId] });
      toast({ title: 'Slug changed', variant: 'success' });
      // The URL still carries the old slug; move to the new one.
      router.replace(`/app/${slug}/settings`);
    },
    onError: (e) => {
      const msg =
        e instanceof ApiError && e.status === 409
          ? 'That slug is already taken.'
          : e instanceof ApiError && e.status === 403
            ? 'Only the owner can change the slug.'
            : 'Couldn’t change the slug.';
      toast({ title: msg, variant: 'error' });
    },
  });

  return (
    <div className="max-w-2xl space-y-10">
      <section>
        <SectionTitle>Profile</SectionTitle>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim() && name.trim() !== org.name) saveName.mutate();
          }}
        >
          <Field id="org-name" label="Name">
            <Input id="org-name" value={name} onChange={(e) => setName(e.target.value)} required />
          </Field>
          <div className="space-y-1.5">
            <span className="text-sm font-medium">Logo</span>
            <p className="rounded-md border bg-secondary/40 p-3 text-xs text-muted-foreground">
              Logo upload isn’t available yet — the API exposes no organization-logo upload
              endpoint. It will land in a later release.
            </p>
          </div>
          <Button type="submit" disabled={saveName.isPending || !name.trim() || name.trim() === org.name}>
            {saveName.isPending ? 'Saving…' : 'Save changes'}
          </Button>
        </form>
      </section>

      <section>
        <SectionTitle>Workspace URL</SectionTitle>
        {isOwner ? (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (slugState === 'available') saveSlug.mutate();
            }}
          >
            <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>
                Changing the slug changes every link to this workspace. The old slug redirects
                for a short window, then stops working.
              </span>
            </div>
            <Field id="org-slug" label="Slug">
              <div className="relative">
                <Input
                  id="org-slug"
                  value={slug}
                  onChange={(e) => setSlug(slugify(e.target.value))}
                  placeholder="acme"
                  required
                />
                <span className="absolute right-3 top-1/2 -translate-y-1/2">
                  {slugState === 'checking' && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
                  {slugState === 'available' && <Check className="h-4 w-4 text-green-600" />}
                  {(slugState === 'taken' || slugState === 'invalid') && <X className="h-4 w-4 text-destructive" />}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {slugState === 'taken'
                  ? 'This slug is taken.'
                  : slugState === 'invalid'
                    ? 'At least 3 characters.'
                    : `Workspace URL: /app/${slug || currentSlug}`}
              </p>
            </Field>
            <Button type="submit" disabled={saveSlug.isPending || slugState !== 'available'}>
              {saveSlug.isPending ? 'Changing…' : 'Change slug'}
            </Button>
          </form>
        ) : (
          <p className="rounded-md border bg-secondary/40 p-4 text-sm text-muted-foreground">
            Your workspace URL is <span className="font-mono">/app/{org.slug}</span>. Only the
            organization owner can change the slug.
          </p>
        )}
      </section>
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">{children}</h2>;
}
