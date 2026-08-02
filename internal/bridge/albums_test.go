package bridge

import (
	"testing"
	"time"
)

func albumMsg(id int64) *BridgeMessage {
	return &BridgeMessage{
		SourcePlatform:  PlatformTelegram,
		SourceChatID:    1,
		SourceMessageID: id,
		MediaGroupID:    "group-1",
		Kind:            KindPhoto,
	}
}

func waitFlush(t *testing.T, ch chan *AlbumUnit) *AlbumUnit {
	t.Helper()
	select {
	case u := <-ch:
		return u
	case <-time.After(2 * time.Second):
		t.Fatal("album not flushed in time")
		return nil
	}
}

func TestAlbumBufferSortsByMessageID(t *testing.T) {
	flushed := make(chan *AlbumUnit, 1)
	b := NewAlbumBuffer(40*time.Millisecond, func(u *AlbumUnit) { flushed <- u })
	b.Add(albumMsg(5))
	b.Add(albumMsg(3))
	u := waitFlush(t, flushed)
	if len(u.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(u.Items))
	}
	if u.Items[0].SourceMessageID != 3 || u.Items[1].SourceMessageID != 5 {
		t.Errorf("order = [%d, %d], want [3, 5]", u.Items[0].SourceMessageID, u.Items[1].SourceMessageID)
	}
}

func TestAlbumBufferExtendsWindowOnNewItem(t *testing.T) {
	flushed := make(chan *AlbumUnit, 1)
	b := NewAlbumBuffer(40*time.Millisecond, func(u *AlbumUnit) { flushed <- u })
	b.Add(albumMsg(1))
	time.Sleep(20 * time.Millisecond)
	b.Add(albumMsg(2))
	u := waitFlush(t, flushed)
	if len(u.Items) != 2 {
		t.Errorf("items = %d, want 2 (late item joined the group)", len(u.Items))
	}
}

func TestAlbumBufferSeparatesGroups(t *testing.T) {
	flushed := make(chan *AlbumUnit, 2)
	b := NewAlbumBuffer(20*time.Millisecond, func(u *AlbumUnit) { flushed <- u })
	m1 := albumMsg(1)
	m1.MediaGroupID = "group-a"
	m2 := albumMsg(2)
	m2.MediaGroupID = "group-b"
	b.Add(m1)
	b.Add(m2)
	seen := map[string]int{}
	for i := 0; i < 2; i++ {
		u := waitFlush(t, flushed)
		seen[u.Key.MediaGroupID] = len(u.Items)
	}
	if seen["group-a"] != 1 || seen["group-b"] != 1 {
		t.Errorf("groups = %v, want one item each", seen)
	}
}

func TestAlbumBufferFlushAll(t *testing.T) {
	flushed := make(chan *AlbumUnit, 1)
	b := NewAlbumBuffer(time.Hour, func(u *AlbumUnit) { flushed <- u })
	b.Add(albumMsg(7))
	b.FlushAll()
	u := waitFlush(t, flushed)
	if len(u.Items) != 1 || u.Items[0].SourceMessageID != 7 {
		t.Errorf("items = %+v, want single message 7", u.Items)
	}
}
