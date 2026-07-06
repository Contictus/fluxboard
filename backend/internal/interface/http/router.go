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
	Orgs          *handlers.OrgHandlers
	Projects      *handlers.ProjectHandlers
	Tasks         *handlers.TaskHandlers
	Authenticator *mw.Authenticator
	Tenant        *mw.TenantGuard
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

	// Operational endpoints (outside auth).
	r.Get("/healthz", d.Health.Live)
	r.Get("/readyz", d.Health.Ready)
	r.Handle("/metrics", d.MetricsHTTP)

	r.Route("/api/v1", func(api chi.Router) {
		// Auth surface (docs/04-AUTH.md §5) — no org context.
		api.Route("/auth", func(a chi.Router) {
			// Public (no access token).
			a.Post("/register", d.Auth.Register)
			a.Post("/login", d.Auth.Login)
			a.Post("/refresh", d.Auth.Refresh)
			a.Post("/2fa/verify", d.Auth.Verify2FA)
			a.Post("/verify-email/request", d.Auth.VerifyEmailRequest)
			a.Post("/verify-email/confirm", d.Auth.VerifyEmailConfirm)
			a.Post("/password/forgot", d.Auth.PasswordForgot)
			a.Post("/password/reset", d.Auth.PasswordReset)
			a.Get("/oauth/google/start", d.Auth.OAuthGoogleStart)
			a.Get("/oauth/google/callback", d.Auth.OAuthGoogleCallback)

			// Authenticated auth endpoints.
			a.Group(func(pr chi.Router) {
				pr.Use(d.Authenticator.Authenticate)
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
			sec.Use(d.Authenticator.Authenticate) // 5
			sec.Use(mw.RequireVerified)           // FR-AUTH-002: block unverified email

			// Org root (no {orgId} — cannot resolve a tenant).
			sec.Post("/orgs", d.Orgs.CreateOrg)
			sec.Get("/orgs", d.Orgs.ListMyOrgs)
			// Static "by-slug" is matched ahead of the {orgId} subrouter; org ids
			// are UUIDs so there is no collision (FR-TEN-007 slug 301 resolver).
			sec.Get("/orgs/by-slug/{slug}", d.Orgs.ResolveSlug)
			sec.Post("/invitations/accept", d.Orgs.AcceptInvitation)

			// Org-scoped surface (docs/08 §4).
			sec.Route("/orgs/{orgId}", func(o chi.Router) {
				o.Use(d.Tenant.Resolve) // 6 (+ RLS scope inside usecases)

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
				o.With(write(tenant.ObjInvitations)).Post("/invitations", d.Orgs.CreateInvitation)
				o.With(write(tenant.ObjInvitations)).Delete("/invitations/{id}", d.Orgs.RevokeInvitation)
				o.With(write(tenant.ObjInvitations)).Post("/invitations/{id}/resend", d.Orgs.ResendInvitation)

				// Phase 3a — projects & boards (docs/01 §PROJ). Create needs the
				// org write:projects gate (MEMBER+); everything else runs under
				// read:org and defers to the project-role gate in projectuc.
				o.With(read(tenant.ObjOrg)).Get("/projects", d.Projects.ListProjects)
				o.With(write(tenant.ObjProjects)).Post("/projects", d.Projects.CreateProject)
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
				})

				// Org-wide task search + bulk actions (FR-TASK-007/008). Static
				// segments; chi matches them ahead of the {taskId} subrouter.
				o.With(read(tenant.ObjOrg)).Get("/tasks/search", d.Tasks.Search)
				o.With(read(tenant.ObjOrg)).Post("/tasks/bulk", d.Tasks.BulkAction)

				// Tasks by id (flat; access derives the project). docs/01 §TASK.
				o.Route("/tasks/{taskId}", func(t chi.Router) {
					t.With(read(tenant.ObjOrg)).Get("/", d.Tasks.GetTask)
					t.With(read(tenant.ObjOrg)).Patch("/", d.Tasks.UpdateTask)
					t.With(read(tenant.ObjOrg)).Delete("/", d.Tasks.TrashTask)          // FR-TASK-009
					t.With(read(tenant.ObjOrg)).Post("/restore", d.Tasks.RestoreTask)   // FR-TASK-009
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
				})

				// Org-scoped labels (docs/01 §TASK FR-TASK-004). Writes need the
				// write:labels gate (MEMBER+); reads under read:org.
				o.With(read(tenant.ObjOrg)).Get("/labels", d.Tasks.ListLabels)
				o.With(write(tenant.ObjLabels)).Post("/labels", d.Tasks.CreateLabel)
				o.With(write(tenant.ObjLabels)).Patch("/labels/{labelId}", d.Tasks.UpdateLabel)
				o.With(write(tenant.ObjLabels)).Delete("/labels/{labelId}", d.Tasks.DeleteLabel)
			})
		})
	})

	return r
}
