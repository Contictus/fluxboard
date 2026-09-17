'use client';

import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { CircleCheck, FolderKanban, Bell } from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useAuth } from '@/lib/auth/context';
import { useOrg } from '@/lib/org/context';
import { searchTasks } from '@/lib/api/tasks';
import { listProjects } from '@/lib/api/projects';
import { listNotifications } from '@/lib/api/notifications';

export default function OrgHomePage() {
  const { org } = useOrg();

  return (
    <div className="mx-auto max-w-5xl px-6 py-8 animate-fade-in">
      <h1 className="mb-1 text-2xl font-bold tracking-tight">{org.name}</h1>
      <p className="mb-6 text-sm text-muted-foreground">Welcome back. Here&apos;s what&apos;s happening.</p>
      <div className="stagger-children grid gap-6 lg:grid-cols-2">
        <AssignedToMe />
        <RecentActivity />
        <Projects />
      </div>
    </div>
  );
}

function Panel({
  title,
  icon,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Card className="group">
      <CardHeader className="flex-row items-center gap-2 space-y-0">
        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/10 text-primary transition-colors group-hover:bg-primary group-hover:text-primary-foreground">
          {icon}
        </div>
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-4 w-3/4" />
      <Skeleton className="h-4 w-5/6" />
    </div>
  );
}
function Failed() {
  return <p className="text-sm text-destructive">Couldn&apos;t load.</p>;
}
function Empty({ text }: { text: string }) {
  return <p className="text-sm text-muted-foreground">{text}</p>;
}

function AssignedToMe() {
  const { orgId, slug } = useOrg();
  const { userId } = useAuth();

  const { data, isLoading, isError } = useQuery({
    queryKey: ['home-assigned', orgId, userId],
    queryFn: () => searchTasks(orgId, { assignee_id: userId ?? '', limit: 8 }),
    enabled: Boolean(userId),
  });

  return (
    <Panel title="Assigned to me" icon={<CircleCheck className="h-4 w-4" />}>
      {isLoading ? (
        <LoadingSkeleton />
      ) : isError ? (
        <Failed />
      ) : !data || data.tasks.length === 0 ? (
        <Empty text="Nothing assigned to you right now." />
      ) : (
        <ul className="space-y-1">
          {data.tasks.map((t) => (
            <li
              key={t.id}
              className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-secondary/60"
            >
              <span className="truncate">{t.title}</span>
              <span className="ml-2 shrink-0 text-xs text-muted-foreground">
                {t.priority.toLowerCase()}
              </span>
            </li>
          ))}
        </ul>
      )}
      {data && data.total > data.tasks.length ? (
        <p className="mt-2 text-xs text-muted-foreground">
          Showing {data.tasks.length} of {data.total}.
        </p>
      ) : null}
      <Link
        href={`/app/${slug}/projects`}
        className="mt-3 inline-block text-xs text-primary hover:underline"
      >
        View all projects →
      </Link>
    </Panel>
  );
}

function RecentActivity() {
  const { orgId } = useOrg();

  const { data, isLoading, isError } = useQuery({
    queryKey: ['home-activity', orgId],
    queryFn: () => listNotifications(orgId, { limit: 8 }),
  });

  return (
    <Panel title="Recent activity" icon={<Bell className="h-4 w-4" />}>
      {isLoading ? (
        <LoadingSkeleton />
      ) : isError ? (
        <Failed />
      ) : !data || data.length === 0 ? (
        <Empty text="No recent activity." />
      ) : (
        <ul className="space-y-2">
          {data.map((n) => (
            <li key={n.id} className="text-sm">
              <p className={n.read_at ? 'text-muted-foreground' : 'font-medium'}>{n.title}</p>
              {n.body ? <p className="text-xs text-muted-foreground">{n.body}</p> : null}
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Projects() {
  const { orgId, slug } = useOrg();

  const { data, isLoading, isError } = useQuery({
    queryKey: ['home-projects', orgId],
    queryFn: () => listProjects(orgId, {}),
  });

  const recent = data?.slice(0, 6);

  return (
    <Panel title="Projects" icon={<FolderKanban className="h-4 w-4" />}>
      {isLoading ? (
        <LoadingSkeleton />
      ) : isError ? (
        <Failed />
      ) : !recent || recent.length === 0 ? (
        <Empty text="No projects yet." />
      ) : (
        <ul className="space-y-1">
          {recent.map((p) => (
            <li key={p.id}>
              <Link
                href={`/app/${slug}/projects/${p.key}`}
                className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-secondary/60"
              >
                <span
                  className="h-2.5 w-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: p.color || 'hsl(var(--muted-foreground))' }}
                />
                <span className="truncate">{p.name}</span>
                <span className="ml-auto shrink-0 text-xs text-muted-foreground">{p.key}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
