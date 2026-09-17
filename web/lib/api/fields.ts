import { apiFetch } from './client';

export type CustomFieldType = 'text' | 'number' | 'date' | 'select';

/** One field definition (GET .../projects/{id}/fields -> { fields: [...] }). */
export interface CustomField {
  id: string;
  name: string;
  type: CustomFieldType;
  options: string[];
  position: number;
}

/** Payload for POST/PATCH field definitions. */
export interface CustomFieldInput {
  name: string;
  type: CustomFieldType;
  options: string[];
  position: number;
}

/** One task value (GET .../tasks/{id}/fields -> { values: [...] }). */
export interface CustomValue {
  field_id: string;
  name: string;
  type: CustomFieldType;
  text?: string | null;
  number?: number | null;
  date?: string | null;
}

/** Payload for PUT .../tasks/{id}/fields/{fieldId}. */
export interface CustomValueInput {
  text?: string | null;
  number?: number | null;
  date?: string | null;
}

export const FIELD_TYPES: { value: CustomFieldType; label: string }[] = [
  { value: 'text', label: 'Text' },
  { value: 'number', label: 'Number' },
  { value: 'date', label: 'Date' },
  { value: 'select', label: 'Select' },
];

/** List definitions in position order. */
export async function listCustomFields(orgId: string, projectId: string): Promise<CustomField[]> {
  const res = await apiFetch<{ fields: CustomField[] }>(`/orgs/${orgId}/projects/${projectId}/fields`);
  return res.fields ?? [];
}

/** Create a definition (201, LEAD). */
export function createCustomField(
  orgId: string,
  projectId: string,
  input: CustomFieldInput,
): Promise<CustomField> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/fields`, { method: 'POST', body: input });
}

/** Rename / re-option / reposition a definition (LEAD). */
export function updateCustomField(
  orgId: string,
  projectId: string,
  fieldId: string,
  input: CustomFieldInput,
): Promise<CustomField> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/fields/${fieldId}`, {
    method: 'PATCH',
    body: input,
  });
}

/** Delete a definition; values cascade (204, LEAD). */
export function deleteCustomField(orgId: string, projectId: string, fieldId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/fields/${fieldId}`, { method: 'DELETE' });
}

/** A task's values in field order. */
export async function listTaskFieldValues(orgId: string, taskId: string): Promise<CustomValue[]> {
  const res = await apiFetch<{ values: CustomValue[] }>(`/orgs/${orgId}/tasks/${taskId}/fields`);
  return res.values ?? [];
}

/** Set a task value (204, CONTRIBUTOR). */
export function setTaskFieldValue(
  orgId: string,
  taskId: string,
  fieldId: string,
  input: CustomValueInput,
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/fields/${fieldId}`, { method: 'PUT', body: input });
}

/** Clear a task value (204, CONTRIBUTOR). */
export function clearTaskFieldValue(orgId: string, taskId: string, fieldId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/fields/${fieldId}`, { method: 'DELETE' });
}
