'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';

import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { getBoard } from '@/lib/api/board';
import { ProjectHeader } from '@/components/board/project-header';
import { priorityClass, priorityLabel, PRIORITIES } from '@/lib/board/priority';
import type { Priority } from '@/lib/api/types';
import { cn } from '@/lib/utils';

interface Row {
  id: string;
  number: number;
  title: string;
  columnName: string;
  priority: Priority;
  assigned: boolean;
}

type SortKey = 'number' | 'title' | 'column' | 'priority';

const PRIORITY_ORDER = new Map(PRIORITIES.map((p, i) => [p, i]));

export default function ProjectListPage({ params }: { params: { projectKey: string } }) {
  const { orgId, slug } = useOrg();
  const { project, isLoading, isError } = useProjectByKey(params.projectKey);

  const board = useQuery({
    queryKey: ['board', project?.id],
    queryFn: () => getBoard(orgId, project!.id),
    enabled: Boolean(project),
  });

  const [sort, setSort] = useState<SortKey>('number');
  const [asc, setAsc] = useState(true);

  const rows = useMemo<Row[]>(() => {
    if (!board.data) return [];
    const out: Row[] = [];
    for (const col of board.data.columns) {
      for (const t of col.tasks) {
        out.push({
          id: t.id,
          number: t.number,
          title: t.title,
          columnName: col.name,
          priority: t.priority,
          assigned: Boolean(t.assignee_id),
        });
      }
    }
    const dir = asc ? 1 : -1;
    out.sort((a, b) => {
      switch (sort) {
        case 'title':
          return dir * a.title.localeCompare(b.title);
        case 'column':
          return dir * a.columnName.localeCompare(b.columnName);
        case 'priority':
          return dir * ((PRIORITY_ORDER.get(a.priority) ?? 99) - (PRIORITY_ORDER.get(b.priority) ?? 99));
        default:
          return dir * (a.number - b.number);
      }
    });
    return out;
  }, [board.data, sort, asc]);

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

  function header(key: SortKey, label: string) {
    return (
      <button
        onClick={() => (sort === key ? setAsc((v) => !v) : (setSort(key), setAsc(true)))}
        className="flex items-center gap-1 font-medium hover:text-foreground"
      >
        {label}
        {sort === key ? <span className="text-xs">{asc ? '▲' : '▼'}</span> : null}
      </button>
    );
  }

  return (
    <div className="px-6 py-6">
      <ProjectHeader project={project} />

      {board.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading tasks…</p>
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">No tasks yet.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="border-b bg-secondary/40 text-left text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">#</th>
                <th className="px-3 py-2">{header('title', 'Title')}</th>
                <th className="px-3 py-2">{header('column', 'Status')}</th>
                <th className="px-3 py-2">{header('priority', 'Priority')}</th>
                <th className="px-3 py-2 font-medium">Assignee</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className="border-b last:border-0 hover:bg-secondary/40">
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{r.number}</td>
                  <td className="px-3 py-2">
                    <Link
                      href={`/app/${slug}/projects/${project.key}/tasks/${r.number}`}
                      className="hover:underline"
                    >
                      {r.title}
                    </Link>
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">{r.columnName}</td>
                  <td className="px-3 py-2">
                    {r.priority !== 'none' ? (
                      <span className={cn('rounded px-1.5 py-0.5 text-xs font-medium', priorityClass(r.priority))}>
                        {priorityLabel(r.priority)}
                      </span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">{r.assigned ? 'Assigned' : 'Unassigned'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
