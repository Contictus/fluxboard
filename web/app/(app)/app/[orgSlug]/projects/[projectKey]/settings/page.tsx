'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Field } from '@/components/auth/field';
import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { archiveProject, unarchiveProject, updateProject } from '@/lib/api/projects';
import {
  createColumn,
  deleteColumn,
  getBoard,
  renameColumn,
  reorderColumn,
} from '@/lib/api/board';
import { between } from '@/lib/board/rank';
import { useToast } from '@/components/ui/toast';
import { ProjectHeader } from '@/components/board/project-header';
import type { BoardColumn, Project, Visibility } from '@/lib/api/types';
import { cn } from '@/lib/utils';

export default function ProjectSettingsPage({ params }: { params: { projectKey: string } }) {
  const { slug, isAdmin } = useOrg();
  const { project, isLoading, isError } = useProjectByKey(params.projectKey);

  if (isLoading) return <p className="px-6 py-8 text-sm text-muted-foreground">Loading project…</p>;
  if (isError || !project)
    return (
      <div className="mx-auto max-w-md px-6 py-16 text-center">
        <h1 className="text-lg font-semibold">Project not found</h1>
        <Link href={`/app/${slug}/projects`} className="mt-4 inline-block text-sm text-primary hover:underline">
          Back to projects
        </Link>
      </div>
    );

  return (
    <div className="mx-auto max-w-6xl px-6 py-10 lg:px-10 lg:py-14">
      <ProjectHeader project={project} />
      {isAdmin ? (
        <div className="max-w-2xl space-y-8">
          <GeneralSection project={project} />
          <ColumnsSection project={project} />
          <DangerSection project={project} />
        </div>
      ) : (
        <p className="max-w-2xl rounded-2xl bg-secondary/55 p-5 text-sm text-muted-foreground">
          Only project leads and organization admins can change project settings.
        </p>
      )}
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">{children}</h2>;
}

function GeneralSection({ project }: { project: Project }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [name, setName] = useState(project.name);
  const [description, setDescription] = useState(project.description);
  const [color, setColor] = useState(project.color);
  const [visibility, setVisibility] = useState<Visibility>(project.visibility);
  const archived = Boolean(project.archived_at);

  const mutation = useMutation({
    mutationFn: () => updateProject(orgId, project.id, { name, description, color, visibility }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['project-by-key', orgId, project.key] });
      queryClient.invalidateQueries({ queryKey: ['projects', orgId] });
      toast({ title: 'Project updated', variant: 'success' });
    },
    onError: () => toast({ title: 'Update failed', variant: 'error' }),
  });

  return (
    <section>
      <SectionTitle>General</SectionTitle>
      {archived ? (
        <p className="mb-3 rounded-2xl bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-400">
          This project is archived and read-only. Unarchive it below to edit.
        </p>
      ) : null}
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
      >
        <Field id="p-name" label="Name">
          <Input id="p-name" value={name} onChange={(e) => setName(e.target.value)} disabled={archived} required />
        </Field>
        <Field id="p-desc" label="Description">
          <textarea
            id="p-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            disabled={archived}
            rows={3}
            className="flex w-full rounded-xl border-0 bg-secondary/60 px-4 py-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          />
        </Field>
        <Field id="p-color" label="Color">
          <input
            id="p-color"
            type="color"
            value={color || '#6366f1'}
            onChange={(e) => setColor(e.target.value)}
            disabled={archived}
            className="h-9 w-16 rounded border border-input bg-background"
          />
        </Field>
        <div className="space-y-1.5">
          <span className="text-sm font-medium">Visibility</span>
          <div className="grid gap-2 sm:grid-cols-2">
            {(
              [
                { v: 'org', label: 'Organization', hint: 'All org members.' },
                { v: 'private', label: 'Private', hint: 'Only project members.' },
              ] as const
            ).map(({ v, label, hint }) => (
              <button
                key={v}
                type="button"
                disabled={archived}
                onClick={() => setVisibility(v)}
                className={cn(
                  'rounded-2xl bg-secondary/45 p-4 text-left text-sm transition-colors disabled:opacity-50',
                  visibility === v ? 'bg-primary/10 text-foreground ring-1 ring-primary/30' : 'hover:bg-secondary',
                )}
              >
                <span className="font-medium">{label}</span>
                <span className="mt-0.5 block text-xs text-muted-foreground">{hint}</span>
              </button>
            ))}
          </div>
        </div>
        <Button type="submit" disabled={archived || mutation.isPending}>
          {mutation.isPending ? 'Saving…' : 'Save changes'}
        </Button>
      </form>
    </section>
  );
}

