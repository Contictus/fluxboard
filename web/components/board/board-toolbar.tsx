'use client';

import { useState } from 'react';
import { Filter, CheckSquare, X } from 'lucide-react';

import type { BoardColumn, Label, Priority } from '@/lib/api/types';
import type { Sprint } from '@/lib/api/sprints';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import { cn } from '@/lib/utils';

export interface BoardFilter {
  text: string;
  priority: Priority | '';
  assignee: 'any' | 'me' | 'unassigned';
  sprint: '' | 'backlog' | string;
}

export const EMPTY_FILTER: BoardFilter = { text: '', priority: '', assignee: 'any', sprint: '' };

const selectCls =
  'h-10 rounded-xl border-0 bg-secondary/60 px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

// Board toolbar: client-side filters (text / priority / assignee — the board
// projection carries no labels, so label is not a filter, ADR-017) plus the
// multi-select mode toggle and, when a selection exists, the bulk action bar.
export function BoardToolbar({
  filter,
  onFilter,
  selectMode,
  onToggleSelectMode,
  selectedCount,
  columns,
  labels,
  sprints,
  bulkPending,
  onBulkMove,
  onBulkAssignMe,
  onBulkUnassign,
  onBulkLabel,
  onClearSelection,
}: {
  filter: BoardFilter;
  onFilter: (f: BoardFilter) => void;
  selectMode: boolean;
  onToggleSelectMode: () => void;
  selectedCount: number;
  columns: BoardColumn[];
  labels: Label[];
  sprints: Sprint[];
  bulkPending: boolean;
  onBulkMove: (columnId: string) => void;
  onBulkAssignMe: () => void;
  onBulkUnassign: () => void;
  onBulkLabel: (labelId: string) => void;
  onClearSelection: () => void;
}) {
  const [moveOpen, setMoveOpen] = useState(false);
  const [labelOpen, setLabelOpen] = useState(false);

  return (
    <div className="mb-6 flex flex-wrap items-center gap-2 py-2">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Filter className="h-4 w-4" />
      </div>
      <input
        id="board-filter"
        value={filter.text}
        onChange={(e) => onFilter({ ...filter, text: e.target.value })}
        placeholder="Filter tasks…  ( / )"
        className="h-10 w-56 rounded-xl border-0 bg-secondary/60 px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <select
        value={filter.priority}
        onChange={(e) => onFilter({ ...filter, priority: e.target.value as Priority | '' })}
        className={selectCls}
        aria-label="Filter by priority"
      >
        <option value="">Any priority</option>
        {PRIORITIES.map((p) => (
          <option key={p} value={p}>
            {priorityLabel(p)}
          </option>
        ))}
      </select>
      <select
        value={filter.assignee}
        onChange={(e) => onFilter({ ...filter, assignee: e.target.value as BoardFilter['assignee'] })}
        className={selectCls}
        aria-label="Filter by assignee"
      >
        <option value="any">Anyone</option>
        <option value="me">Assigned to me</option>
        <option value="unassigned">Unassigned</option>
      </select>
      <select
        value={filter.sprint}
        onChange={(e) => onFilter({ ...filter, sprint: e.target.value })}
        className={selectCls}
        aria-label="Filter by sprint"
      >
        <option value="">All sprints</option>
        <option value="backlog">Backlog</option>
        {sprints.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
          </option>
        ))}
      </select>

      <button
        onClick={onToggleSelectMode}
        className={cn(
          'ml-auto inline-flex h-10 items-center gap-2 rounded-xl px-3 text-sm transition-colors',
          selectMode ? 'bg-primary/10 text-primary' : 'bg-secondary/60 hover:bg-secondary',
        )}
      >
        <CheckSquare className="h-4 w-4" /> {selectMode ? 'Selecting' : 'Select'}
      </button>

      {selectMode && selectedCount > 0 ? (
        <div className="flex w-full items-center gap-2 rounded-2xl bg-card px-4 py-3 shadow-sm">
          <span className="text-sm font-medium">{selectedCount} selected</span>

          <div className="relative">
            <button
              onClick={() => setMoveOpen((o) => !o)}
              disabled={bulkPending}
              className="h-8 rounded-md border px-2 text-sm hover:bg-secondary disabled:opacity-50"
            >
              Move to…
            </button>
            {moveOpen ? (
              <div className="absolute z-10 mt-1 w-44 rounded-md border bg-card p-1 shadow-lg">
                {columns.map((c) => (
                  <button
                    key={c.id}
                    onClick={() => {
                      setMoveOpen(false);
                      onBulkMove(c.id);
                    }}
                    className="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-secondary"
                  >
                    {c.name}
                  </button>
                ))}
              </div>
            ) : null}
          </div>

          <button
            onClick={onBulkAssignMe}
            disabled={bulkPending}
            className="h-8 rounded-md border px-2 text-sm hover:bg-secondary disabled:opacity-50"
          >
            Assign to me
          </button>
          <button
            onClick={onBulkUnassign}
            disabled={bulkPending}
            className="h-8 rounded-md border px-2 text-sm hover:bg-secondary disabled:opacity-50"
          >
            Unassign
          </button>

          {labels.length > 0 ? (
            <div className="relative">
              <button
                onClick={() => setLabelOpen((o) => !o)}
                disabled={bulkPending}
                className="h-8 rounded-md border px-2 text-sm hover:bg-secondary disabled:opacity-50"
              >
                Add label…
              </button>
              {labelOpen ? (
                <div className="absolute z-10 mt-1 w-44 rounded-md border bg-card p-1 shadow-lg">
                  {labels.map((l) => (
                    <button
                      key={l.id}
                      onClick={() => {
                        setLabelOpen(false);
                        onBulkLabel(l.id);
                      }}
                      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-secondary"
                    >
                      <span
                        className="h-2.5 w-2.5 rounded-full"
                        style={{ backgroundColor: l.color || 'hsl(var(--muted-foreground))' }}
                      />
                      {l.name}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          <button
            onClick={onClearSelection}
            className="ml-auto inline-flex h-8 items-center gap-1 rounded-md px-2 text-sm text-muted-foreground hover:text-foreground"
          >
            <X className="h-4 w-4" /> Clear
          </button>
        </div>
      ) : null}
    </div>
  );
}
