package bridge

import (
	"sync"
	"time"
)

type SentTracker struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[SentKey]time.Time
}

func NewSentTracker(ttl time.Duration) *SentTracker {
	return &SentTracker{ttl: ttl, seen: make(map[SentKey]time.Time)}
}
func (t *SentTracker) Mark(platform Platform, chatID, messageID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seen[SentKey{Platform: platform, ChatID: chatID, MessageID: messageID}] = time.Now()
}
func (t *SentTracker) Seen(platform Platform, chatID, messageID int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for k, at := range t.seen {
		if now.Sub(at) > t.ttl {
			delete(t.seen, k)
		}
	}
	_, ok := t.seen[SentKey{Platform: platform, ChatID: chatID, MessageID: messageID}]
	return ok
}
