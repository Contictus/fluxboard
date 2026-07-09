// Hand-authored API types mirroring docs/08-API-SPEC.md. The served OpenAPI spec
// only covers Phase-6 handlers, so the auth/org/account contracts are typed here
// directly and grown per section. Keep field names in sync with the Go handlers.

/** Error codes from the single error envelope (docs/08 §1). */
export type ApiErrorCode =
  | 'validation_failed'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'plan_limit_exceeded'
  | 'rate_limited'
  | 'internal';

/** The `error` object every non-2xx response carries. */
export interface ApiErrorBody {
  code: ApiErrorCode | string;
  message: string;
  details?: Record<string, unknown>;
  request_id?: string;
}

/** Session tokens returned by login / 2fa-verify / refresh (docs/04 §5). */
export interface TokenResponse {
  access_token: string;
  token_type: 'Bearer';
  expires_at: string; // RFC3339
  user_id: string;
}

/** Login may short-circuit into the TOTP challenge instead of a session. */
export interface TwoFactorRequired {
  status: '2fa_required';
  pending_token: string;
}

export type LoginResponse = TokenResponse | TwoFactorResponse;
export type TwoFactorResponse = TwoFactorRequired;

export function isTwoFactorRequired(r: LoginResponse): r is TwoFactorRequired {
  return 'status' in r && r.status === '2fa_required';
}

/** A live session row (GET /auth/sessions). */
export interface SessionInfo {
  id: string;
  user_agent: string;
  ip?: string;
  current: boolean;
  created_at: string;
  last_used_at: string;
  expires_at: string;
}

/** 2FA enrollment payload (POST /auth/2fa/enroll). */
export interface Enroll2FAResponse {
  secret: string;
  provisioning_uri: string;
}

/** Membership roles (domain/tenant/tenant.go). Note GUEST, not "VIEWER". */
export type Role = 'OWNER' | 'ADMIN' | 'MEMBER' | 'GUEST';

/** Org membership summary row (GET /orgs -> { items: [...] }). */
export interface OrgSummary {
  org_id: string;
  role: Role;
  slug: string;
  name: string;
  created_at: string;
}

/** Full org (GET /orgs/{id}, GET /orgs/by-slug/{slug}, POST /orgs). */
export interface Org {
  id: string;
  slug: string;
  name: string;
  logo_key?: string;
  /** Set while the org is soft-deleted and in its restore grace window. */
  deleted_at?: string;
  created_at: string;
  updated_at: string;
}

/** Plan entitlements embedded in the billing summary (docs/06 §2). */
export interface Entitlements {
  plan: string;
  status: string;
  max_members: number;
  max_projects: number;
  max_storage_bytes: number;
  api_rate_per_min: number;
  audit_retention_days: number;
  metered: boolean;
}

/** Subscription status values (domain/billing/billing.go). */
export type SubStatus =
  | 'none'
  | 'trialing'
  | 'active'
  | 'past_due'
  | 'unpaid'
  | 'canceled';

/** Billing summary (GET /orgs/{id}/billing/summary — ADMIN+ only). */
export interface BillingSummary {
  plan: string;
  status: SubStatus;
  current_period_end?: string;
  cancel_at_period_end: boolean;
  past_due_warning: boolean;
  has_subscription: boolean;
  entitlements: Entitlements;
}

/** Project visibility (domain/project/project.go). */
export type Visibility = 'org' | 'private';

/** Task priority (domain/project/project.go). */
export type Priority = 'urgent' | 'high' | 'medium' | 'low' | 'none';

