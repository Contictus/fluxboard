'use client';

import { useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, Paperclip, Trash2, Upload } from 'lucide-react';

import {
  attachmentDownloadUrl,
  confirmUpload,
  deleteAttachment,
  listAttachments,
  requestUpload,
} from '@/lib/api/tasks';
import { ApiError } from '@/lib/api/client';
import { useOrg } from '@/lib/org/context';
import { useToast } from '@/components/ui/toast';

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// Attachment list + upload (FR-TASK-006). Upload is a three-step dance: request
// a presigned PUT, PUT the bytes straight to MinIO (no auth header — the URL is
// pre-signed), then confirm so the API commits the row. Download resolves a
// short-lived presigned GET and opens it.
export function Attachments({ taskId, canEdit }: { taskId: string; canEdit: boolean }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const qc = useQueryClient();
  const key = ['attachments', orgId, taskId] as const;
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);

  const { data: attachments } = useQuery({
    queryKey: key,
    queryFn: () => listAttachments(orgId, taskId),
  });

  async function upload(file: File) {
    setUploading(true);
    try {
      const { attachment, upload_url } = await requestUpload(orgId, taskId, {
        filename: file.name,
        content_type: file.type || 'application/octet-stream',
        size: file.size,
      });
      const put = await fetch(upload_url, {
        method: 'PUT',
        body: file,
        headers: { 'Content-Type': file.type || 'application/octet-stream' },
      });
      if (!put.ok) throw new Error(`upload failed (${put.status})`);
      await confirmUpload(orgId, taskId, attachment.id);
      qc.invalidateQueries({ queryKey: key });
      toast({ title: 'Attachment uploaded', variant: 'success' });
    } catch (err) {
      const msg =
        err instanceof ApiError && err.status === 402
          ? 'Storage limit reached — upgrade your plan.'
          : 'Upload failed. Please try again.';
      toast({ title: 'Couldn’t upload', description: msg, variant: 'error' });
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  }

  const remove = useMutation({
    mutationFn: (id: string) => deleteAttachment(orgId, taskId, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
    onError: () => toast({ title: 'Couldn’t delete attachment', variant: 'error' }),
  });

  async function download(id: string) {
    try {
      const url = await attachmentDownloadUrl(orgId, taskId, id);
      window.open(url, '_blank', 'noopener');
    } catch {
      toast({ title: 'Couldn’t get download link', variant: 'error' });
    }
  }

  const list = (attachments ?? []).filter((a) => a.status !== 'pending');

  return (
    <section>
      <h3 className="mb-2 text-sm font-semibold">Attachments</h3>

      <ul className="space-y-1">
        {list.map((a) => (
          <li key={a.id} className="group flex items-center gap-2 rounded px-1 py-1 hover:bg-secondary/50">
            <Paperclip className="h-4 w-4 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate text-sm">{a.filename}</span>
            <span className="text-xs text-muted-foreground">{humanSize(a.size_bytes)}</span>
            <button
              type="button"
              onClick={() => download(a.id)}
              aria-label={`Download ${a.filename}`}
              className="text-muted-foreground hover:text-foreground"
            >
              <Download className="h-4 w-4" />
            </button>
            {canEdit ? (
              <button
                type="button"
                onClick={() => remove.mutate(a.id)}
                aria-label={`Delete ${a.filename}`}
                className="opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            ) : null}
          </li>
        ))}
        {list.length === 0 ? <li className="text-sm text-muted-foreground">No attachments.</li> : null}
      </ul>

      {canEdit ? (
        <div className="mt-2">
          <input
            ref={fileRef}
            type="file"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void upload(f);
            }}
          />
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            disabled={uploading}
            className="inline-flex items-center gap-1.5 rounded-md border border-dashed px-3 py-1.5 text-sm text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
          >
            <Upload className="h-4 w-4" /> {uploading ? 'Uploading…' : 'Upload file'}
          </button>
        </div>
      ) : null}
    </section>
  );
}
