// Package httpx builds the API's HTTP surface: the chi router, the
// contract-ordered middleware chain, health probes, and route mounting.
package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/handlers"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
)

// Deps are the collaborators the router needs. main.go owns construction.
type Deps struct {
	Logger        *slog.Logger
	WebOrigin     string
	Health        Health
	MetricsHTTP   http.Handler
	Auth          *handlers.AuthHandlers
	User          *handlers.UserHandlers // nil ⇒ /me account surface disabled
	Orgs          *handlers.OrgHandlers
	Projects      *handlers.ProjectHandlers
	Tasks         *handlers.TaskHandlers
	Automations   *handlers.AutomationHandlers // nil ⇒ automations disabled
	Billing       *handlers.BillingHandlers
	Webhooks      *handlers.WebhookHandlers
	Events        *handlers.EventHandlers        // nil ⇒ SSE disabled (tests)
	Notifications *handlers.NotificationHandlers // nil ⇒ notification center disabled (tests)
	APIKeys       *handlers.APIKeyHandlers       // nil ⇒ API-key management disabled
	AuditView     *handlers.AuditHandlers        // nil ⇒ org audit viewer disabled
	Analytics     *handlers.AnalyticsHandlers    // nil ⇒ analytics disabled
	Admin         *handlers.AdminHandlers        // nil ⇒ /admin surface disabled
	OpenAPI       *handlers.OpenAPIHandlers      // nil ⇒ openapi.json / docs disabled
	DevDocs       bool                           // true ⇒ mount interactive Swagger UI at /api/docs (non-prod)
	Authenticator *mw.Authenticator
	Tenant        *mw.TenantGuard
	Entitlement   *mw.EntitlementGuard
	AuthThrottle  *mw.AuthThrottle         // nil ⇒ public auth endpoints unthrottled (tests)
	RateLimit     *mw.RateLimiter          // nil ⇒ no plan rate limiting (tests)
	PlatformAdmin *mw.PlatformAdminGuard   // nil ⇒ /admin surface disabled
	APIKeyResolver mw.APIKeyResolver       // nil ⇒ API-key auth path disabled (session only)
	HTTPMetrics   mw.HTTPMetrics           // nil ⇒ no request-duration histogram (tests)
}

