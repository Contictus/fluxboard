'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useEffect, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, Loader2, X } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { createOrg, isSlugAvailable } from '@/lib/api/orgs';
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

export default function NewOrganizationPage() {
  const router = useRouter();
  const queryClient = useQueryClient();

  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [slugEdited, setSlugEdited] = useState(false);
  const [slugState, setSlugState] = useState<SlugState>('idle');

  // Keep the slug in sync with the name until the user edits it directly.
  useEffect(() => {
    if (!slugEdited) setSlug(slugify(name));
  }, [name, slugEdited]);

  // Debounced live availability check.
  useEffect(() => {
    if (!slug) {
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
  }, [slug]);

  const mutation = useMutation({
    mutationFn: () => createOrg({ name, slug }),
    onSuccess: async (org) => {
      await queryClient.invalidateQueries({ queryKey: ['orgs'] });
      router.replace(`/app/${org.slug}`);
    },
  });

  const err = mutation.error;
  const banner =
    err instanceof ApiError
      ? err.status === 409
        ? 'That slug is already taken.'
        : err.status !== 422
          ? err.message
          : null
      : err
        ? 'Something went wrong.'
        : null;
  const fieldErrors = err instanceof ApiError ? err.fieldErrors() : {};

  return (
    <div className="mx-auto flex min-h-screen max-w-md flex-col justify-center p-6">
      <Card>
        <CardHeader>
          <CardTitle>Create an organization</CardTitle>
          <CardDescription>Your team&apos;s workspace for projects and billing.</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              mutation.mutate();
            }}
          >
            <FormError message={banner} />
            <Field id="name" label="Name" error={fieldErrors.name}>
              <Input
                id="name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Acme Inc."
              />
            </Field>
            <Field id="slug" label="Slug" error={fieldErrors.slug}>
              <div className="relative">
                <Input
                  id="slug"
                  required
                  value={slug}
                  onChange={(e) => {
                    setSlugEdited(true);
                    setSlug(slugify(e.target.value));
                  }}
                  placeholder="acme"
                />
                <span className="absolute right-3 top-1/2 -translate-y-1/2">
                  {slugState === 'checking' && (
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                  )}
                  {slugState === 'available' && <Check className="h-4 w-4 text-green-600" />}
                  {(slugState === 'taken' || slugState === 'invalid') && (
                    <X className="h-4 w-4 text-destructive" />
                  )}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {slugState === 'taken'
                  ? 'This slug is taken.'
                  : slugState === 'invalid'
                    ? 'At least 3 characters.'
                    : 'Used in your workspace URL: /app/' + (slug || 'your-org')}
              </p>
            </Field>
            <Button
              type="submit"
              className="w-full"
              disabled={mutation.isPending || slugState === 'taken' || slugState === 'invalid'}
            >
              {mutation.isPending ? 'Creating…' : 'Create organization'}
            </Button>
          </form>
          <p className="mt-6 text-center text-sm text-muted-foreground">
            <Link href="/app" className="text-primary hover:underline">
              Back to organizations
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
