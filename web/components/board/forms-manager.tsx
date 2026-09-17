'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, Copy, Link2, Plus, RefreshCw, Trash2 } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import {
  createForm,
  deleteForm,
  listForms,
  rotateFormToken,
  updateForm,
  type IntakeForm,
} from '@/lib/api/forms';
import { getBoard } from '@/lib/api/board';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Field } from '@/components/auth/field';
import { useToast } from '@/components/ui/toast';
import { cn } from '@/lib/utils';

// Project intake-form manager (ADR-023). LEAD-only surface. The raw token is
// shown once at create/rotate (API-key-style reveal) because only its hash is
// stored — known tokens are tracked in-session for the copy button.
export function FormsManager({ projectId }: { projectId: string }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const key = ['intake-forms', projectId] as const;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: key });

  const formsQ = useQuery({ queryKey: key, queryFn: () => listForms(orgId, projectId) });
  const forms = formsQ.data ?? [];
  const boardQ = useQuery({ queryKey: ['board', projectId], queryFn: () => getBoard(orgId, projectId) });
  const columns = boardQ.data?.columns ?? [];

  // formId -> raw token revealed this session (create/rotate).
  const [revealed, setRevealed] = useState<Record<string, string>>({});
  const [copied, setCopied] = useState<string | null>(null);

  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [targetColumn, setTargetColumn] = useState('');

  const createMutation = useMutation({
    mutationFn: () =>
      createForm(orgId, projectId, {
        name: name.trim(),
        description: description.trim(),
        target_column_id: targetColumn || (columns[0]?.id ?? ''),
      }),
    onSuccess: ({ form, token }) => {
      invalidate();
      setRevealed((m) => ({ ...m, [form.id]: token }));
      setShowForm(false);
      setName('');
      setDescription('');
      setTargetColumn('');
      toast({ title: 'Form created — copy the link below', variant: 'success' });
    },
    onError: () => toast({ title: 'Could not create form', variant: 'error' }),
  });

  const toggleMutation = useMutation({
    mutationFn: (f: IntakeForm) =>
      updateForm(orgId, projectId, f.id, {
        name: f.name,
        description: f.description,
        target_column_id: f.target_column_id,
        is_active: !f.is_active,
      }),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Could not update form', variant: 'error' }),
  });

  const rotateMutation = useMutation({
    mutationFn: (formId: string) => rotateFormToken(orgId, projectId, formId),
    onSuccess: (token, formId) => {
      setRevealed((m) => ({ ...m, [formId]: token }));
      toast({ title: 'New link issued — the old URL is dead', variant: 'success' });
    },
    onError: () => toast({ title: 'Could not rotate token', variant: 'error' }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteForm(orgId, projectId, id),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Could not delete form', variant: 'error' }),
  });

  function publicUrl(token: string): string {
    return `${window.location.origin}/f/${token}`;
  }

  async function copyLink(formId: string) {
    const token = revealed[formId];
    if (!token) return;
    try {
      await navigator.clipboard.writeText(publicUrl(token));
      setCopied(formId);
      setTimeout(() => setCopied((c) => (c === formId ? null : c)), 2000);
    } catch {
      toast({ title: 'Copy failed — select the link manually', variant: 'error' });
    }
  }

  const canSave = name.trim() !== '' && columns.length > 0;

  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Intake forms
        </h2>
        <Button size="sm" variant="outline" onClick={() => setShowForm((v) => !v)}>
          <Plus className="h-3.5 w-3.5" /> New form
        </Button>
      </div>
      <p className="mb-3 text-xs text-muted-foreground">
        Shareable links that file tasks into this project — no account needed. Submissions land in
        the form&apos;s target column.
      </p>

      {showForm ? (
        <div className="mb-3 space-y-3 rounded-2xl border p-4">
          <Field id="form-name" label="Name">
            <Input
              id="form-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Bug reports"
            />
          </Field>
          <Field id="form-desc" label="Description (shown on the form page)">
            <Input
              id="form-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What should submitters include?"
            />
          </Field>
          <Field id="form-target" label="Target column">
            <select
              id="form-target"
              value={targetColumn || (columns[0]?.id ?? '')}
              onChange={(e) => setTargetColumn(e.target.value)}
              className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm"
            >
              {columns.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </Field>
          <Button
            size="sm"
            onClick={() => canSave && createMutation.mutate()}
            disabled={!canSave || createMutation.isPending}
          >
            {createMutation.isPending ? 'Creating…' : 'Create form'}
          </Button>
        </div>
      ) : null}

      {formsQ.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading forms…</p>
      ) : forms.length === 0 && !showForm ? (
        <p className="rounded-2xl bg-secondary/45 p-4 text-sm text-muted-foreground">
          No intake forms yet. Create one to accept tasks from anyone with the link.
        </p>
      ) : (
        <div className="space-y-2">
          {forms.map((f) => {
            const token = revealed[f.id];
            return (
              <div key={f.id} className="rounded-2xl bg-card p-4 shadow-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{f.name}</p>
                    <p className="text-xs text-muted-foreground">
                      → {columns.find((c) => c.id === f.target_column_id)?.name ?? 'column'} ·{' '}
                      {f.is_active ? 'accepting' : 'paused'}
                    </p>
                  </div>
                  <button
                    onClick={() =>
                      toggleMutation.mutate(f)
                    }
                    disabled={toggleMutation.isPending}
                    className={cn(
                      'relative h-6 w-11 shrink-0 rounded-full transition-colors disabled:opacity-50',
                      f.is_active ? 'bg-primary' : 'bg-secondary',
                    )}
                    role="switch"
                    aria-checked={f.is_active}
                    aria-label={f.is_active ? 'Pause form' : 'Resume form'}
                    title={f.is_active ? 'Pause form' : 'Resume form'}
                  >
                    <span
                      className={cn(
                        'absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all',
                        f.is_active ? 'left-[1.375rem]' : 'left-0.5',
                      )}
                    />
                  </button>
                  <button
                    onClick={() => rotateMutation.mutate(f.id)}
                    disabled={rotateMutation.isPending}
                    className="rounded p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
                    aria-label="Issue new link"
                    title="Issue new link (kills the old URL)"
                  >
                    <RefreshCw className="h-4 w-4" />
                  </button>
                  <button
                    onClick={() => deleteMutation.mutate(f.id)}
                    disabled={deleteMutation.isPending}
                    className="rounded p-1.5 text-destructive hover:bg-destructive/10 disabled:opacity-50"
                    aria-label="Delete form"
                    title="Delete form (submitted tasks stay)"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
                {token ? (
                  <div className="mt-3 flex items-center gap-2 rounded-xl bg-secondary/60 px-3 py-2">
                    <Link2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <code className="min-w-0 flex-1 truncate text-xs">{publicUrl(token)}</code>
                    <button
                      onClick={() => copyLink(f.id)}
                      className="rounded p-1 hover:bg-secondary"
                      aria-label="Copy link"
                      title="Copy link"
                    >
                      {copied === f.id ? (
                        <Check className="h-3.5 w-3.5 text-green-600" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  </div>
                ) : (
                  <p className="mt-2 text-xs text-muted-foreground">
                    Link hidden — rotate to issue a new one (only the hash is stored).
                  </p>
                )}
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
