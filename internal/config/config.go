package config
import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)
type Direction string
const (
	TelegramToBale Direction = "telegram-to-bale"
	BaleToTelegram Direction = "bale-to-telegram"
	Bidirectional  Direction = "bidirectional"
)
type Config struct {
	TelegramBotToken    string
	TelegramAPIBaseURL  string
	TelegramChatID      int64
	BaleBotToken        string
	BaleChatID          int64
	BaleAPIBaseURL      string
	Direction Direction
	DatabasePath string
	TempDir      string
	QueueSize    int
	AlbumDelay   time.Duration
	LogLevel     slog.Level
}
func Load() (*Config, error) {
	get := func(key string) string { return strings.TrimSpace(os.Getenv(key)) }
	cfg := &Config{
		TelegramBotToken:    get("TELEGRAM_BOT_TOKEN"),
		TelegramAPIBaseURL:  strings.TrimRight(get("TELEGRAM_API_BASE_URL"), "/"),
		BaleBotToken:        get("BALE_BOT_TOKEN"),
		BaleAPIBaseURL:      strings.TrimRight(get("BALE_API_BASE_URL"), "/"),
		DatabasePath:        defaultStr(get("DATABASE_PATH"), "./data/bridge.db"),
		TempDir:             defaultStr(get("TEMP_DIRECTORY"), "./data/tmp"),
	}
	if cfg.TelegramAPIBaseURL == "" {
		cfg.TelegramAPIBaseURL = "https://api.telegram.org"
	}
	if cfg.BaleAPIBaseURL == "" {
		cfg.BaleAPIBaseURL = "https://tapi.bale.ai"
	}
	switch get("BRIDGE_DIRECTION") {
	case "", "bidirectional":
		cfg.Direction = Bidirectional
	case "telegram-to-bale":
		cfg.Direction = TelegramToBale
	case "bale-to-telegram":
		cfg.Direction = BaleToTelegram
	default:
		return nil, fmt.Errorf("BRIDGE_DIRECTION invalid: %q (want telegram-to-bale, bale-to-telegram or bidirectional)", get("BRIDGE_DIRECTION"))
	}
	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	id, err := parseChatID(get("TELEGRAM_CHAT_ID"))
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID: %w", err)
	}
	cfg.TelegramChatID = id
	if cfg.BaleBotToken == "" {
		return nil, fmt.Errorf("BALE_BOT_TOKEN is required")
	}
	id, err = parseChatID(get("BALE_CHAT_ID"))
	if err != nil {
		return nil, fmt.Errorf("BALE_CHAT_ID: %w", err)
	}
	cfg.BaleChatID = id
	if cfg.QueueSize, err = positiveInt(get("QUEUE_SIZE"), 100); err != nil {
		return nil, fmt.Errorf("QUEUE_SIZE: %w", err)
	}
	if cfg.AlbumDelay, err = parseDelay(get("ALBUM_DELAY"), 700*time.Millisecond); err != nil {
		return nil, fmt.Errorf("ALBUM_DELAY: %w", err)
	}
	if cfg.LogLevel, err = parseLogLevel(get("LOG_LEVEL")); err != nil {
		return nil, err
	}
	return cfg, nil
}
func parseChatID(raw string) (int64, error) {
	if raw == "" {
		return 0, fmt.Errorf("required (numeric chat id)")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("must be a numeric chat id, got %q", raw)
	}
	return id, nil
}
func positiveInt(raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("must be a positive integer, got %q", raw)
	}
	return n, nil
}
func parseDelay(raw string, def time.Duration) (time.Duration, error) {
	if raw == "" {
		return def, nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("must be positive, got %q", raw)
		}
		return d, nil
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Millisecond, nil
	}
	return 0, fmt.Errorf("must be a duration like \"700ms\" or milliseconds, got %q", raw)
}
func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToUpper(raw) {
	case "", "INFO":
		return slog.LevelInfo, nil
	case "DEBUG":
		return slog.LevelDebug, nil
	case "WARN", "WARNING":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL invalid: %q (want DEBUG, INFO, WARN or ERROR)", raw)
	}
}
func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
