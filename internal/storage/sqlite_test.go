package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/RivnZero/telegram-bale-bridge/internal/bridge"
)

func openTest(t *testing.T) *SQLite {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMarkProcessingDeduplicates(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	first, err := s.MarkProcessing(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale)
	if err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}
	if !first {
		t.Error("first MarkProcessing = false, want true")
	}
	second, err := s.MarkProcessing(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale)
	if err != nil {
		t.Fatalf("MarkProcessing second: %v", err)
	}
	if second {
		t.Error("second MarkProcessing = true, want false (deduplicated)")
	}
}

func TestDeliveredFlow(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if ok, _ := s.Delivered(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale); ok {
		t.Error("Delivered = true before delivery")
	}
	if ok, _ := s.MarkProcessing(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale); !ok {
		t.Fatal("MarkProcessing failed")
	}
	if err := s.MarkDelivered(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale, 500); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if ok, err := s.Delivered(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale); err != nil || !ok {
		t.Errorf("Delivered = %v/%v, want true", ok, err)
	}
}

func TestFailedIsNotDelivered(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if ok, _ := s.MarkProcessing(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale); !ok {
		t.Fatal("MarkProcessing failed")
	}
	if err := s.MarkFailed(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale, "boom"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if ok, _ := s.Delivered(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale); ok {
		t.Error("Delivered = true for failed delivery")
	}
}

func TestRecoverInterrupted(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if ok, _ := s.MarkProcessing(ctx, bridge.PlatformBale, 2, "message:7", bridge.PlatformTelegram); !ok {
		t.Fatal("MarkProcessing failed")
	}
	n, err := s.RecoverInterrupted(ctx)
	if err != nil {
		t.Fatalf("RecoverInterrupted: %v", err)
	}
	if n != 1 {
		t.Errorf("recovered = %d, want 1", n)
	}
	n, err = s.RecoverInterrupted(ctx)
	if err != nil {
		t.Fatalf("RecoverInterrupted second: %v", err)
	}
	if n != 0 {
		t.Errorf("recovered second = %d, want 0", n)
	}
}

func TestDifferentDestinationsAreIndependent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if ok, _ := s.MarkProcessing(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformBale); !ok {
		t.Fatal("MarkProcessing failed")
	}
	if ok, err := s.MarkProcessing(ctx, bridge.PlatformTelegram, 1, "message:100", bridge.PlatformTelegram); err != nil || !ok {
		t.Errorf("same source key to another destination should be allowed, got %v/%v", ok, err)
	}
}
