import { createColumn, deleteColumn, getBoard, renameColumn } from '@/lib/api/board';
import { createLabel } from '@/lib/api/labels';
import { createProject } from '@/lib/api/projects';
import type { Project, Visibility } from '@/lib/api/types';

export interface ProjectTemplate {
  key: string;
  name: string;
  description: string;
  columns: string[];
  labels: { name: string; color: string }[];
}

export const PROJECT_TEMPLATES: ProjectTemplate[] = [
  {
    key: 'blank',
    name: 'Blank',
    description: 'The default board. Shape it yourself.',
    columns: ['Backlog', 'Todo', 'In Progress', 'Done'],
    labels: [],
  },
  {
    key: 'sprint',
    name: 'Software sprint',
    description: 'Backlog to Done with a review step, plus bug/feature labels.',
    columns: ['Backlog', 'To Do', 'In Progress', 'In Review', 'Done'],
    labels: [
      { name: 'bug', color: '#ef4444' },
      { name: 'feature', color: '#3b82f6' },
      { name: 'chore', color: '#6b7280' },
    ],
  },
  {
    key: 'bugs',
    name: 'Bug tracking',
    description: 'Triage flow from report to closed, with severity labels.',
    columns: ['New', 'Triaged', 'In Progress', 'In Review', 'Closed'],
    labels: [
      { name: 'critical', color: '#dc2626' },
      { name: 'major', color: '#f59e0b' },
      { name: 'minor', color: '#3b82f6' },
    ],
  },
  {
    key: 'marketing',
    name: 'Marketing campaign',
    description: 'Content pipeline from idea to published.',
    columns: ['Ideas', 'Planned', 'In Production', 'Review', 'Published'],
    labels: [
      { name: 'blog', color: '#8b5cf6' },
      { name: 'social', color: '#06b6d4' },
      { name: 'email', color: '#f59e0b' },
    ],
  },
];

export interface TemplateProjectInput {
  key: string;
  name: string;
  description: string;
  color: string;
  visibility: Visibility;
}

// Create a project from a template: base project, then reshape the seeded
// columns (rename in place, create extras, drop surplus) and seed labels.
// The project starts empty so dropping columns never orphans tasks.
export async function createProjectFromTemplate(
  orgId: string,
  input: TemplateProjectInput,
  template: ProjectTemplate,
): Promise<Project> {
  const project = await createProject(orgId, { ...input });
  const board = await getBoard(orgId, project.id);
  const seeded = board.columns;

  const keep = Math.min(seeded.length, template.columns.length);
  for (let i = 0; i < keep; i++) {
    const current = seeded[i]!;
    const want = template.columns[i]!;
    if (current.name !== want) {
      await renameColumn(orgId, project.id, current.id, { name: want });
    }
  }
  for (let i = keep; i < template.columns.length; i++) {
    await createColumn(orgId, project.id, { name: template.columns[i]! });
  }
  if (seeded.length > template.columns.length) {
    const target = seeded[0]!.id;
    for (let i = template.columns.length; i < seeded.length; i++) {
      await deleteColumn(orgId, project.id, seeded[i]!.id, target);
    }
  }
  for (const label of template.labels) {
    try {
      await createLabel(orgId, label);
    } catch {
      // Label names may already exist in the org; template apply continues.
    }
  }
  return project;
}
