'use client';

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useOrg } from '@/lib/org/context';
import { useToast } from '@/components/ui/toast';
import { createLabel, deleteLabel, listLabels, updateLabel } from '@/lib/api/labels';
import type { Label } from '@/lib/api/types';

const DEFAULT_COLOR = '#6366f1';

export default function LabelsSettingsPage() {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const labelsKey = ['labels', orgId] as const;

  const labels = useQuery({ queryKey: labelsKey, queryFn: () => listLabels(orgId) });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: labelsKey });

  const [newName, setNewName] = useState('');
  const [newColor, setNewColor] = useState(DEFAULT_COLOR);

  const add = useMutation({
    mutationFn: () => createLabel(orgId, { name: newName.trim(), color: newColor }),
    onSuccess: () => {
      setNewName('');
      setNewColor(DEFAULT_COLOR);
      invalidate();
    },
    onError: () => toast({ title: 'Couldn’t create label', variant: 'error' }),
  });

  const rename = useMutation({
    mutationFn: (v: Label) => updateLabel(orgId, v.id, { name: v.name, color: v.color }),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Couldn’t update label', variant: 'error' }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteLabel(orgId, id),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Couldn’t delete label', variant: 'error' }),
  });

  const items = labels.data ?? [];

  return (
    <div className="max-w-2xl space-y-6">
      <div>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Labels</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Labels are shared across all projects and used to tag and filter tasks.
        </p>
      </div>

      {labels.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading labels…</p>
      ) : (
        <div className="space-y-2">
          {items.map((l) => (
            <LabelRow
              key={l.id}
              label={l}
              onSave={(name, color) => rename.mutate({ ...l, name, color })}
              onDelete={() => remove.mutate(l.id)}
              busy={rename.isPending || remove.isPending}
            />
          ))}
          {items.length === 0 ? <p className="text-sm text-muted-foreground">No labels yet.</p> : null}
        </div>
      )}

      <form
        className="flex items-center gap-2 border-t pt-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (newName.trim()) add.mutate();
        }}
      >
        <input
          type="color"
          value={newColor}
          onChange={(e) => setNewColor(e.target.value)}
          className="h-9 w-12 rounded border border-input bg-background"
          aria-label="New label color"
        />
        <Input
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          placeholder="New label name"
          className="max-w-xs"
        />
        <Button type="submit" variant="outline" disabled={add.isPending || !newName.trim()}>
          Add label
        </Button>
      </form>
    </div>
  );
}

function LabelRow({
  label,
  onSave,
  onDelete,
  busy,
}: {
  label: Label;
  onSave: (name: string, color: string) => void;
  onDelete: () => void;
  busy: boolean;
}) {
  const [name, setName] = useState(label.name);
  const [color, setColor] = useState(label.color || DEFAULT_COLOR);
  const [confirming, setConfirming] = useState(false);

  useEffect(() => {
    setName(label.name);
    setColor(label.color || DEFAULT_COLOR);
  }, [label.name, label.color]);

  function commit() {
    const n = name.trim();
    if (n && (n !== label.name || color !== label.color)) onSave(n, color);
  }

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border bg-card p-2">
      <input
        type="color"
        value={color}
        onChange={(e) => setColor(e.target.value)}
        onBlur={commit}
        className="h-8 w-10 rounded border border-input bg-background"
        aria-label="Label color"
      />
      <Input value={name} onChange={(e) => setName(e.target.value)} onBlur={commit} className="max-w-xs" />
      <span
        className="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium text-white"
        style={{ backgroundColor: color }}
      >
        {name || 'label'}
      </span>
      <div className="ml-auto">
        {confirming ? (
          <span className="flex items-center gap-1 text-xs">
            <button
              onClick={() => {
                onDelete();
                setConfirming(false);
              }}
              className="rounded bg-destructive px-2 py-1 font-medium text-destructive-foreground"
            >
              Delete
            </button>
            <button onClick={() => setConfirming(false)} className="px-1 text-muted-foreground hover:text-foreground">
              Cancel
            </button>
          </span>
        ) : (
          <button
            onClick={() => setConfirming(true)}
            disabled={busy}
            className="rounded p-1.5 text-destructive hover:bg-destructive/10 disabled:opacity-30"
            aria-label="Delete label"
          >
            <Trash2 className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  );
}
