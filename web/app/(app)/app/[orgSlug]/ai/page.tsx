'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { Brain, ListChecks, ScrollText, ShieldAlert } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { listProjects } from '@/lib/api/projects';
import { getBoard } from '@/lib/api/board';
import {
  applyPlan,
  chatTurn,
  dismissRisk,
  listRisks,
  parseTasks,
  planDraft,
  projectDigest,
  scanRisks,
} from '@/lib/api/ai';
import { ApiError } from '@/lib/api/client';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

const inputCls =
  'w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';
const selectCls =
  'w-full rounded-lg border border-input bg-background px-2 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

/** Shared error line: 402 points at billing, 403 at the org switch, else raw. */
function AIError({ error, slug }: { error: unknown; slug: string }) {
  if (!(error instanceof ApiError)) return null;
  if (error.code === 'plan_limit_exceeded') {
    return (
      <p className="text-sm text-destructive">
        Monthly AI budget used.{' '}
        <Link className="underline" href={`/app/${slug}/billing`}>
          Manage billing
        </Link>
      </p>
    );
  }
  if (error.code === 'forbidden') {
    return <p className="text-sm text-destructive">AI is disabled for this workspace.</p>;
  }
  return <p className="text-sm text-destructive">{error.message}</p>;
}

interface ChatMsg {
  role: 'you' | 'ai';
  text: string;
}

function ChatCard() {
  const { orgId, slug } = useOrg();
  const [draft, setDraft] = useState('');
  const [history, setHistory] = useState<ChatMsg[]>([]);
  const chat = useMutation({
    mutationFn: (message: string) =>
      chatTurn(
        orgId,
        message,
        history.filter((m) => m.role === 'ai').map((m) => m.text),
      ),
    onSuccess: (res, message) => {
      setHistory((h) => [...h, { role: 'you', text: message }, { role: 'ai', text: res.text }]);
      setDraft('');
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Brain className="h-4 w-4" /> Assistant
        </CardTitle>
        <CardDescription>Ask about your workspace. Every turn is ledgered and audited.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="max-h-64 space-y-2 overflow-y-auto rounded-lg border border-input p-3">
          {history.length === 0 && <p className="text-sm text-muted-foreground">No messages yet.</p>}
          {history.map((m, i) => (
            <p key={i} className="whitespace-pre-wrap text-sm">
              <span className="font-medium">{m.role === 'you' ? 'You' : 'AI'}: </span>
              {m.text}
            </p>
          ))}
        </div>
        <div className="flex gap-2">
          <input
            className={inputCls}
            placeholder="e.g. what is overdue in Payments?"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && draft.trim()) chat.mutate(draft.trim());
            }}
          />
          <Button isLoading={chat.isPending} disabled={!draft.trim()} onClick={() => chat.mutate(draft.trim())}>
            Send
          </Button>
        </div>
        <AIError error={chat.error} slug={slug} />
      </CardContent>
    </Card>
  );
}

function ParseCard() {
  const { orgId, slug } = useOrg();
  const [input, setInput] = useState('');
  const parse = useMutation({ mutationFn: () => parseTasks(orgId, input.trim()) });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ListChecks className="h-4 w-4" /> Task parser
        </CardTitle>
        <CardDescription>One sentence in, structured task drafts out.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <textarea
          className={inputCls}
          rows={3}
          placeholder="next week: finalise wireframes, kickoff design review, draft sprint demo"
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
        <Button isLoading={parse.isPending} disabled={!input.trim()} onClick={() => parse.mutate()}>
          Parse
        </Button>
        {parse.data && (
          <ul className="list-disc space-y-1 pl-5 text-sm">
            {parse.data.items.map((item, i) => (
              <li key={i}>{item}</li>
            ))}
          </ul>
        )}
        <AIError error={parse.error} slug={slug} />
      </CardContent>
    </Card>
  );
}

