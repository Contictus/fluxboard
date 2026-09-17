'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Trash2 } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import {
  FIELD_TYPES,
  createCustomField,
  deleteCustomField,
  listCustomFields,
  type CustomFieldType,
} from '@/lib/api/fields';
import { Button } from '@/components/ui/button';
import { Field } from '@/components/auth/field';
import { useToast } from '@/components/ui/toast';

const inputCls =
  'w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

// Project custom-field manager (FR-FIELDS). LEAD-only surface; definitions are
// typed (text/number/date/select) and ordered by position.
export function FieldsManager({ projectId }: { projectId: string }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const key = ['custom-fields', projectId] as const;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: key });

  const fieldsQ = useQuery({ queryKey: key, queryFn: () => listCustomFields(orgId, projectId) });
  const fields = fieldsQ.data ?? [];

  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [type, setType] = useState<CustomFieldType>('text');
  const [options, setOptions] = useState('');

  const createMutation = useMutation({
    mutationFn: () =>
      createCustomField(orgId, projectId, {
        name: name.trim(),
        type,
        options: options
          .split(',')
          .map((o) => o.trim())
          .filter(Boolean),
        position: fields.length,
      }),
    onSuccess: () => {
      invalidate();
      setShowForm(false);
      setName('');
      setOptions('');
    },
    onError: () => toast({ title: 'Could not create field', variant: 'error' }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteCustomField(orgId, projectId, id),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Could not delete field', variant: 'error' }),
  });

  const canSave = name.trim() !== '' && (type !== 'select' || options.split(',').some((o) => o.trim() !== ''));

  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Custom fields
        </h2>
        <Button size="sm" variant="outline" onClick={() => setShowForm((v) => !v)}>
          <Plus className="h-3.5 w-3.5" /> New field
        </Button>
      </div>

      {showForm ? (
        <div className="mb-3 space-y-3 rounded-2xl border p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field id="field-name" label="Name">
              <input
                id="field-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Story points"
                className={inputCls}
              />
            </Field>
            <Field id="field-type" label="Type">
              <select
                id="field-type"
                value={type}
                onChange={(e) => setType(e.target.value as CustomFieldType)}
                className={inputCls}
              >
                {FIELD_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>
                    {t.label}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          {type === 'select' ? (
            <Field id="field-options" label="Options (comma separated)">
              <input
                id="field-options"
                value={options}
                onChange={(e) => setOptions(e.target.value)}
                placeholder="e.g. S, M, L"
                className={inputCls}
              />
            </Field>
          ) : null}
          <div className="flex gap-2">
            <Button size="sm" disabled={!canSave || createMutation.isPending} onClick={() => createMutation.mutate()}>
              {createMutation.isPending ? 'Creating…' : 'Create field'}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setShowForm(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : null}

      {fieldsQ.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading fields…</p>
      ) : fields.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No custom fields yet. Add typed attributes tasks can carry.
        </p>
      ) : (
        <ul className="space-y-1.5">
          {fields.map((f) => (
            <li
              key={f.id}
              className="flex items-center gap-2 rounded-xl border bg-card px-3 py-2 text-sm"
            >
              <span className="font-medium">{f.name}</span>
              <span className="rounded-full bg-secondary px-2 py-0.5 text-[11px] text-muted-foreground">
                {FIELD_TYPES.find((t) => t.value === f.type)?.label ?? f.type}
              </span>
              {f.type === 'select' ? (
                <span className="truncate text-xs text-muted-foreground">{f.options.join(', ')}</span>
              ) : null}
              <button
                type="button"
                onClick={() => deleteMutation.mutate(f.id)}
                aria-label={`Delete ${f.name}`}
                className="ml-auto rounded-md p-1.5 text-muted-foreground hover:bg-secondary hover:text-destructive"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
