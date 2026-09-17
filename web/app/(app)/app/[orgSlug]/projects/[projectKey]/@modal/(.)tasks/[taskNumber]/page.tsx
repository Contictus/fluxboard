'use client';

import { useRouter } from 'next/navigation';
import { useEffect } from 'react';
import { X } from 'lucide-react';

import { useProjectByKey } from '@/lib/board/use-project';
import { useTaskIdByNumber } from '@/lib/board/use-task';
import { TaskDetail } from '@/components/task/task-detail';

// Intercepted task route (ADR-018): clicking a card opens the task detail in a
// modal over the board instead of navigating away. Refresh/direct load still
// serves the full page at .../tasks/[taskNumber].
export default function InterceptedTaskModal({
  params,
}: {
  params: { projectKey: string; taskNumber: string };
}) {
  const router = useRouter();
  const number = Number(params.taskNumber);
  const { project } = useProjectByKey(params.projectKey);
  const { taskId, isLoading, isError } = useTaskIdByNumber(project?.id, number);

  function close() {
    router.back();
  }

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') close();
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 sm:p-8"
      onClick={close}
      role="dialog"
      aria-modal="true"
      aria-label={Number.isNaN(number) ? 'Task' : `Task #${number}`}
    >
      <div
        className="relative w-full max-w-3xl rounded-3xl bg-background p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <button
          onClick={close}
          aria-label="Close"
          className="absolute right-4 top-4 rounded-full p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground"
        >
          <X className="h-4 w-4" />
        </button>
        {Number.isNaN(number) || isError || (!isLoading && !taskId) ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            Couldn’t load this task.{' '}
            <button onClick={close} className="text-primary hover:underline">
              Back to board
            </button>
          </p>
        ) : isLoading || !project || !taskId ? (
          <p className="py-8 text-center text-sm text-muted-foreground">Loading task…</p>
        ) : (
          <TaskDetail taskId={taskId} projectId={project.id} projectKey={project.key} />
        )}
      </div>
    </div>
  );
}
