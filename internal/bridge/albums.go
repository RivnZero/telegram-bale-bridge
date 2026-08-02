package bridge

import (
	"sort"
	"sync"
	"time"
)

type AlbumBuffer struct {
	mu     sync.Mutex
	groups map[AlbumKey]*albumGroup
	delay  time.Duration
	flush  func(*AlbumUnit)
}
type albumGroup struct {
	items []*BridgeMessage
	timer *time.Timer
}

func NewAlbumBuffer(delay time.Duration, flush func(*AlbumUnit)) *AlbumBuffer {
	return &AlbumBuffer{groups: make(map[AlbumKey]*albumGroup), delay: delay, flush: flush}
}
func (b *AlbumBuffer) Add(m *BridgeMessage) {
	key := AlbumKey{Platform: m.SourcePlatform, ChatID: m.SourceChatID, MediaGroupID: m.MediaGroupID}
	b.mu.Lock()
	g := b.groups[key]
	if g == nil {
		g = &albumGroup{}
		b.groups[key] = g
	}
	g.items = append(g.items, m)
	if g.timer != nil {
		g.timer.Stop()
	}
	g.timer = time.AfterFunc(b.delay, func() { b.fire(key) })
	b.mu.Unlock()
}
func (b *AlbumBuffer) fire(key AlbumKey) {
	b.mu.Lock()
	g := b.groups[key]
	if g == nil {
		b.mu.Unlock()
		return
	}
	delete(b.groups, key)
	if g.timer != nil {
		g.timer.Stop()
	}
	items := g.items
	b.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].SourceMessageID < items[j].SourceMessageID })
	b.flush(&AlbumUnit{Key: key, Items: items})
}
func (b *AlbumBuffer) FlushAll() {
	b.mu.Lock()
	keys := make([]AlbumKey, 0, len(b.groups))
	for k := range b.groups {
		keys = append(keys, k)
	}
	b.mu.Unlock()
	for _, k := range keys {
		b.fire(k)
	}
}
