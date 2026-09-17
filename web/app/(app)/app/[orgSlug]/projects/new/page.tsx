'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FormError } from '@/components/auth/field';
import { useOrg } from '@/lib/org/context';
import { createProject } from '@/lib/api/projects';
import { ApiError } from '@/lib/api/client';
import type { Visibility } from '@/lib/api/types';
import { cn } from '@/lib/utils';

// A project key is 2–10 upper-case letters/digits starting with a letter
// (mirrors ValidKey server-side). We suggest one from the name until edited.
function suggestKey(name: string): string {
  const cleaned = name.toUpperCase().replace(/[^A-Z0-9]/g, '');
  const withLetterHead = cleaned.replace(/^[0-9]+/, '');
  return withLetterHead.slice(0, 6);
}

const COLORS = ['#2f7468', '#6e9f8d', '#d49a52', '#c36d58', '#8d9a65', '#5f8ea0', '#a77c91', '#64748b'];

export default function NewProjectPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { orgId, slug } = useOrg();

  const [name, setName] = useState('');
  const [key, setKey] = useState('');
  const [keyEdited, setKeyEdited] = useState(false);
  const [description, setDescription] = useState('');
  const [color, setColor] = useState(COLORS[0] as string);
  const [visibility, setVisibility] = useState<Visibility>('org');

  useEffect(() => {
    if (!keyEdited) setKey(suggestKey(name));
  }, [name, keyEdited]);

  const mutation = useMutation({
    mutationFn: () => createProject(orgId, { key, name, description, color, visibility }),
    onSuccess: async (project) => {
      await queryClient.invalidateQueries({ queryKey: ['projects', orgId] });
      router.replace(`/app/${slug}/projects/${project.key}`);
    },
  });

  const err = mutation.error;
  const fieldErrors = err instanceof ApiError ? err.fieldErrors() : {};
  const banner =
    err instanceof ApiError
      ? err.status === 409
        ? 'That project key is already taken.'
        : err.status === 402
          ? 'Your plan has reached its project limit. Upgrade to add more.'
          : err.status !== 422
            ? err.message
            : null
      : err
        ? 'Something went wrong.'
        : null;
  const planLimited = err instanceof ApiError && err.status === 402;

  return (
    <div className="mx-auto max-w-2xl px-6 py-12 lg:py-16">
      <Card>
        <CardHeader>
          <CardTitle>New project</CardTitle>
          <CardDescription>
            Starts with a default board (To Do / In Progress / Done) you can edit later.
          </CardDescription>
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
            {planLimited ? (
              <Link
                href={`/app/${slug}/billing`}
                className="block text-sm text-primary hover:underline"
              >
                Go to billing →
              </Link>
            ) : null}

            <Field id="name" label="Name" error={fieldErrors.name}>
              <Input
                id="name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Website Redesign"
              />
            </Field>

            <Field id="key" label="Key" error={fieldErrors.key}>
              <Input
                id="key"
                required
                value={key}
                onChange={(e) => {
                  setKeyEdited(true);
                  setKey(e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 10));
                }}
                placeholder="WEB"
                className="font-mono uppercase"
              />
              <p className="text-xs text-muted-foreground">
                Short prefix for task numbers, e.g. {key || 'WEB'}-42. 2–10 letters/digits.
              </p>
            </Field>

            <Field id="description" label="Description (optional)">
              <textarea
                id="description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={3}
                placeholder="What is this project about?"
                className="flex w-full rounded-xl border-0 bg-secondary/60 px-4 py-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              />
            </Field>

            <div className="space-y-1.5">
              <span className="text-sm font-medium">Color</span>
              <div className="flex flex-wrap gap-2">
                {COLORS.map((c) => (
                  <button
                    key={c}
                    type="button"
                    onClick={() => setColor(c)}
                    aria-label={`Select color ${c}`}
                    className={cn(
                      'h-7 w-7 rounded-full ring-offset-2 ring-offset-background transition',
                      color === c && 'ring-2 ring-ring',
                    )}
                    style={{ backgroundColor: c }}
                  />
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <span className="text-sm font-medium">Visibility</span>
              <div className="grid gap-2 sm:grid-cols-2">
                {(
                  [
                    { v: 'org', label: 'Organization', hint: 'All org members can see it.' },
                    { v: 'private', label: 'Private', hint: 'Only project members.' },
                  ] as const
                ).map(({ v, label, hint }) => (
                  <button
                    key={v}
                    type="button"
                    onClick={() => setVisibility(v)}
                    className={cn(
                      'rounded-2xl bg-secondary/45 p-4 text-left text-sm transition-colors',
                      visibility === v ? 'bg-primary/10 text-foreground ring-1 ring-primary/30' : 'hover:bg-secondary',
                    )}
                  >
                    <span className="font-medium">{label}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">{hint}</span>
                  </button>
                ))}
              </div>
            </div>

            <Button type="submit" className="w-full" disabled={mutation.isPending || !name || !key}>
              {mutation.isPending ? 'Creating…' : 'Create project'}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-muted-foreground">
            <Link href={`/app/${slug}/projects`} className="text-primary hover:underline">
              Back to projects
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
