'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { useOrg } from '@/lib/org/context';
import {
  clearTaskFieldValue,
  listCustomFields,
  listTaskFieldValues,
  setTaskFieldValue,
  type CustomField,
  type CustomValue,
} from '@/lib/api/fields';
import { useToast } from '@/components/ui/toast';

const inputCls =
  'w-full rounded-lg border border-input bg-background px-2 py-1.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

// Per-task custom values (FR-FIELDS). Renders every project field with its
// current value editor; empty values show the type-appropriate control.
export function TaskCustomValues({
  projectId,
  taskId,
  canEdit,
}: {
  projectId: string;
  taskId: string;
  canEdit: boolean;
}) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const key = ['task-fields', taskId] as const;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: key });

  const fieldsQ = useQuery({
    queryKey: ['custom-fields', projectId],
    queryFn: () => listCustomFields(orgId, projectId),
  });
  const valuesQ = useQuery({ queryKey: key, queryFn: () => listTaskFieldValues(orgId, taskId) });
  const fields = fieldsQ.data ?? [];
  const byField = new Map((valuesQ.data ?? []).map((v) => [v.field_id, v]));

  const setMutation = useMutation({
    mutationFn: (v: { fieldId: string; value: { text?: string | null; number?: number | null; date?: string | null } }) =>
      setTaskFieldValue(orgId, taskId, v.fieldId, v.value),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Could not save value', variant: 'error' }),
  });
  const clearMutation = useMutation({
    mutationFn: (fieldId: string) => clearTaskFieldValue(orgId, taskId, fieldId),
    onSuccess: invalidate,
  });

  if (fields.length === 0) return null;

  return (
    <section>
      <h3 className="mb-2 text-sm font-semibold">Custom fields</h3>
      <div className="space-y-2">
        {fields.map((f) => (
          <FieldRow
            key={f.id}
            field={f}
            value={byField.get(f.id)}
            canEdit={canEdit}
            onSet={(value) => setMutation.mutate({ fieldId: f.id, value })}
            onClear={() => clearMutation.mutate(f.id)}
          />
        ))}
      </div>
    </section>
  );
}

function FieldRow({
  field,
  value,
  canEdit,
  onSet,
  onClear,
}: {
  field: CustomField;
  value?: CustomValue;
  canEdit: boolean;
  onSet: (value: { text?: string | null; number?: number | null; date?: string | null }) => void;
  onClear: () => void;
}) {
  const control = (() => {
    switch (field.type) {
      case 'number':
        return (
          <input
            type="number"
            step="any"
            defaultValue={value?.number ?? ''}
            key={value?.number ?? 'empty'}
            disabled={!canEdit}
            onBlur={(e) => {
              if (e.target.value === '') {
                if (value) onClear();
                return;
              }
              onSet({ number: Number(e.target.value) });
            }}
            className={inputCls}
            aria-label={field.name}
          />
        );
      case 'date':
        return (
          <input
            type="date"
            defaultValue={value?.date ? value.date.slice(0, 10) : ''}
            key={value?.date ?? 'empty'}
            disabled={!canEdit}
            onChange={(e) => {
              if (e.target.value === '') {
                if (value) onClear();
                return;
              }
              onSet({ date: e.target.value });
            }}
            className={inputCls}
            aria-label={field.name}
          />
        );
      case 'select':
        return (
          <select
            defaultValue={value?.text ?? ''}
            key={value?.text ?? 'empty'}
            disabled={!canEdit}
            onChange={(e) => {
              if (e.target.value === '') {
                if (value) onClear();
                return;
              }
              onSet({ text: e.target.value });
            }}
            className={inputCls}
            aria-label={field.name}
          >
            <option value="">—</option>
            {field.options.map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </select>
        );
      default:
        return (
          <input
            type="text"
            defaultValue={value?.text ?? ''}
            key={value?.text ?? 'empty'}
            disabled={!canEdit}
            onBlur={(e) => {
              if (e.target.value.trim() === '') {
                if (value) onClear();
                return;
              }
              onSet({ text: e.target.value });
            }}
            className={inputCls}
            aria-label={field.name}
          />
        );
    }
  })();

  return (
    <label className="block">
      <span className="mb-1 block text-xs text-muted-foreground">{field.name}</span>
      {control}
    </label>
  );
}
