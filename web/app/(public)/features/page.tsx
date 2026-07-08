import type { Metadata } from 'next';
import {
  Kanban,
  Users,
  CreditCard,
  Activity,
  ShieldCheck,
  Search,
  Bell,
  BarChart3,
} from 'lucide-react';

export const metadata: Metadata = { title: 'Features' };

const groups = [
  {
    icon: Kanban,
    title: 'Boards & tasks',
    body: 'Kanban boards with drag-drop ordering, WIP limits, subtasks, labels, comments with @mentions, and attachments. A list view and full-text search keep large projects navigable.',
  },
  {
    icon: Users,
    title: 'Teams & permissions',
    body: 'Organizations with role-based access (Owner, Admin, Member, Guest), project-level membership, and email invitations. Every tenant is isolated at the database level.',
  },
  {
    icon: CreditCard,
    title: 'Usage-based billing',
    body: 'Stripe Checkout and Billing Portal, metered seats/storage/API calls, plan enforcement, proration previews, and invoice history.',
  },
  {
    icon: Activity,
    title: 'Realtime',
    body: 'Server-sent events stream board and task changes to every connected client with surgical cache updates — no manual refresh.',
  },
  {
    icon: Bell,
    title: 'Notifications',
    body: 'An in-app notification center with unread tracking plus per-channel preferences.',
  },
  {
    icon: Search,
    title: 'Search & trash',
    body: 'Full-text task search with filters, a cross-project my-tasks view, and a trash with restore/purge.',
  },
  {
    icon: BarChart3,
    title: 'Analytics',
    body: 'Project analytics — completed-per-week, cumulative flow, cycle time, and per-assignee breakdowns — from nightly rollups.',
  },
  {
    icon: ShieldCheck,
    title: 'Security & audit',
    body: 'Two-factor authentication, session management, refresh-token rotation, and an org-scoped audit log with CSV export.',
  },
];

export default function FeaturesPage() {
  return (
    <div className="container py-16">
      <div className="mx-auto mb-12 max-w-2xl text-center">
        <h1 className="text-4xl font-bold tracking-tight">Everything your team needs</h1>
        <p className="mt-3 text-muted-foreground">
          A complete project-management platform with billing baked in.
        </p>
      </div>
      <div className="grid gap-8 md:grid-cols-2">
        {groups.map((g) => (
          <div key={g.title} className="flex gap-4">
            <g.icon className="mt-1 h-6 w-6 shrink-0 text-primary" />
            <div>
              <h2 className="font-semibold">{g.title}</h2>
              <p className="mt-1 text-sm text-muted-foreground">{g.body}</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