function PlanCard() {
  const { orgId, slug } = useOrg();
  const [brief, setBrief] = useState('');
  const [projectId, setProjectId] = useState('');
  const [columnId, setColumnId] = useState('');
  const plan = useMutation({ mutationFn: () => planDraft(orgId, brief.trim()) });
  const boardQ = useQuery({
    queryKey: ['board', orgId, projectId],
    queryFn: () => getBoard(orgId, projectId),
    enabled: projectId !== '',
  });
  const apply = useMutation({
    mutationFn: () =>
      applyPlan(orgId, {
        project_id: projectId,
        column_id: columnId,
        items: (plan.data?.items ?? []).map((title) => ({ title })),
      }),
  });
  const columns = boardQ.data?.columns ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ScrollText className="h-4 w-4" /> Plan draft
        </CardTitle>
        <CardDescription>Brief in, phased draft out. Apply writes real tasks, all or nothing.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <textarea
          className={inputCls}
          rows={3}
          placeholder="Launch usage-based billing for EU customers…"
          value={brief}
          onChange={(e) => setBrief(e.target.value)}
        />
        <Button isLoading={plan.isPending} disabled={!brief.trim()} onClick={() => plan.mutate()}>
          Draft plan
        </Button>
        {plan.data && <p className="whitespace-pre-wrap text-sm">{plan.data.text}</p>}
        {plan.data && plan.data.items.length > 0 && (
          <div className="space-y-3 rounded-lg border border-input p-3">
            <ProjectPicker
              projectId={projectId}
              onChange={(id) => {
                setProjectId(id);
                setColumnId('');
              }}
            />
            <select className={selectCls} value={columnId} onChange={(e) => setColumnId(e.target.value)}>
              <option value="">Select a column…</option>
              {columns.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <Button
              variant="secondary"
              isLoading={apply.isPending}
              disabled={!projectId || !columnId}
              onClick={() => apply.mutate()}
            >
              Apply {plan.data.items.length} tasks
            </Button>
            {apply.data && (
              <div className="text-sm">
                {apply.data.replayed && <p className="text-muted-foreground">Replayed — nothing duplicated.</p>}
                <ul className="list-disc pl-5">
                  {apply.data.tasks.map((t) => (
                    <li key={t.id}>
                      #{t.number} {t.title}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <AIError error={apply.error} slug={slug} />
          </div>
        )}
        <AIError error={plan.error} slug={slug} />
      </CardContent>
    </Card>
  );
}

function ProjectPicker({
  projectId,
  onChange,
}: {
  projectId: string;
  onChange: (id: string) => void;
}) {
  const { orgId } = useOrg();
  const projectsQ = useQuery({ queryKey: ['projects', orgId], queryFn: () => listProjects(orgId) });
  const projects = projectsQ.data ?? [];
  return (
    <select className={selectCls} value={projectId} onChange={(e) => onChange(e.target.value)}>
      <option value="">Select a project…</option>
      {projects.map((p) => (
        <option key={p.id} value={p.id}>
          {p.key} — {p.name}
        </option>
      ))}
    </select>
  );
}

function DigestCard() {
  const { orgId, slug } = useOrg();
  const [projectId, setProjectId] = useState('');
  const digest = useMutation({ mutationFn: () => projectDigest(orgId, projectId) });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ScrollText className="h-4 w-4" /> Status digest
        </CardTitle>
        <CardDescription>Numbers come from live data. The AI only narrates.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <ProjectPicker projectId={projectId} onChange={setProjectId} />
        <Button isLoading={digest.isPending} disabled={!projectId} onClick={() => digest.mutate()}>
          Generate digest
        </Button>
        {digest.data && (
          <div className="space-y-2 text-sm">
            <p>
              Total {digest.data.stats.total} · Overdue {digest.data.stats.overdue} · Unassigned{' '}
              {digest.data.stats.unassigned} · Urgent/high {digest.data.stats.urgent_high}
            </p>
            <p className="whitespace-pre-wrap">{digest.data.text}</p>
          </div>
        )}
        <AIError error={digest.error} slug={slug} />
      </CardContent>
    </Card>
  );
}

function RisksCard() {
  const { orgId, slug } = useOrg();
  const queryClient = useQueryClient();
  const [projectId, setProjectId] = useState('');
  const risksQ = useQuery({
    queryKey: ['ai-risks', orgId, projectId],
    queryFn: () => listRisks(orgId, projectId),
    enabled: projectId !== '',
  });
  const scan = useMutation({
    mutationFn: () => scanRisks(orgId, projectId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ai-risks', orgId, projectId] }),
  });
  const dismiss = useMutation({
    mutationFn: (taskId: string) => dismissRisk(orgId, taskId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ai-risks', orgId, projectId] }),
  });
  const risks = risksQ.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldAlert className="h-4 w-4" /> Risk review
        </CardTitle>
        <CardDescription>Overdue, ownership and capacity signals. Business plan only.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <ProjectPicker projectId={projectId} onChange={setProjectId} />
        <div className="flex gap-2">
          <Button
            variant="secondary"
            isLoading={scan.isPending}
            disabled={!projectId}
            onClick={() => scan.mutate()}
          >
            Scan now
          </Button>
        </div>
        {risks.map((r) => (
          <div key={r.id} className="flex items-start justify-between gap-2 rounded-lg border border-input p-3">
            <div className="text-sm">
              <p className="font-medium">
                {r.score} · {r.task_id.slice(0, 8)}
              </p>
              <ul className="list-disc pl-5 text-muted-foreground">
                {r.signals.map((s, i) => (
                  <li key={i}>
                    {s.code}: {s.detail}
                  </li>
                ))}
              </ul>
            </div>
            <Button variant="outline" size="sm" isLoading={dismiss.isPending} onClick={() => dismiss.mutate(r.task_id)}>
              Dismiss
            </Button>
          </div>
        ))}
        {projectId && risks.length === 0 && !risksQ.isLoading && (
          <p className="text-sm text-muted-foreground">No open risks. Scan to recompute.</p>
        )}
        <AIError error={scan.error ?? dismiss.error ?? risksQ.error} slug={slug} />
      </CardContent>
    </Card>
  );
}

export default function AIPage() {
  return (
    <div className="mx-auto grid max-w-4xl gap-4 p-4">
      <h1 className="text-xl font-semibold">AI Assistant</h1>
      <ChatCard />
      <ParseCard />
      <PlanCard />
      <DigestCard />
      <RisksCard />
    </div>
  );
}
