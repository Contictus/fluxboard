'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core';
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { Plus } from 'lucide-react';

import type { Board as BoardData, BulkActionInput, TaskCard } from '@/lib/api/types';
import { getBoard, bulkTasks, createColumn, createTask, moveTask } from '@/lib/api/board';
import { listLabels } from '@/lib/api/labels';
import { useOrg } from '@/lib/org/context';
import { useAuth } from '@/lib/auth/context';
import { ApiError } from '@/lib/api/client';
import { useToast } from '@/components/ui/toast';
import { between } from '@/lib/board/rank';
import { priorityClass, priorityLabel } from '@/lib/board/priority';
import { cn } from '@/lib/utils';
import { Column } from './column';
import { BoardToolbar, EMPTY_FILTER, type BoardFilter } from './board-toolbar';

interface MoveVars {
  taskId: string;
  columnId: string;
  rank: string;
  snapshot: BoardData;
}

// Move `taskId` to (destColId, newRank) in the cached board projection, keeping
// each column sorted by rank. Pure — returns a new board.
function applyMove(board: BoardData, taskId: string, destColId: string, newRank: string): BoardData {
  let moving: TaskCard | undefined;
  const stripped = board.columns.map((col) => {
    const idx = col.tasks.findIndex((t) => t.id === taskId);
    if (idx < 0) return col;
    moving = { ...(col.tasks[idx] as TaskCard) };
    return { ...col, tasks: col.tasks.filter((t) => t.id !== taskId) };
  });
  if (!moving) return board;
  const placed = { ...moving, rank: newRank };
  return {
    ...board,
    columns: stripped.map((col) => {
      if (col.id !== destColId) return col;
      const tasks = [...col.tasks, placed].sort((a, b) => (a.rank < b.rank ? -1 : a.rank > b.rank ? 1 : 0));
      return { ...col, tasks };
    }),
  };
}