/** A project (GET /orgs/{id}/projects -> { projects: [...] }). */
export interface Project {
  id: string;
  key: string;
  name: string;
  description: string;
  color: string;
  visibility: Visibility;
  archived_at?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

/** Payload for POST /orgs/{id}/projects. */
export interface CreateProjectInput {
  key: string;
  name: string;
  description: string;
  color: string;
  visibility: Visibility;
}

/** Payload for PATCH /orgs/{id}/projects/{projectId}. */
export interface UpdateProjectInput {
  name: string;
  description: string;
  color: string;
  visibility: Visibility;
}

/** A board column (GET .../board). */
export interface Column {
  id: string;
  name: string;
  rank: string;
  wip_limit?: number;
}

/** A card in the board projection (columns[].tasks[]). */
export interface TaskCard {
  id: string;
  number: number;
  title: string;
  priority: Priority;
  assignee_id?: string;
  rank: string;
}

/** One column with its cards in the board projection. */
export interface BoardColumn extends Column {
  tasks: TaskCard[];
}

/** The kanban projection (GET .../projects/{projectId}/board). */
export interface Board {
  board_id: string;
  columns: BoardColumn[];
}

/** Payload for POST .../projects/{projectId}/tasks. */
export interface CreateTaskInput {
  column_id: string;
  title: string;
  description: string;
  assignee_id?: string | null;
  priority: Priority;
  due_date?: string | null;
}

/** Payload for PATCH /orgs/{id}/tasks/{taskId}. UpdateTask replaces the whole set. */
export interface UpdateTaskInput {
  title: string;
  description: string;
  assignee_id?: string | null;
  priority: Priority;
  due_date?: string | null;
}

/** A subtask (GET .../tasks/{taskId}/subtasks -> { subtasks: [...] }). */
export interface Subtask {
  id: string;
  title: string;
  done: boolean;
  rank: string;
}

/** A comment (GET .../tasks/{taskId}/comments -> { comments: [...] }). */
export interface Comment {
  id: string;
  author_id: string;
  /** Empty string when soft-deleted (server blanks the body). */
  body: string;
  edited: boolean;
  deleted_at?: string;
  created_at: string;
  updated_at: string;
}

/** A task activity-log row (GET .../tasks/{taskId}/activity -> { activity: [...] }). */
export interface Activity {
  id: string;
  actor_id: string;
  field: string;
  old_value?: string;
  new_value?: string;
  created_at: string;
}

/** Attachment upload status (domain/project). */
export type AttachmentStatus = 'pending' | 'ready';

/** An attachment (GET .../tasks/{taskId}/attachments -> { attachments: [...] }). */
export interface Attachment {
  id: string;
  task_id: string;
  uploader_id: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  status: string;
  created_at: string;
  confirmed_at?: string;
}

/** An org member row (GET /orgs/{id}/members -> { items: [...], next_cursor }). */
export interface OrgMember {
  user_id: string;
  email: string;
  name: string;
  avatar_key?: string;
  role: Role;
  created_at: string;
}

/** Bulk board action (POST /orgs/{id}/tasks/bulk). */
export type BulkAction = 'assign' | 'move' | 'label';

/** Payload for the bulk endpoint; fields beyond `action`/`task_ids` are per-action. */
export interface BulkActionInput {
  action: BulkAction;
  task_ids: string[];
  assignee_id?: string | null;
  column_id?: string;
  label_id?: string;
}

/** An org label (GET /orgs/{id}/labels -> { labels: [...] }). */
export interface Label {
  id: string;
  name: string;
  color: string;
}

/** A task row (GET /orgs/{id}/tasks/search -> { tasks: [...], total }). */
export interface Task {
  id: string;
  project_id: string;
  column_id: string;
  number: number;
  title: string;
  description: string;
  assignee_id?: string;
  priority: string;
  due_date?: string;
  rank: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

/** An in-app notification row (GET /orgs/{id}/notifications -> { notifications }). */
export interface Notification {
  id: string;
  category: string;
  title: string;
  body: string;
  entity_type?: string;
  entity_id?: string;
  read_at?: string;
  created_at: string;
}

/** The caller's account profile (GET /me). */
export interface Profile {
  id: string;
  email: string;
  name: string;
  avatar_url?: string;
  email_verified: boolean;
  platform_role: string;
  totp_enabled: boolean;
  created_at: string;
}

/** A stored notification preference row (per-org, GET /orgs/{id}/notifications/prefs). */
export interface NotificationPref {
  category: string;
  email: boolean;
  in_app: boolean;
}
