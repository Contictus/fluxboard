// Package reqmeta carries request-scoped client metadata (IP, user-agent) on the
// context so deep usecase code can enrich audit entries without threading the
// values through every signature. Set once by the ClientMeta HTTP middleware;
// read wherever an audit.Entry is built. Stdlib-only leaf package.
package reqmeta

import "context"

type ctxKey int

const ctxKeyMeta ctxKey = 0

// Meta is the request-scoped client metadata. ActorUserID is filled once the
// caller is authenticated; IP/UserAgent are set for every request.
type Meta struct {
	IP          string
	UserAgent   string
	ActorUserID string
}

// WithMeta stores m on ctx.
func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, ctxKeyMeta, m)
}

// WithActor returns a context whose Meta carries actorUserID, preserving any
// IP/UserAgent already set by the ClientMeta middleware.
func WithActor(ctx context.Context, actorUserID string) context.Context {
	m := From(ctx)
	m.ActorUserID = actorUserID
	return WithMeta(ctx, m)
}

// From returns the Meta stored on ctx, or a zero Meta if none is present.
func From(ctx context.Context) Meta {
	m, _ := ctx.Value(ctxKeyMeta).(Meta)
	return m
}
