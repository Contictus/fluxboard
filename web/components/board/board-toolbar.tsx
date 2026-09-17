'use client';

import { useState } from 'react';
import { Filter, CheckSquare, X } from 'lucide-react';

import type { BoardColumn, Label, Priority } from '@/lib/api/types';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import { cn } from '@/lib/utils';

export interface BoardFilter {
  text: string;
  priority: Priority | '';
  assignee: 'any' | 'me' | 'unassigned';
}

export const EMPTY_FILTER: BoardFilter = { text: '', priority: '', assignee: 'any' };

const selectCls =
  'h-9 rounded-sm border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

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
    <div className="mb-5 flex flex-wrap items-center gap-2 border-y border-border py-3">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Filter className="h-4 w-4" />
      </div>
      <input
        value={filter.text}
        onChange={(e) => onFilter({ ...filter, text: e.target.value })}
        placeholder="Filter tasks…"
        className="h-9 w-52 rounded-sm border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
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

      <button
        onClick={onToggleSelectMode}
        className={cn(
          'ml-auto inline-flex h-9 items-center gap-2 rounded-sm border px-3 text-sm transition-colors',
          selectMode ? 'border-primary bg-primary/10 text-primary' : 'hover:bg-secondary',
        )}
      >
        <CheckSquare className="h-4 w-4" /> {selectMode ? 'Selecting' : 'Select'}
      </button>

      {selectMode && selectedCount > 0 ? (
        <div className="flex w-full items-center gap-2 border-l-2 border-primary bg-card px-3 py-2">
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
