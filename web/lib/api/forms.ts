import { apiFetch } from './client';

/** One intake form (GET .../projects/{id}/forms -> { forms: [...] }). Never carries a token. */
export interface IntakeForm {
  id: string;
  name: string;
  description: string;
  target_column_id: string;
  is_active: boolean;
  created_at: string;
}

/** Payload for POST .../projects/{id}/forms. */
export interface CreateFormInput {
  name: string;
  description?: string;
  target_column_id: string;
}

/** Payload for PATCH .../projects/{id}/forms/{formId}. */
export interface UpdateFormInput {
  name: string;
  description?: string;
  target_column_id: string;
  is_active: boolean;
}

/** List a project's intake forms oldest-first (VIEWER). */
export async function listForms(orgId: string, projectId: string): Promise<IntakeForm[]> {
  const res = await apiFetch<{ forms: IntakeForm[] }>(`/orgs/${orgId}/projects/${projectId}/forms`);
  return res.forms ?? [];
}

/** Create a form (201, LEAD). Returns the form plus the one-time raw token. */
export function createForm(
  orgId: string,
  projectId: string,
  input: CreateFormInput,
): Promise<{ form: IntakeForm; token: string }> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/forms`, { method: 'POST', body: input });
}

/** Rewrite a form (LEAD). */
export function updateForm(
  orgId: string,
  projectId: string,
  formId: string,
  input: UpdateFormInput,
): Promise<IntakeForm> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/forms/${formId}`, {
    method: 'PATCH',
    body: input,
  });
}

/** Swap a form's token, killing the old URL (LEAD). Returns the new raw token. */
export async function rotateFormToken(
  orgId: string,
  projectId: string,
  formId: string,
): Promise<string> {
  const res = await apiFetch<{ token: string }>(
    `/orgs/${orgId}/projects/${projectId}/forms/${formId}/rotate`,
    { method: 'POST' },
  );
  return res.token;
}

/** Delete a form (204, LEAD). Submitted tasks are untouched. */
export function deleteForm(orgId: string, projectId: string, formId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/forms/${formId}`, { method: 'DELETE' });
}

// ---- Public surface (no auth — the token is the credential) ------------------

/** Public form page data. 404 when the token is unknown, disabled, or archived. */
export function getPublicForm(token: string): Promise<{ name: string; description: string }> {
  return apiFetch(`/forms/${token}`);
}

/** Payload for POST /forms/{token}/submit. */
export interface SubmitFormInput {
  title: string;
  description?: string;
  priority?: string;
  submitter_name?: string;
  submitter_email?: string;
}

/** File an anonymous task (201 -> { id, number }). Per-IP throttled server-side. */
export function submitPublicForm(
  token: string,
  input: SubmitFormInput,
): Promise<{ id: string; number: number }> {
  return apiFetch(`/forms/${token}/submit`, { method: 'POST', body: input });
}