function ColumnsSection({ project }: { project: Project }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const boardKey = ['board', project.id] as const;

  const board = useQuery({ queryKey: boardKey, queryFn: () => getBoard(orgId, project.id) });
  const columns = board.data?.columns ?? [];

  const [newName, setNewName] = useState('');

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: boardKey });
  }

  const add = useMutation({
    mutationFn: () => createColumn(orgId, project.id, { name: newName.trim() }),
    onSuccess: () => {
      setNewName('');
      invalidate();
    },
    onError: () => toast({ title: 'Couldn’t add column', variant: 'error' }),
  });

  const rename = useMutation({
    mutationFn: (v: { id: string; name: string; wip_limit: number | null }) =>
      renameColumn(orgId, project.id, v.id, { name: v.name, wip_limit: v.wip_limit }),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Couldn’t update column', variant: 'error' }),
  });

  const reorder = useMutation({
    mutationFn: (v: { id: string; rank: string }) => reorderColumn(orgId, project.id, v.id, v.rank),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Couldn’t reorder column', variant: 'error' }),
  });

  const remove = useMutation({
    mutationFn: (v: { id: string; target: string }) => deleteColumn(orgId, project.id, v.id, v.target),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Couldn’t delete column', variant: 'error' }),
  });

  function move(i: number, dir: -1 | 1) {
    const j = i + dir;
    if (j < 0 || j >= columns.length) return;
    const col = columns[i] as BoardColumn;
    // Compute a rank that lands the column on the other side of its neighbour.
    let rank: string;
    try {
      if (dir === -1) {
        const before = columns[j - 1]?.rank ?? '';
        rank = between(before, (columns[j] as BoardColumn).rank);
      } else {
        const after = columns[j + 1]?.rank ?? '';
        rank = between((columns[j] as BoardColumn).rank, after);
      }
    } catch {
      return;
    }
    reorder.mutate({ id: col.id, rank });
  }

  return (
    <section>
      <SectionTitle>Columns</SectionTitle>
      {board.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading columns…</p>
      ) : (
        <div className="space-y-2">
          {columns.map((c, i) => (
            <ColumnRow
              key={c.id}
              column={c}
              index={i}
              total={columns.length}
              others={columns.filter((o) => o.id !== c.id)}
              onRename={(name, wip) => rename.mutate({ id: c.id, name, wip_limit: wip })}
              onMove={(dir) => move(i, dir)}
              onDelete={(target) => remove.mutate({ id: c.id, target })}
              busy={rename.isPending || reorder.isPending || remove.isPending}
            />
          ))}

          <div className="flex items-center gap-2 pt-2">
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="New column name"
              className="max-w-xs"
            />
            <Button
              variant="outline"
              onClick={() => newName.trim() && add.mutate()}
              disabled={add.isPending || !newName.trim()}
            >
              Add column
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}

function ColumnRow({
  column,
  index,
  total,
  others,
  onRename,
  onMove,
  onDelete,
  busy,
}: {
  column: BoardColumn;
  index: number;
  total: number;
  others: BoardColumn[];
  onRename: (name: string, wip: number | null) => void;
  onMove: (dir: -1 | 1) => void;
  onDelete: (target: string) => void;
  busy: boolean;
}) {
  const [name, setName] = useState(column.name);
  const [wip, setWip] = useState<string>(column.wip_limit != null ? String(column.wip_limit) : '');
  const [confirming, setConfirming] = useState(false);
  const [target, setTarget] = useState(others[0]?.id ?? '');

  useEffect(() => {
    setName(column.name);
    setWip(column.wip_limit != null ? String(column.wip_limit) : '');
  }, [column.name, column.wip_limit]);

  function commit() {
    const parsedWip = wip.trim() === '' ? null : Number.parseInt(wip, 10);
    const wipVal = parsedWip != null && Number.isFinite(parsedWip) && parsedWip > 0 ? parsedWip : null;
    if (name.trim() && (name.trim() !== column.name || wipVal !== (column.wip_limit ?? null))) {
      onRename(name.trim(), wipVal);
    }
  }

  return (
    <div className="rounded-2xl bg-card p-3 shadow-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Input value={name} onChange={(e) => setName(e.target.value)} onBlur={commit} className="max-w-[16rem]" />
        <label className="flex items-center gap-1 text-xs text-muted-foreground">
          WIP
          <Input
            type="number"
            min={0}
            value={wip}
            onChange={(e) => setWip(e.target.value)}
            onBlur={commit}
            placeholder="—"
            className="h-8 w-16"
          />
        </label>
        <div className="ml-auto flex items-center gap-1">
          <button
            onClick={() => onMove(-1)}
            disabled={index === 0 || busy}
            className="rounded p-1.5 hover:bg-secondary disabled:opacity-30"
            aria-label="Move column up"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
          <button
            onClick={() => onMove(1)}
            disabled={index === total - 1 || busy}
            className="rounded p-1.5 hover:bg-secondary disabled:opacity-30"
            aria-label="Move column down"
          >
            <ArrowDown className="h-4 w-4" />
          </button>
          <button
            onClick={() => setConfirming((v) => !v)}
            disabled={others.length === 0 || busy}
            className="rounded p-1.5 text-destructive hover:bg-destructive/10 disabled:opacity-30"
            aria-label="Delete column"
            title={others.length === 0 ? 'Cannot delete the only column' : 'Delete column'}
          >
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      </div>

      {confirming ? (
        <div className="mt-2 flex flex-wrap items-center gap-2 rounded-2xl bg-destructive/5 p-3 text-sm">
          <span>Move its tasks to</span>
          <select
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            className="h-8 rounded border border-input bg-background px-2"
          >
            {others.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
          <button
            onClick={() => {
              onDelete(target);
              setConfirming(false);
            }}
            disabled={!target}
            className="rounded bg-destructive px-2 py-1 text-xs font-medium text-destructive-foreground disabled:opacity-50"
          >
            Delete column
          </button>
          <button onClick={() => setConfirming(false)} className="text-xs text-muted-foreground hover:text-foreground">
            Cancel
          </button>
        </div>
      ) : null}
    </div>
  );
}

function DangerSection({ project }: { project: Project }) {
  const { orgId, slug } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const archived = Boolean(project.archived_at);

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ['project-by-key', orgId, project.key] });
    queryClient.invalidateQueries({ queryKey: ['projects', orgId] });
  }

  const archive = useMutation({
    mutationFn: () => archiveProject(orgId, project.id),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Project archived', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t archive', variant: 'error' }),
  });

  const unarchive = useMutation({
    mutationFn: () => unarchiveProject(orgId, project.id),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Project restored', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t restore', variant: 'error' }),
  });

  return (
    <section>
      <SectionTitle>Danger zone</SectionTitle>
      <div className="flex items-center justify-between rounded-2xl bg-destructive/5 p-5">
        <div>
          <p className="text-sm font-medium">{archived ? 'Restore project' : 'Archive project'}</p>
          <p className="text-sm text-muted-foreground">
            {archived
              ? 'Make this project active and editable again.'
              : 'Hide it from the active list and make it read-only. You can restore it later.'}
          </p>
        </div>
        {archived ? (
          <Button variant="outline" onClick={() => unarchive.mutate()} disabled={unarchive.isPending}>
            Restore
          </Button>
        ) : (
          <Button variant="destructive" onClick={() => archive.mutate()} disabled={archive.isPending}>
            Archive
          </Button>
        )}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        <Link href={`/app/${slug}/projects`} className="hover:underline">
          Back to projects
        </Link>
      </p>
    </section>
  );
}
