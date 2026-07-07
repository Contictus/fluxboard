package redisx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
)

// EventBus is the Redis-Stream-backed notify.EventBus (docs/09-REALTIME-JOBS.md
// §1). Producers XADD to events:{orgId} (MAXLEN ~1000 ≈ 5 min of history); each
// API replica runs one consumer goroutine per org with active subscribers
// (XREAD BLOCK) that demuxes stream entries to in-process subscriber channels.
// A reconnecting SSE client Replays from its Last-Event-ID via XRANGE. This
// makes SSE horizontally scalable: the stream is the bus, so any replica can
// serve any subscriber.
type EventBus struct {
	rdb    *redis.Client
	logger *slog.Logger

	mu   sync.Mutex
	orgs map[string]*orgFanout // consumers keyed by orgID (present ⇒ ≥1 subscriber)
}

// NewEventBus builds an EventBus over the given client. A nil logger defaults to
// slog.Default.
func NewEventBus(rdb *redis.Client, logger *slog.Logger) *EventBus {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventBus{rdb: rdb, logger: logger, orgs: map[string]*orgFanout{}}
}

var _ notify.EventBus = (*EventBus)(nil)

const (
	streamMaxLen  = 1000             // ~5 min of history under normal load (09 §1)
	subBuffer     = 64               // per-subscriber channel buffer
	readBlock     = 25 * time.Second // XREAD BLOCK window (matches SSE heartbeat)
	streamKeyPref = "events:"
)

func streamKey(orgID string) string { return streamKeyPref + orgID }

// ---- Publish ---------------------------------------------------------------

// Publish appends ev to the org stream and returns the assigned entry id. The
// payload stored is ev.Data plus the injected actor_id and schema version v:1
// (the SSE wire invariants, 09 §1).
func (b *EventBus) Publish(ctx context.Context, orgID string, ev notify.Event) (string, error) {
	payload := make(map[string]any, len(ev.Data)+2)
	for k, v := range ev.Data {
		payload[k] = v
	}
	payload["actor_id"] = ev.ActorID
	if _, ok := payload["v"]; !ok {
		payload["v"] = 1
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	id, err := b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey(orgID),
		MaxLen: streamMaxLen,
		Approx: true, // MAXLEN ~1000
		Values: map[string]any{"name": ev.Name, "data": string(data)},
	}).Result()
	if err != nil {
		return "", err
	}
	return id, nil
}

// ---- Replay ----------------------------------------------------------------

// Replay returns events after lastID (exclusive). gap=true when lastID predates
// the oldest surviving entry (evicted by MAXLEN) — the caller emits resync
// instead of a partial backlog.
func (b *EventBus) Replay(ctx context.Context, orgID, lastID string) ([]notify.Event, bool, error) {
	key := streamKey(orgID)
	// Oldest surviving entry; empty stream ⇒ nothing to replay, no gap.
	oldest, err := b.rdb.XRangeN(ctx, key, "-", "+", 1).Result()
	if err != nil {
		return nil, false, err
	}
	if len(oldest) == 0 {
		return nil, false, nil
	}
	if idLess(lastID, oldest[0].ID) {
		// Everything the client is missing was already evicted.
		return nil, true, nil
	}
	// Exclusive lower bound: "(lastID".
	msgs, err := b.rdb.XRange(ctx, key, "("+lastID, "+").Result()
	if err != nil {
		return nil, false, err
	}
	out := make([]notify.Event, 0, len(msgs))
	for _, m := range msgs {
		if ev, ok := decodeEntry(m); ok {
			out = append(out, ev)
		}
	}
	return out, false, nil
}

// ---- Subscribe / fan-out ---------------------------------------------------

type orgFanout struct {
	mu   sync.Mutex
	subs map[chan notify.Event]struct{}
	stop context.CancelFunc
}

// Subscribe returns a channel of live events for the org plus a cancel func the
// caller MUST invoke when the SSE connection ends. The first subscriber for an
// org starts its consumer goroutine; the last one to cancel stops it.
func (b *EventBus) Subscribe(ctx context.Context, orgID string) (<-chan notify.Event, func(), error) {
	ch := make(chan notify.Event, subBuffer)

	b.mu.Lock()
	fo, ok := b.orgs[orgID]
	if !ok {
		cctx, cancel := context.WithCancel(context.Background())
		fo = &orgFanout{subs: map[chan notify.Event]struct{}{}, stop: cancel}
		b.orgs[orgID] = fo
		go b.consume(cctx, orgID, fo)
	}
	fo.mu.Lock()
	fo.subs[ch] = struct{}{}
	fo.mu.Unlock()
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			fo.mu.Lock()
			delete(fo.subs, ch)
			empty := len(fo.subs) == 0
			fo.mu.Unlock()
			if empty {
				fo.stop()
				delete(b.orgs, orgID)
			}
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel, nil
}

// consume runs one XREAD BLOCK loop per org, demuxing entries to subscribers.
// It exits when the fanout is stopped (last subscriber left). A read error
// backs off briefly to avoid a busy loop.
func (b *EventBus) consume(ctx context.Context, orgID string, fo *orgFanout) {
	key := streamKey(orgID)
	lastID := "$" // only entries published after subscription
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := b.rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{key, lastID},
			Block:   readBlock,
			Count:   128,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || ctx.Err() != nil {
				continue // block window elapsed with no new entries
			}
			b.logger.Warn("eventbus consume read failed", "org", orgID, "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		for _, stream := range res {
			for _, m := range stream.Messages {
				lastID = m.ID
				ev, ok := decodeEntry(m)
				if !ok {
					continue
				}
				b.dispatch(fo, ev)
			}
		}
	}
}

// dispatch delivers ev to every subscriber, non-blocking: a full buffer drops
// the event for that slow client, which recovers via reconnect + replay/resync.
func (b *EventBus) dispatch(fo *orgFanout, ev notify.Event) {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	for ch := range fo.subs {
		select {
		case ch <- ev:
		default:
			b.logger.Warn("eventbus subscriber buffer full; dropping event", "event", ev.Name)
		}
	}
}

// ---- helpers ---------------------------------------------------------------

// decodeEntry parses a stream entry into a notify.Event. ok=false on a
// malformed entry (skipped).
func decodeEntry(m redis.XMessage) (notify.Event, bool) {
	name, _ := m.Values["name"].(string)
	if name == "" {
		return notify.Event{}, false
	}
	ev := notify.Event{ID: m.ID, Name: name, Data: map[string]any{}}
	if raw, ok := m.Values["data"].(string); ok && raw != "" {
		_ = json.Unmarshal([]byte(raw), &ev.Data)
	}
	if a, ok := ev.Data["actor_id"].(string); ok {
		ev.ActorID = a
	}
	return ev, true
}

// idLess reports whether stream id a is numerically older than b ("ms-seq"
// compared as two integers). An empty a is treated as oldest.
func idLess(a, b string) bool {
	if a == "" {
		return true
	}
	ams, aseq := splitID(a)
	bms, bseq := splitID(b)
	if ams != bms {
		return ams < bms
	}
	return aseq < bseq
}

func splitID(id string) (ms, seq uint64) {
	parts := strings.SplitN(id, "-", 2)
	ms, _ = strconv.ParseUint(parts[0], 10, 64)
	if len(parts) == 2 {
		seq, _ = strconv.ParseUint(parts[1], 10, 64)
	}
	return ms, seq
}
