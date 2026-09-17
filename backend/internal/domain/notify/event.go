package notify

// Event is a realtime event published to an org's Redis Stream and fanned out
// over SSE (docs/09-REALTIME-JOBS.md §1). ID is the stream entry id (doubles as
// the SSE id / Last-Event-ID) and is assigned by the bus on publish — producers
// leave it empty. Data is the JSON payload object; the bus injects "actor_id"
// and the schema version "v":1 when encoding the SSE frame.
type Event struct {
	ID      string         // stream entry id; set by EventBus.Publish
	Name    string         // one of the Event* constants (the catalog)
	Data    map[string]any // payload keys per the frozen catalog
	ActorID string         // user whose action produced the event ("" = system)
}

// NewEvent builds an event with the given name, actor, and payload. The bus
// assigns ID on publish.
func NewEvent(name, actorID string, data map[string]any) Event {
	if data == nil {
		data = map[string]any{}
	}
	return Event{Name: name, Data: data, ActorID: actorID}
}

// Event names — the frozen catalog (docs/build/PHASE-5 §0.3, 09 §1). Broadcast
// events fan out to every org subscriber; targeted events carry a user_id the
// client filters on.
const (
	EventTaskCreated  = "task.created"
	EventTaskUpdated  = "task.updated"
	EventTaskMoved    = "task.moved"
	EventTaskDeleted  = "task.deleted"
	EventTaskRestored = "task.restored"

	EventCommentCreated = "comment.created"

	EventMemberJoined      = "member.joined"
	EventMemberLeft        = "member.left"
	EventMemberRoleChanged = "member.role_changed"
	EventMembershipRevoked = "membership.revoked" // targeted

	EventNotificationCreated  = "notification.created" // targeted by user_id
	EventBillingStatusChanged = "billing.status_changed"

	// EventResync is emitted by the server when a client's Last-Event-ID is
	// older than stream retention; the client invalidates caches and refetches.
	EventResync = "resync"
)
