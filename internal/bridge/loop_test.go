package bridge

import (
	"testing"
	"time"
)

func TestSentTrackerMarkAndSeen(t *testing.T) {
	tr := NewSentTracker(time.Minute)
	tr.Mark(PlatformTelegram, 1, 100)
	if !tr.Seen(PlatformTelegram, 1, 100) {
		t.Error("Seen = false for marked message")
	}
	if tr.Seen(PlatformTelegram, 1, 101) {
		t.Error("Seen = true for unmarked message")
	}
	if tr.Seen(PlatformBale, 1, 100) {
		t.Error("Seen = true for different platform")
	}
	if tr.Seen(PlatformTelegram, 2, 100) {
		t.Error("Seen = true for different chat")
	}
}

func TestSentTrackerExpiration(t *testing.T) {
	tr := NewSentTracker(30 * time.Millisecond)
	tr.Mark(PlatformTelegram, 1, 100)
	if !tr.Seen(PlatformTelegram, 1, 100) {
		t.Fatal("Seen = false before expiry")
	}
	time.Sleep(60 * time.Millisecond)
	if tr.Seen(PlatformTelegram, 1, 100) {
		t.Error("Seen = true after TTL expired")
	}
}
