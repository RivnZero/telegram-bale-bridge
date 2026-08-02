package bale

import (
	"context"
	"log/slog"
	"time"
)

type Poller struct {
	client *Client
	logger *slog.Logger
	handle func(context.Context, *Message)
}

func NewPoller(client *Client, logger *slog.Logger, handle func(context.Context, *Message)) *Poller {
	return &Poller{client: client, logger: logger, handle: handle}
}
func (p *Poller) Run(ctx context.Context) {
	var offset int64
	for {
		updates, err := p.client.GetUpdates(ctx, offset, MaxUpdatesPerPoll, LongPollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.logger.Error("getUpdates failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message != nil {
				p.handle(ctx, u.Message)
			}
		}
	}
}
