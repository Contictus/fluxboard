package tenant

// Authorization objects and actions — the coarse org-role gate mirrored by the
// Casbin policy (docs/05-TENANCY-RBAC.md §2 permission matrix). Objects are
// symbolic resource classes (not URL paths): the caller's role and org are
// already resolved by TenantResolver, so Casbin answers only "role → can do
// action on object". Fine-grained resource checks (project membership, comment
// authorship, OWNER-protection) stay in the usecase layer. See ADR-013.
const (
	ObjOrg         = "org"         // rename/logo (write), read
	ObjOwnership   = "ownership"   // slug change, delete, transfer (OWNER)
	ObjMembers     = "members"     // invite/remove/change role
	ObjInvitations = "invitations" // manage invitations
	ObjBilling     = "billing"     // view/manage billing
	ObjAPIKeys     = "apikeys"     // manage API keys
	ObjAudit       = "audit"       // view org audit log
	ObjProjects    = "projects"    // create projects
	ObjLabels      = "labels"      // manage labels
	ObjAutomations = "automations" // manage automation rules

	ActRead  = "read"
	ActWrite = "write"
)
