'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Trash2 } from 'lucide-react';

import { addSubtask, deleteSubtask, listSubtasks, updateSubtask } from '@/lib/api/tasks';
import { useOrg } from '@/lib/org/context';
import { useToast } from '@/components/ui/toast';
import type { Subtask } from '@/lib/api/types';

// Subtask checklist (FR-TASK-003). Toggle done, add, delete. Editable by
// CONTRIBUTOR+; a 403 surfaces as a toast and the optimistic toggle is undone by
// the invalidate.
export function Subtasks({ taskId, canEdit }: { taskId: string; canEdit: boolean }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const qc = useQueryClient();
  const key = ['subtasks', orgId, taskId] as const;

  const { data: subtasks } = useQuery({ queryKey: key, queryFn: () => listSubtasks(orgId, taskId) });
  const [title, setTitle] = useState('');

  const invalidate = () => qc.invalidateQueries({ queryKey: key });
  const onError = () => toast({ title: 'Couldn’t update subtask', variant: 'error' });

  const add = useMutation({
    mutationFn: (t: string) => addSubtask(orgId, taskId, t),
    onSuccess: () => {
      setTitle('');
      invalidate();
    },
    onError,
  });
  const toggle = useMutation({
    mutationFn: (st: Subtask) => updateSubtask(orgId, taskId, st.id, { title: st.title, done: !st.done }),
    onSuccess: invalidate,
    onError,
  });
  const remove = useMutation({
    mutationFn: (id: string) => deleteSubtask(orgId, taskId, id),
    onSuccess: invalidate,
    onError,
  });

  const list = subtasks ?? [];
  const done = list.filter((s) => s.done).length;

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Subtasks</h3>
        {list.length > 0 ? (
          <span className="text-xs text-muted-foreground">
            {done}/{list.length}
          </span>
        ) : null}
      </div>

      <ul className="space-y-1">
        {list.map((st) => (
          <li key={st.id} className="group flex items-center gap-2 rounded px-1 py-0.5 hover:bg-secondary/50">
            <input
              type="checkbox"
              checked={st.done}
              disabled={!canEdit || toggle.isPending}
              onChange={() => toggle.mutate(st)}
              className="h-4 w-4"
              aria-label={st.title}
            />
            <span className={st.done ? 'flex-1 text-sm text-muted-foreground line-through' : 'flex-1 text-sm'}>
              {st.title}
            </span>
            {canEdit ? (
              <button
                type="button"
                onClick={() => remove.mutate(st.id)}
                className="opacity-0 transition-opacity group-hover:opacity-100"
                aria-label="Delete subtask"
              >
                <Trash2 className="h-3.5 w-3.5 text-muted-foreground hover:text-destructive" />
              </button>
            ) : null}
          </li>
        ))}
        {list.length === 0 ? <li className="text-sm text-muted-foreground">No subtasks yet.</li> : null}
      </ul>

      {canEdit ? (
        <form
          className="mt-2 flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (title.trim()) add.mutate(title.trim());
          }}
        >
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Add a subtask…"
            className="flex-1 rounded-md border border-input bg-background px-2 py-1 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <button
            type="submit"
            disabled={!title.trim() || add.isPending}
            className="inline-flex items-center gap-1 rounded-md bg-secondary px-2 py-1 text-sm hover:bg-secondary/80 disabled:opacity-50"
          >
            <Plus className="h-3.5 w-3.5" /> Add
          </button>
        </form>
      ) : null}
    </section>
  );
}
