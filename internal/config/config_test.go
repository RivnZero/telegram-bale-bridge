package config

import "testing"

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:telegram")
	t.Setenv("TELEGRAM_CHAT_ID", "1")
	t.Setenv("BALE_BOT_TOKEN", "456:bale")
	t.Setenv("BALE_CHAT_ID", "2")
}

func TestLoadValid(t *testing.T) {
	setRequired(t)
	t.Setenv("BRIDGE_DIRECTION", "telegram-to-bale")
	t.Setenv("QUEUE_SIZE", "50")
	t.Setenv("ALBUM_DELAY", "2s")
	t.Setenv("LOG_LEVEL", "DEBUG")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Direction != TelegramToBale {
		t.Errorf("Direction = %q, want %q", cfg.Direction, TelegramToBale)
	}
	if cfg.QueueSize != 50 {
		t.Errorf("QueueSize = %d, want 50", cfg.QueueSize)
	}
	if cfg.AlbumDelay.String() != "2s" {
		t.Errorf("AlbumDelay = %v, want 2s", cfg.AlbumDelay)
	}
	if cfg.LogLevel.String() != "DEBUG" {
		t.Errorf("LogLevel = %v, want DEBUG", cfg.LogLevel)
	}
	if cfg.TelegramChatID != 1 || cfg.BaleChatID != 2 {
		t.Errorf("chat ids = %d/%d, want 1/2", cfg.TelegramChatID, cfg.BaleChatID)
	}
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Direction != Bidirectional {
		t.Errorf("Direction = %q, want bidirectional", cfg.Direction)
	}
	if cfg.QueueSize != 100 {
		t.Errorf("QueueSize = %d, want 100", cfg.QueueSize)
	}
	if cfg.AlbumDelay != 700_000_000 {
		t.Errorf("AlbumDelay = %v, want 700ms", cfg.AlbumDelay)
	}
	if cfg.LogLevel.String() != "INFO" {
		t.Errorf("LogLevel = %v, want INFO", cfg.LogLevel)
	}
	if cfg.TelegramAPIBaseURL != "https://api.telegram.org" {
		t.Errorf("TelegramAPIBaseURL = %q", cfg.TelegramAPIBaseURL)
	}
	if cfg.BaleAPIBaseURL != "https://tapi.bale.ai" {
		t.Errorf("BaleAPIBaseURL = %q", cfg.BaleAPIBaseURL)
	}
	if cfg.DatabasePath != "./data/bridge.db" || cfg.TempDir != "./data/tmp" {
		t.Errorf("paths = %q/%q", cfg.DatabasePath, cfg.TempDir)
	}
}

func TestLoadMissingToken(t *testing.T) {
	setRequired(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing TELEGRAM_BOT_TOKEN")
	}
}

func TestLoadBadChatID(t *testing.T) {
	setRequired(t)
	t.Setenv("TELEGRAM_CHAT_ID", "abc")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-numeric chat id")
	}
}

func TestLoadBadDirection(t *testing.T) {
	setRequired(t)
	t.Setenv("BRIDGE_DIRECTION", "sideways")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid direction")
	}
}

func TestLoadBadQueueSize(t *testing.T) {
	setRequired(t)
	t.Setenv("QUEUE_SIZE", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for zero queue size")
	}
}

func TestLoadBadAlbumDelay(t *testing.T) {
	setRequired(t)
	t.Setenv("ALBUM_DELAY", "soon")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid album delay")
	}
}

func TestLoadAlbumDelayBareMilliseconds(t *testing.T) {
	setRequired(t)
	t.Setenv("ALBUM_DELAY", "700")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AlbumDelay != 700_000_000 {
		t.Errorf("AlbumDelay = %v, want 700ms", cfg.AlbumDelay)
	}
}

func TestLoadBadLogLevel(t *testing.T) {
	setRequired(t)
	t.Setenv("LOG_LEVEL", "LOUD")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}
