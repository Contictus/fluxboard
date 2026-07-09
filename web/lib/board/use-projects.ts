'use client';

import { useQuery } from '@tanstack/react-query';

import { listProjects } from '@/lib/api/projects';
import { useOrg } from '@/lib/org/context';
import type { Project } from '@/lib/api/types';

// All projects (active + archived, deduped) with a byId map. Shared by the
// org-level task views (search / my-tasks / trash) that need to turn a task's
// project_id into a key for its detail URL and to list projects as a filter.
async function fetchAllProjects(orgId: string): Promise<Project[]> {
  const [active, archived] = await Promise.all([
    listProjects(orgId, {}),
    listProjects(orgId, { archived: true }),
  ]);
  const map = new Map<string, Project>();
  for (const p of [...active, ...archived]) map.set(p.id, p);
  return [...map.values()];
}

export function useProjectsMap(): {
  projects: Project[];
  byId: Map<string, Project>;
  isLoading: boolean;
} {
  const { orgId } = useOrg();
  const { data, isLoading } = useQuery({
    queryKey: ['projects', orgId, 'all'],
    queryFn: () => fetchAllProjects(orgId),
  });
  const projects = data ?? [];
  const byId = new Map(projects.map((p) => [p.id, p]));
  return { projects, byId, isLoading };
}
