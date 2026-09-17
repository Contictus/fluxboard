'use client';

import { useEffect, useState } from 'react';

import { ApiError } from '@/lib/api/client';
import { getPublicForm, submitPublicForm } from '@/lib/api/forms';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Field } from '@/components/auth/field';

const PRIORITIES = [
  { value: '', label: 'No priority' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'urgent', label: 'Urgent' },
] as const;

// Public intake form (ADR-023). No auth — the token in the URL is the only
// credential. Unknown/disabled forms render the same "unavailable" state.
export default function PublicFormPage({ params }: { params: { token: string } }) {
  const [form, setForm] = useState<{ name: string; description: string } | null>(null);
  const [missing, setMissing] = useState(false);

  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState('');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [receipt, setReceipt] = useState<number | null>(null);

  useEffect(() => {
    getPublicForm(params.token).then(setForm).catch(setMissingTrue);
    function setMissingTrue() {
      setMissing(true);
    }
  }, [params.token]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setFieldError(null);
    setSubmitting(true);
    try {
      const res = await submitPublicForm(params.token, {
        title: title.trim(),
        description: description.trim(),
        priority: priority || undefined,
        submitter_name: name.trim() || undefined,
        submitter_email: email.trim() || undefined,
      });
      setReceipt(res.number);
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        setFieldError('Please check the highlighted fields and try again.');
      } else if (err instanceof ApiError && err.status === 404) {
        setMissing(true);
      } else {
        setFieldError('Something went wrong. Please try again later.');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-xl px-6 py-12">
      {missing ? (
        <div className="rounded-2xl bg-card p-8 text-center shadow-sm">
          <h1 className="text-lg font-semibold">This form is unavailable</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            The link is invalid, the form was paused, or the project was archived.
          </p>
        </div>
      ) : !form ? (
        <p className="py-8 text-center text-sm text-muted-foreground">Loading form…</p>
      ) : receipt !== null ? (
        <div className="rounded-2xl bg-card p-8 text-center shadow-sm">
          <h1 className="text-lg font-semibold">Thanks — we got it</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Your request was filed as task #{receipt}. The team will take it from here.
          </p>
        </div>
      ) : (
        <div className="rounded-2xl bg-card p-8 shadow-sm">
          <h1 className="text-xl font-semibold">{form.name}</h1>
          {form.description ? (
            <p className="mt-2 text-sm text-muted-foreground">{form.description}</p>
          ) : null}
          <form onSubmit={onSubmit} className="mt-6 space-y-4">
            <Field id="f-title" label="Title">
              <Input
                id="f-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="What do you need?"
                required
                maxLength={200}
              />
            </Field>
            <Field id="f-desc" label="Details">
              <textarea
                id="f-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={4}
                placeholder="Anything that helps us understand the request…"
                className="flex w-full rounded-xl border-0 bg-secondary/60 px-4 py-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="f-priority" label="Priority">
                <select
                  id="f-priority"
                  value={priority}
                  onChange={(e) => setPriority(e.target.value)}
                  className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm"
                >
                  {PRIORITIES.map((p) => (
                    <option key={p.value} value={p.value}>
                      {p.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field id="f-name" label="Your name (optional)">
                <Input
                  id="f-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Ada Lovelace"
                  maxLength={120}
                />
              </Field>
            </div>
            <Field id="f-email" label="Your email (optional)">
              <Input
                id="f-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="ada@example.com"
                maxLength={120}
              />
            </Field>
            {fieldError ? <p className="text-sm text-destructive">{fieldError}</p> : null}
            <Button type="submit" className="w-full" disabled={submitting || !title.trim()}>
              {submitting ? 'Sending…' : 'Submit request'}
            </Button>
          </form>
        </div>
      )}
    </div>
  );
}
