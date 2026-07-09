'use client';

import { useQuery } from '@tanstack/react-query';

import { getBoard } from '@/lib/api/board';
import { useOrg } from '@/lib/org/context';

// Resolves a `/tasks/{taskNumber}` URL segment to a task UUID. There is no
// by-number API route and search carries no number filter (ADR-018), but the
// board projection lists every card's `number` + `id`, so we resolve through it.
// The board query is shared with the board page, so navigating from a card is a
// cache hit; a direct load fetches the board once.
export function useTaskIdByNumber(
  projectId: string | undefined,
  taskNumber: number,
): { taskId: string | null | undefined; isLoading: boolean; isError: boolean } {
  const { orgId } = useOrg();
  const { data, isLoading, isError } = useQuery({
    queryKey: ['board', projectId],
    queryFn: () => getBoard(orgId, projectId as string),
    enabled: Boolean(projectId),
  });

  if (!data) return { taskId: data as null | undefined, isLoading, isError };

  const card = data.columns.flatMap((c) => c.tasks).find((t) => t.number === taskNumber);
  return { taskId: card?.id ?? null, isLoading, isError };
}
