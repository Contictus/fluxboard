package notifyuc

import (
	"context"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
)

// StreamInit computes the initial SSE backlog for a (re)connecting client
// (FR-NTF-001, 09 §1). With an empty lastEventID it is a fresh connection: no
// backlog. With a lastEventID it replays events after that id; if the id is
// older than stream retention the return is resync=true and no backlog, so the
// handler emits a single `resync` frame and the client refetches. A nil bus
// (SSE disabled) yields an empty, no-resync result.
func (s *Service) StreamInit(ctx context.Context, orgID, lastEventID string) (backlog []notify.Event, resync bool, err error) {
	if s.bus == nil || lastEventID == "" {
		return nil, false, nil
	}
	events, gap, err := s.bus.Replay(ctx, orgID, lastEventID)
	if err != nil {
		return nil, false, err
	}
	if gap {
		return nil, true, nil
	}
	return events, false, nil
}

// Subscribe opens the live event channel for an org (FR-NTF-001). The caller
// MUST invoke cancel when the SSE connection ends to release the subscription.
// A nil bus yields a closed channel and a no-op cancel.
func (s *Service) Subscribe(ctx context.Context, orgID string) (<-chan notify.Event, func(), error) {
	if s.bus == nil {
		ch := make(chan notify.Event)
		close(ch)
		return ch, func() {}, nil
	}
	return s.bus.Subscribe(ctx, orgID)
}