export function Board({ projectId, projectKey }: { projectId: string; projectKey: string }) {
  const { orgId, slug, isAdmin } = useOrg();
  const { userId } = useAuth();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const boardKey = ['board', projectId] as const;

  const { data: board, isLoading, isError } = useQuery({ queryKey: boardKey, queryFn: () => getBoard(orgId, projectId) });
  const { data: labels } = useQuery({ queryKey: ['labels', orgId], queryFn: () => listLabels(orgId) });

  const [filter, setFilter] = useState<BoardFilter>(EMPTY_FILTER);
  const [selectMode, setSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [activeId, setActiveId] = useState<string | null>(null);
  const [addingColumn, setAddingColumn] = useState(false);
  const [newColumnName, setNewColumnName] = useState('');

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const moveMutation = useMutation({
    mutationFn: (v: MoveVars) => moveTask(orgId, v.taskId, { column_id: v.columnId, rank: v.rank }),
    onError: (err, v) => {
      queryClient.setQueryData(boardKey, v.snapshot);
      const conflict = err instanceof ApiError && err.status === 409;
      toast({
        title: conflict ? 'Card moved elsewhere' : 'Move failed',
        description: conflict
          ? 'Someone changed this board — it has been refreshed.'
          : 'Please try again.',
        variant: 'error',
      });
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: boardKey }),
  });

  const createTaskMutation = useMutation({
    mutationFn: (v: { columnId: string; title: string }) =>
      createTask(orgId, projectId, { column_id: v.columnId, title: v.title, description: '', priority: 'none' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKey }),
    onError: (err) =>
      toast({
        title: 'Couldn’t add task',
        description: err instanceof ApiError && err.status === 403 ? 'You don’t have access to add tasks here.' : 'Please try again.',
        variant: 'error',
      }),
  });

  const bulkMutation = useMutation({
    mutationFn: (input: BulkActionInput) => bulkTasks(orgId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKey });
      setSelectedIds(new Set());
    },
    onError: () => toast({ title: 'Bulk action failed', description: 'Please try again.', variant: 'error' }),
  });

  const addColumnMutation = useMutation({
    mutationFn: (name: string) => createColumn(orgId, projectId, { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKey });
      setNewColumnName('');
      setAddingColumn(false);
    },
    onError: () => toast({ title: 'Couldn’t add column', variant: 'error' }),
  });

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading board…</p>;
  if (isError || !board) return <p className="text-sm text-destructive">Couldn’t load the board.</p>;

  function visible(tasks: TaskCard[]): TaskCard[] {
    const q = filter.text.trim().toLowerCase();
    return tasks.filter((t) => {
      if (q && !t.title.toLowerCase().includes(q)) return false;
      if (filter.priority && t.priority !== filter.priority) return false;
      if (filter.assignee === 'me' && t.assignee_id !== userId) return false;
      if (filter.assignee === 'unassigned' && t.assignee_id) return false;
      return true;
    });
  }

  function toggleSelect(id: string) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function onDragStart(e: DragStartEvent) {
    setActiveId(String(e.active.id));
  }

  function onDragEnd(e: DragEndEvent) {
    setActiveId(null);
    const { active, over } = e;
    if (!over) return;
    const activeTaskId = String(active.id);
    const overId = String(over.id);

    const current = queryClient.getQueryData<BoardData>(boardKey);
    if (!current) return;

    const activeTask = current.columns.flatMap((c) => c.tasks).find((t) => t.id === activeTaskId);
    if (!activeTask) return;

    // Destination column: `over` is either a column id or a card id.
    let destCol = current.columns.find((c) => c.id === overId);
    if (!destCol) destCol = current.columns.find((c) => c.tasks.some((t) => t.id === overId));
    if (!destCol) return;

    const destTasks = destCol.tasks.filter((t) => t.id !== activeTaskId);
    let insertIndex: number;
    if (overId === destCol.id) insertIndex = destTasks.length;
    else {
      const oi = destTasks.findIndex((t) => t.id === overId);
      insertIndex = oi < 0 ? destTasks.length : oi;
    }
    const prev = destTasks[insertIndex - 1]?.rank ?? '';
    const next = destTasks[insertIndex]?.rank ?? '';

    let newRank: string;
    try {
      newRank = between(prev, next);
    } catch {
      // Neighbours out of order (shouldn't happen with sorted ranks) — bail.
      return;
    }

    queryClient.setQueryData(boardKey, applyMove(current, activeTaskId, destCol.id, newRank));
    moveMutation.mutate({ taskId: activeTaskId, columnId: destCol.id, rank: newRank, snapshot: current });
  }

  const cardHref = (t: TaskCard) => `/app/${slug}/projects/${projectKey}/tasks/${t.number}`;
  const activeTask = activeId
    ? board.columns.flatMap((c) => c.tasks).find((t) => t.id === activeId)
    : undefined;

  return (
    <div>
      <BoardToolbar
        filter={filter}
        onFilter={setFilter}
        selectMode={selectMode}
        onToggleSelectMode={() => {
          setSelectMode((m) => !m);
          setSelectedIds(new Set());
        }}
        selectedCount={selectedIds.size}
        columns={board.columns}
        labels={labels ?? []}
        bulkPending={bulkMutation.isPending}
        onBulkMove={(columnId) => bulkMutation.mutate({ action: 'move', task_ids: [...selectedIds], column_id: columnId })}
        onBulkAssignMe={() =>
          bulkMutation.mutate({ action: 'assign', task_ids: [...selectedIds], assignee_id: userId })
        }
        onBulkUnassign={() =>
          bulkMutation.mutate({ action: 'assign', task_ids: [...selectedIds], assignee_id: null })
        }
        onBulkLabel={(labelId) => bulkMutation.mutate({ action: 'label', task_ids: [...selectedIds], label_id: labelId })}
        onClearSelection={() => setSelectedIds(new Set())}
      />

      <DndContext
        sensors={sensors}
        collisionDetection={closestCorners}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
      >
        <div className="scrollbar-thin flex gap-3 overflow-x-auto pb-4">
          {board.columns.map((col) => (
            <Column
              key={col.id}
              column={col}
              tasks={visible(col.tasks)}
              cardHref={cardHref}
              selectMode={selectMode}
              selectedIds={selectedIds}
              onToggleSelect={toggleSelect}
              onAddTask={(columnId, title) => createTaskMutation.mutate({ columnId, title })}
              addPending={createTaskMutation.isPending}
            />
          ))}

          {isAdmin ? (
            <div className="w-72 shrink-0">
              {addingColumn ? (
                <div className="rounded-lg border bg-card p-2">
                  <input
                    autoFocus
                    value={newColumnName}
                    onChange={(e) => setNewColumnName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && newColumnName.trim()) addColumnMutation.mutate(newColumnName.trim());
                      if (e.key === 'Escape') setAddingColumn(false);
                    }}
                    placeholder="Column name…"
                    className="w-full rounded border border-input bg-background px-2 py-1 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                  <div className="mt-1 flex gap-2">
                    <button
                      onClick={() => newColumnName.trim() && addColumnMutation.mutate(newColumnName.trim())}
                      disabled={addColumnMutation.isPending || !newColumnName.trim()}
                      className="rounded bg-primary px-2 py-1 text-xs font-medium text-primary-foreground disabled:opacity-50"
                    >
                      Add
                    </button>
                    <button
                      onClick={() => setAddingColumn(false)}
                      className="text-xs text-muted-foreground hover:text-foreground"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <button
                  onClick={() => setAddingColumn(true)}
                  className="flex w-full items-center gap-1.5 rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground hover:bg-secondary hover:text-foreground"
                >
                  <Plus className="h-4 w-4" /> Add column
                </button>
              )}
            </div>
          ) : null}
        </div>

        <DragOverlay>
          {activeTask ? (
            <div className="w-64 rounded-md border bg-card p-2.5 text-sm shadow-lg">
              <p className="truncate font-medium">{activeTask.title}</p>
              <div className="mt-1.5 flex items-center gap-2">
                <span className="font-mono text-xs text-muted-foreground">#{activeTask.number}</span>
                {activeTask.priority !== 'none' ? (
                  <span className={cn('rounded px-1.5 py-0.5 text-[10px] font-medium', priorityClass(activeTask.priority))}>
                    {priorityLabel(activeTask.priority)}
                  </span>
                ) : null}
              </div>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
    </div>
  );
}
