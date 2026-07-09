import type { Priority } from '@/lib/api/types';

// Shared priority presentation for board cards, the list view and filters.

export const PRIORITIES: Priority[] = ['urgent', 'high', 'medium', 'low', 'none'];

const STYLES: Record<Priority, string> = {
  urgent: 'bg-red-500/15 text-red-600 dark:text-red-400',
  high: 'bg-orange-500/15 text-orange-600 dark:text-orange-400',
  medium: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  low: 'bg-sky-500/15 text-sky-600 dark:text-sky-400',
  none: 'bg-secondary text-muted-foreground',
};

export function priorityClass(p: Priority): string {
  return STYLES[p] ?? STYLES.none;
}

export function priorityLabel(p: Priority): string {
  return p === 'none' ? 'No priority' : p.charAt(0).toUpperCase() + p.slice(1);
}
