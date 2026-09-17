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
    <div className="mx-auto max-w-[1320px] px-6 py-10 animate-fade-in lg:px-10">
      <div className="mb-10 flex items-end justify-between gap-6 border-b border-border pb-6">
        <div>
          <p className="mb-3 font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-primary">Workspace / overview</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] sm:text-4xl">{org.name}</h1>
          <p className="mt-2 text-sm text-muted-foreground">A clear view of the work moving through your team.</p>
        </div>
        <span className="hidden font-mono text-[11px] text-muted-foreground sm:block">LIVE / {new Date().toLocaleDateString('en-GB')}</span>
      </div>
      <div className="stagger-children grid gap-x-12 gap-y-10 lg:grid-cols-[1.35fr_0.9fr]">
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
    <Card className="group rounded-none border-0 border-t border-border bg-transparent shadow-none hover:shadow-none">
      <CardHeader className="flex-row items-center gap-3 px-0 py-4">
        <div className="flex h-6 w-6 items-center justify-center rounded-sm bg-primary/10 text-primary">
          {icon}
        </div>
        <CardTitle className="font-mono text-[11px] font-medium uppercase tracking-[0.16em]">{title}</CardTitle>
      </CardHeader>
      <CardContent className="px-0 pb-0">{children}</CardContent>
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
              className="flex items-center justify-between border-b border-border/70 px-0 py-3 text-sm transition-colors hover:text-primary"
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
        className="mt-4 inline-block font-mono text-[11px] uppercase tracking-wider text-primary hover:text-foreground"
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
            <li key={n.id} className="border-b border-border/70 py-3 text-sm last:border-0">
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
                className="flex items-center gap-3 border-b border-border/70 px-0 py-3 text-sm transition-colors hover:text-primary"
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