// NewRouter assembles the router. Infrastructure middleware wrap every route;
// the security chain (auth → tenant → rbac → …) wraps only the routes that need
// it, in the order fixed by docs/03-ARCHITECTURE.md §2.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Infrastructure chain (1–4), applied to all routes.
	r.Use(mw.RequestID)
	r.Use(mw.Logger(d.Logger))
	r.Use(mw.Recoverer)
	r.Use(mw.CORS(d.WebOrigin))
	r.Use(mw.ClientMeta) // stash client IP/UA for audit enrichment (pkg/reqmeta)
	if d.HTTPMetrics != nil {
		r.Use(mw.Metrics(d.HTTPMetrics)) // request-duration histogram (10 §5)
	}

	// Operational endpoints (outside auth).
	r.Get("/healthz", d.Health.Live)
	r.Get("/readyz", d.Health.Ready)
	r.Handle("/metrics", d.MetricsHTTP)

	// Interactive API docs (Swagger UI) — non-prod only (FR-API-003).
	if d.OpenAPI != nil && d.DevDocs {
		r.Get("/api/docs", d.OpenAPI.Docs)
	}

	r.Route("/api/v1", func(api chi.Router) {
		// Stripe webhook (FR-BILL-005). Mounted OUTSIDE session auth — Stripe
		// carries no bearer token; the handler authenticates the request by
		// verifying the payload signature through the gateway. docs/06 §4.
		api.Post("/webhooks/stripe", d.Webhooks.Stripe)

		// OpenAPI 3.1 spec (FR-API-003) — public, outside auth so client codegen
		// and Swagger UI can fetch it without a token.
		if d.OpenAPI != nil {
			api.Get("/openapi.json", d.OpenAPI.Spec)
		}

		// Auth surface (docs/04-AUTH.md §5) — no org context.
		api.Route("/auth", func(a chi.Router) {
			// Per-IP throttle for the unauthenticated abuse-prone endpoints. Login
			// is throttled by the usecase on (email, IP); these have no trusted
			// email to key on. No-op passthrough when the throttle is unwired.
			throttle := func(tag string) func(http.Handler) http.Handler {
				if d.AuthThrottle == nil {
					return func(next http.Handler) http.Handler { return next }
				}
				return d.AuthThrottle.PerIP(tag)
			}

			// Public (no access token).
			a.With(throttle("register")).Post("/register", d.Auth.Register)
			a.Post("/login", d.Auth.Login)
			a.Post("/refresh", d.Auth.Refresh)
			a.Post("/2fa/verify", d.Auth.Verify2FA)
			a.With(throttle("verify_email")).Post("/verify-email/request", d.Auth.VerifyEmailRequest)
			a.Post("/verify-email/confirm", d.Auth.VerifyEmailConfirm)
			a.With(throttle("password_forgot")).Post("/password/forgot", d.Auth.PasswordForgot)
			a.With(throttle("password_reset")).Post("/password/reset", d.Auth.PasswordReset)
			a.Get("/oauth/google/start", d.Auth.OAuthGoogleStart)
			a.Get("/oauth/google/callback", d.Auth.OAuthGoogleCallback)

			// Authenticated auth endpoints.
			a.Group(func(pr chi.Router) {
				pr.Use(d.Authenticator.Authenticate)
				pr.Use(mw.RejectImpersonationWrite) // FR-ADM-003: no self-account mutation under an impersonation token
				pr.Post("/logout", d.Auth.Logout)
				pr.Post("/logout-all", d.Auth.LogoutAll)
				pr.Post("/password/change", d.Auth.PasswordChange)
				pr.Get("/sessions", d.Auth.Sessions)
				pr.Delete("/sessions/{id}", d.Auth.RevokeSession)
				pr.Post("/2fa/enroll", d.Auth.Enroll2FA)
				pr.Post("/2fa/activate", d.Auth.Activate2FA)
				pr.Delete("/2fa", d.Auth.Disable2FA)
			})
		})

		// Secured resource surface (Phase 2+). Org-root routes run under auth
		// only (no tenant context yet); org-scoped routes add TenantGuard
		// (resolve membership + RLS scope) and a per-route Casbin gate, in the
		// order fixed by docs/03-ARCHITECTURE.md §2.
		api.Group(func(sec chi.Router) {
			// 5: authenticate. When the API-key path is wired, a Bearer fbk_… key
			// resolves its org here instead of a session (FR-API-002); otherwise the
			// session authenticator runs alone.
			if d.APIKeyResolver != nil {
				sec.Use(mw.HybridAuth(d.Authenticator, d.APIKeyResolver))
			} else {
				sec.Use(d.Authenticator.Authenticate)
			}
			sec.Use(mw.RequireVerified)           // FR-AUTH-002: block unverified email (API keys are marked verified)
			sec.Use(mw.RejectImpersonationWrite) // FR-ADM-003: an impersonation token is read-only everywhere, incl. /me and POST /orgs

			// Account self-service (docs/08 §3, FR-AUTH-014). User-scoped, org-independent.
			if d.User != nil {
				sec.Get("/me", d.User.Me)
				sec.Patch("/me", d.User.UpdateMe)
				sec.Delete("/me", d.User.DeleteMe)
				sec.Post("/me/avatar/upload-url", d.User.AvatarUploadURL)
				sec.Post("/me/avatar/confirm", d.User.AvatarConfirm)
			}

			// Org root (no {orgId} — cannot resolve a tenant).
			sec.Post("/orgs", d.Orgs.CreateOrg)
			sec.Get("/orgs", d.Orgs.ListMyOrgs)
			// Static "by-slug" is matched ahead of the {orgId} subrouter; org ids
			// are UUIDs so there is no collision (FR-TEN-007 slug 301 resolver).
			sec.Get("/orgs/by-slug/{slug}", d.Orgs.ResolveSlug)
			sec.Post("/invitations/accept", d.Orgs.AcceptInvitation)

			// Org-scoped surface (docs/08 §4).
			sec.Route("/orgs/{orgId}", func(o chi.Router) {
				o.Use(d.Tenant.Resolve)         // 6 (+ RLS scope inside usecases)
				o.Use(mw.ImpersonationReadOnly) // FR-ADM-003: block writes under an impersonation token
				o.Use(mw.APIKeyScopeGuard)      // FR-API-002: block writes from read-scoped API keys
				if d.RateLimit != nil {
					o.Use(d.RateLimit.Limit) // 7: plan api_rate_per_min + usage counters (06 §5/§7)
				}

				read := func(obj string) func(http.Handler) http.Handler {
					return d.Tenant.Require(obj, tenant.ActRead)
				}
				write := func(obj string) func(http.Handler) http.Handler {
					return d.Tenant.Require(obj, tenant.ActWrite)
				}

				o.With(read(tenant.ObjOrg)).Get("/", d.Orgs.GetOrg)
				o.With(write(tenant.ObjOrg)).Patch("/", d.Orgs.UpdateOrg)
				o.With(write(tenant.ObjOwnership)).Delete("/", d.Orgs.DeleteOrg)
				o.With(write(tenant.ObjOwnership)).Post("/restore", d.Orgs.RestoreOrg)
				o.With(write(tenant.ObjOwnership)).Post("/transfer-ownership", d.Orgs.TransferOwnership)

				o.With(read(tenant.ObjOrg)).Get("/members", d.Orgs.ListMembers)
				o.With(read(tenant.ObjOrg)).Delete("/members/me", d.Orgs.Leave)
				o.With(write(tenant.ObjMembers)).Patch("/members/{userId}", d.Orgs.ChangeMemberRole)
				o.With(write(tenant.ObjMembers)).Delete("/members/{userId}", d.Orgs.RemoveMember)

				o.With(write(tenant.ObjInvitations)).Get("/invitations", d.Orgs.ListInvitations)
				o.With(write(tenant.ObjInvitations), d.Entitlement.RequireMembers()).Post("/invitations", d.Orgs.CreateInvitation)
				o.With(write(tenant.ObjInvitations)).Delete("/invitations/{id}", d.Orgs.RevokeInvitation)
				o.With(write(tenant.ObjInvitations)).Post("/invitations/{id}/resend", d.Orgs.ResendInvitation)

				// Phase 3a — projects & boards (docs/01 §PROJ). Create needs the
				// org write:projects gate (MEMBER+); everything else runs under
				// read:org and defers to the project-role gate in projectuc.
				o.With(read(tenant.ObjOrg)).Get("/projects", d.Projects.ListProjects)
				o.With(write(tenant.ObjProjects), d.Entitlement.RequireProjects()).Post("/projects", d.Projects.CreateProject)
				o.Route("/projects/{projectId}", func(p chi.Router) {
					p.With(read(tenant.ObjOrg)).Get("/", d.Projects.GetProject)
					p.With(read(tenant.ObjOrg)).Patch("/", d.Projects.UpdateProject)
					p.With(read(tenant.ObjOrg)).Post("/archive", d.Projects.ArchiveProject)
					p.With(read(tenant.ObjOrg)).Post("/unarchive", d.Projects.UnarchiveProject)
					p.With(read(tenant.ObjOrg)).Get("/members", d.Projects.ListMembers)
					p.With(read(tenant.ObjOrg)).Post("/members", d.Projects.AddMember)
					p.With(read(tenant.ObjOrg)).Delete("/members/{userId}", d.Projects.RemoveMember)
					p.With(read(tenant.ObjOrg)).Get("/board", d.Projects.GetBoard)
					p.With(read(tenant.ObjOrg)).Post("/columns", d.Projects.AddColumn)
					p.With(read(tenant.ObjOrg)).Patch("/columns/{columnId}", d.Projects.RenameColumn)
					p.With(read(tenant.ObjOrg)).Patch("/columns/{columnId}/position", d.Projects.ReorderColumn)
					p.With(read(tenant.ObjOrg)).Delete("/columns/{columnId}", d.Projects.DeleteColumn)
				p.With(read(tenant.ObjOrg)).Post("/tasks", d.Tasks.CreateTask)
				p.With(read(tenant.ObjOrg)).Get("/tasks", d.Tasks.ListTasks)
				p.With(read(tenant.ObjOrg)).Get("/trash", d.Tasks.ListTrash) // FR-TASK-009
				// Sprints (docs/08, FR-SPRINT). Fine-grained gates in projectuc.
				p.With(read(tenant.ObjOrg)).Get("/sprints", d.Projects.ListSprints)
				p.With(read(tenant.ObjOrg)).Post("/sprints", d.Projects.CreateSprint)
				p.With(read(tenant.ObjOrg)).Post("/sprints/{sprintId}/start", d.Projects.StartSprint)
				p.With(read(tenant.ObjOrg)).Post("/sprints/{sprintId}/complete", d.Projects.CompleteSprint)
				p.With(read(tenant.ObjOrg)).Delete("/sprints/{sprintId}", d.Projects.DeleteSprint)
				p.With(read(tenant.ObjOrg)).Post("/sprints/assign", d.Projects.AssignSprint)
				// Custom fields (docs/08, FR-FIELDS). Fine-grained gates in projectuc.
				p.With(read(tenant.ObjOrg)).Get("/fields", d.Projects.ListFields)
				p.With(read(tenant.ObjOrg)).Post("/fields", d.Projects.CreateField)
				p.With(read(tenant.ObjOrg)).Patch("/fields/{fieldId}", d.Projects.UpdateField)
				p.With(read(tenant.ObjOrg)).Delete("/fields/{fieldId}", d.Projects.DeleteField)
			})

				// Org-wide task search + bulk actions (FR-TASK-007/008). Static
				// segments; chi matches them ahead of the {taskId} subrouter.
				o.With(read(tenant.ObjOrg)).Get("/tasks/search", d.Tasks.Search)
				o.With(read(tenant.ObjOrg)).Post("/tasks/bulk", d.Tasks.BulkAction)

				// Tasks by id (flat; access derives the project). docs/01 §TASK.
				o.Route("/tasks/{taskId}", func(t chi.Router) {
					t.With(read(tenant.ObjOrg)).Get("/", d.Tasks.GetTask)
					t.With(read(tenant.ObjOrg)).Patch("/", d.Tasks.UpdateTask)
					t.With(read(tenant.ObjOrg)).Delete("/", d.Tasks.TrashTask)        // FR-TASK-009
					t.With(read(tenant.ObjOrg)).Post("/restore", d.Tasks.RestoreTask) // FR-TASK-009
					t.With(read(tenant.ObjOrg)).Patch("/position", d.Projects.MoveTask)
					t.With(read(tenant.ObjOrg)).Get("/activity", d.Tasks.ListActivity)
					// Attachments (FR-TASK-006). Presigned PUT/GET; API proxies no bytes.
					t.With(read(tenant.ObjOrg)).Get("/attachments", d.Tasks.ListAttachments)
					t.With(read(tenant.ObjOrg)).Post("/attachments", d.Tasks.RequestUpload)
					t.With(read(tenant.ObjOrg)).Post("/attachments/{attachmentId}/confirm", d.Tasks.ConfirmUpload)
					t.With(read(tenant.ObjOrg)).Get("/attachments/{attachmentId}/download", d.Tasks.DownloadAttachment)
					t.With(read(tenant.ObjOrg)).Delete("/attachments/{attachmentId}", d.Tasks.DeleteAttachment)
					t.With(read(tenant.ObjOrg)).Get("/subtasks", d.Tasks.ListSubtasks)
					t.With(read(tenant.ObjOrg)).Post("/subtasks", d.Tasks.AddSubtask)
					t.With(read(tenant.ObjOrg)).Patch("/subtasks/{subtaskId}", d.Tasks.UpdateSubtask)
					t.With(read(tenant.ObjOrg)).Delete("/subtasks/{subtaskId}", d.Tasks.DeleteSubtask)
					t.With(read(tenant.ObjOrg)).Get("/comments", d.Tasks.ListComments)
					t.With(read(tenant.ObjOrg)).Post("/comments", d.Tasks.AddComment)
				t.With(read(tenant.ObjOrg)).Patch("/comments/{commentId}", d.Tasks.EditComment)
				t.With(read(tenant.ObjOrg)).Delete("/comments/{commentId}", d.Tasks.DeleteComment)
				t.With(read(tenant.ObjOrg)).Get("/labels", d.Tasks.ListTaskLabels)
				t.With(read(tenant.ObjOrg)).Post("/labels", d.Tasks.AttachLabel)
				t.With(read(tenant.ObjOrg)).Delete("/labels/{labelId}", d.Tasks.DetachLabel)
				// Task custom values (docs/08, FR-FIELDS).
				t.With(read(tenant.ObjOrg)).Get("/fields", d.Projects.TaskFieldValues)
				t.With(read(tenant.ObjOrg)).Put("/fields/{fieldId}", d.Projects.SetFieldValue)
				t.With(read(tenant.ObjOrg)).Delete("/fields/{fieldId}", d.Projects.ClearFieldValue)
				// Dependencies (docs/08, FR-LINKS). Fine-grained gates in taskuc.
				t.With(read(tenant.ObjOrg)).Get("/links", d.Tasks.ListTaskLinks)
				t.With(read(tenant.ObjOrg)).Post("/links", d.Tasks.AddTaskLink)
				t.With(read(tenant.ObjOrg)).Delete("/links/{linkedId}", d.Tasks.RemoveTaskLink)
				// Time tracking (docs/08, FR-TIME). Fine-grained gates in taskuc.
				t.With(read(tenant.ObjOrg)).Get("/time", d.Tasks.ListTimeEntries)
				t.With(read(tenant.ObjOrg)).Post("/time/start", d.Tasks.StartTimer)
				t.With(read(tenant.ObjOrg)).Post("/time", d.Tasks.LogTime)
				t.With(read(tenant.ObjOrg)).Post("/time/{entryId}/stop", d.Tasks.StopTimer)
				t.With(read(tenant.ObjOrg)).Delete("/time/{entryId}", d.Tasks.DeleteTimeEntry)
			})

				// Org-scoped labels (docs/01 §TASK FR-TASK-004). Writes need the
				// write:labels gate (MEMBER+); reads under read:org.
				o.With(read(tenant.ObjOrg)).Get("/labels", d.Tasks.ListLabels)
				o.With(write(tenant.ObjLabels)).Post("/labels", d.Tasks.CreateLabel)
				o.With(write(tenant.ObjLabels)).Patch("/labels/{labelId}", d.Tasks.UpdateLabel)
				o.With(write(tenant.ObjLabels)).Delete("/labels/{labelId}", d.Tasks.DeleteLabel)

				// Automation rules (docs/08, FR-AUTO). Reads under read:org;
				// writes need the ADMIN automations gate.
				if d.Automations != nil {
					o.With(read(tenant.ObjOrg)).Get("/automations", d.Automations.ListRules)
					o.With(write(tenant.ObjAutomations)).Post("/automations", d.Automations.CreateRule)
					o.With(write(tenant.ObjAutomations)).Patch("/automations/{ruleId}", d.Automations.UpdateRule)
					o.With(write(tenant.ObjAutomations)).Post("/automations/{ruleId}/enabled", d.Automations.SetRuleEnabled)
					o.With(write(tenant.ObjAutomations)).Delete("/automations/{ruleId}", d.Automations.DeleteRule)
				}

				// Phase 4 — billing (docs/06). The billing object gate resolves to
				// ADMIN+ (docs/05 §2); reads and writes are both admin-only, so a
				// MEMBER/GUEST gets 403. Card data never transits the API — Checkout
				// and Portal hand the browser to Stripe-hosted pages.
				o.With(read(tenant.ObjBilling)).Get("/billing/summary", d.Billing.Summary)
				o.With(read(tenant.ObjBilling)).Get("/billing/invoices", d.Billing.ListInvoices)
				o.With(write(tenant.ObjBilling)).Post("/billing/checkout", d.Billing.Checkout)
				o.With(write(tenant.ObjBilling)).Post("/billing/preview-change", d.Billing.PreviewChange)
				o.With(write(tenant.ObjBilling)).Post("/billing/change", d.Billing.ApplyChange)
				o.With(write(tenant.ObjBilling)).Post("/billing/cancel", d.Billing.Cancel)
				o.With(write(tenant.ObjBilling)).Post("/billing/resume", d.Billing.Resume)
				o.With(write(tenant.ObjBilling)).Post("/billing/portal", d.Billing.Portal)

				// Phase 5 — realtime SSE + notification center (docs/09 §1/§3,
				// FR-NTF-001/002/004). All under read:org; the notification
				// endpoints scope to the caller's own rows (userID from the
				// tenant context). Registered only when wired (nil in tests).
				if d.Events != nil {
					o.With(read(tenant.ObjOrg)).Get("/events", d.Events.Stream)
				}
				if d.Notifications != nil {
					o.With(read(tenant.ObjOrg)).Get("/notifications", d.Notifications.List)
					o.With(read(tenant.ObjOrg)).Get("/notifications/unread-count", d.Notifications.UnreadCount)
					o.With(read(tenant.ObjOrg)).Post("/notifications/read-all", d.Notifications.MarkAllRead)
					o.With(read(tenant.ObjOrg)).Post("/notifications/{id}/read", d.Notifications.MarkRead)
					o.With(read(tenant.ObjOrg)).Get("/notifications/prefs", d.Notifications.GetPrefs)
					o.With(read(tenant.ObjOrg)).Put("/notifications/prefs", d.Notifications.SetPref)
				}

				// Phase 6 — org settings API keys (ADMIN, FR-API-001), the org audit
				// viewer + CSV (ADMIN, FR-AUD-003), and analytics (project = any
				// member, usage = ADMIN; FR-AN-001/002).
				if d.APIKeys != nil {
					o.With(write(tenant.ObjAPIKeys)).Get("/api-keys", d.APIKeys.List)
					o.With(write(tenant.ObjAPIKeys)).Post("/api-keys", d.APIKeys.Create)
					o.With(write(tenant.ObjAPIKeys)).Delete("/api-keys/{id}", d.APIKeys.Revoke)
				}
				if d.AuditView != nil {
					o.With(read(tenant.ObjAudit)).Get("/audit", d.AuditView.List)
					o.With(read(tenant.ObjAudit)).Get("/audit.csv", d.AuditView.ExportCSV)
				}
				if d.Analytics != nil {
					o.With(read(tenant.ObjOrg)).Get("/projects/{projectId}/analytics", d.Analytics.ProjectAnalytics)
					o.With(read(tenant.ObjBilling)).Get("/usage", d.Analytics.Usage)
				}
			})
		})
	})

	// Platform-admin surface (docs/build/PHASE-6 §5). A SEPARATE router, deliberately
	// NOT under the tenant middleware: the platform-admin guard (platform_role=admin
	// + 2FA) is the only gate, and adminuc reads cross-tenant on the owner pool.
	if d.Admin != nil && d.PlatformAdmin != nil {
		r.Route("/admin", func(a chi.Router) {
			a.Use(d.Authenticator.Authenticate)
			a.Use(d.PlatformAdmin.RequireAdmin)

			a.Get("/tenants", d.Admin.ListTenants)
			a.Get("/tenants/{orgId}", d.Admin.GetTenant)
			a.Post("/tenants/{orgId}/impersonate", d.Admin.Impersonate)
			a.Post("/tenants/{orgId}/webhooks/{eventId}/retry", d.Admin.RetryWebhook)
			a.Put("/tenants/{orgId}/flags", d.Admin.SetFlag)
			a.Put("/tenants/{orgId}/overrides", d.Admin.SetOverride)
			a.Delete("/tenants/{orgId}/overrides/{key}", d.Admin.DeleteOverride)
			a.Get("/audit", d.Admin.GlobalAudit)
			a.Get("/jobs", d.Admin.Jobs)
		})
	}

	return r
}
