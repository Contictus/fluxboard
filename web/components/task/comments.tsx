'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { addComment, deleteComment, editComment, listComments } from '@/lib/api/tasks';
import { useOrg } from '@/lib/org/context';
import { useAuth } from '@/lib/auth/context';
import { useOrgMembers, memberName } from '@/lib/org/use-members';
import { useToast } from '@/components/ui/toast';
import { MentionInput } from './mention-input';
import type { Comment } from '@/lib/api/types';

const EDIT_WINDOW_MS = 15 * 60 * 1000;

// Comment thread (FR-TASK-005). Markdown bodies render as pre-wrapped text (a
// full markdown renderer is out of scope this section). Authors may edit within
// a 15-minute window; the author or a LEAD/ADMIN may delete. @mention
// autocomplete comes from MentionInput.
export function Comments({ taskId, canComment }: { taskId: string; canComment: boolean }) {
  const { orgId, isAdmin } = useOrg();
  const { userId } = useAuth();
  const { byId } = useOrgMembers();
  const { toast } = useToast();
  const qc = useQueryClient();
  const key = ['comments', orgId, taskId] as const;

  const { data: comments } = useQuery({ queryKey: key, queryFn: () => listComments(orgId, taskId) });
  const [draft, setDraft] = useState('');
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editBody, setEditBody] = useState('');

  const invalidate = () => qc.invalidateQueries({ queryKey: key });
  const onError = () => toast({ title: 'Comment action failed', variant: 'error' });

  const add = useMutation({
    mutationFn: (body: string) => addComment(orgId, taskId, body),
    onSuccess: () => {
      setDraft('');
      invalidate();
    },
    onError,
  });
  const edit = useMutation({
    mutationFn: (v: { id: string; body: string }) => editComment(orgId, taskId, v.id, v.body),
    onSuccess: () => {
      setEditingId(null);
      invalidate();
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (id: string) => deleteComment(orgId, taskId, id),
    onSuccess: invalidate,
    onError,
  });

  const list = comments ?? [];

  function withinWindow(c: Comment): boolean {
    return Date.now() - new Date(c.created_at).getTime() < EDIT_WINDOW_MS;
  }

  return (
    <section>
      <h3 className="mb-2 text-sm font-semibold">Comments</h3>

      <ul className="space-y-4">
        {list.map((c) => {
          const deleted = Boolean(c.deleted_at);
          const mine = c.author_id === userId;
          return (
            <li key={c.id} className="text-sm">
              <div className="flex items-center gap-2">
                <span className="font-medium">{memberName(byId, c.author_id)}</span>
                <span className="text-xs text-muted-foreground">
                  {new Date(c.created_at).toLocaleString()}
                </span>
                {c.edited && !deleted ? (
                  <span className="text-xs text-muted-foreground">(edited)</span>
                ) : null}
              </div>

              {deleted ? (
                <p className="mt-1 italic text-muted-foreground">Comment deleted.</p>
              ) : editingId === c.id ? (
                <div className="mt-1">
                  <MentionInput value={editBody} onChange={setEditBody} rows={2} />
                  <div className="mt-1 flex gap-2">
                    <button
                      type="button"
                      onClick={() => editBody.trim() && edit.mutate({ id: c.id, body: editBody.trim() })}
                      disabled={edit.isPending || !editBody.trim()}
                      className="rounded bg-primary px-2 py-1 text-xs font-medium text-primary-foreground disabled:opacity-50"
                    >
                      Save
                    </button>
                    <button
                      type="button"
                      onClick={() => setEditingId(null)}
                      className="text-xs text-muted-foreground hover:text-foreground"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <p className="mt-1 whitespace-pre-wrap">{c.body}</p>
                  {(mine || isAdmin) && (mine ? withinWindow(c) : true) ? (
                    <div className="mt-1 flex gap-3 text-xs text-muted-foreground">
                      {mine && withinWindow(c) ? (
                        <button
                          type="button"
                          onClick={() => {
                            setEditingId(c.id);
                            setEditBody(c.body);
                          }}
                          className="hover:text-foreground"
                        >
                          Edit
                        </button>
                      ) : null}
                      <button
                        type="button"
                        onClick={() => remove.mutate(c.id)}
                        className="hover:text-destructive"
                      >
                        Delete
                      </button>
                    </div>
                  ) : null}
                </>
              )}
            </li>
          );
        })}
        {list.length === 0 ? <li className="text-sm text-muted-foreground">No comments yet.</li> : null}
      </ul>

      {canComment ? (
        <div className="mt-4">
          <MentionInput
            value={draft}
            onChange={setDraft}
            placeholder="Write a comment… use @ to mention someone"
          />
          <div className="mt-2">
            <button
              type="button"
              onClick={() => draft.trim() && add.mutate(draft.trim())}
              disabled={add.isPending || !draft.trim()}
              className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              Comment
            </button>
          </div>
        </div>
      ) : null}
    </section>
  );
}
