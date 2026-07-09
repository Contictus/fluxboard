'use client';

import { useQuery } from '@tanstack/react-query';

import { resolveProjectByKey } from '@/lib/api/projects';
import { useOrg } from '@/lib/org/context';
import type { Project } from '@/lib/api/types';

// Resolves a `/projects/{projectKey}` URL segment to its project (there is no
// by-key API route — ADR-017). Shared by the board, list and settings pages.
export function useProjectByKey(projectKey: string): {
  project: Project | null | undefined;
  isLoading: boolean;
  isError: boolean;
} {
  const { orgId } = useOrg();
  const { data, isLoading, isError } = useQuery({
    queryKey: ['project-by-key', orgId, projectKey],
    queryFn: () => resolveProjectByKey(orgId, projectKey),
  });
  return { project: data, isLoading, isError };
}
