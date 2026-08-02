package main

import (
	"context"
	"fmt"
	"github.com/RivnZero/telegram-bale-bridge/internal/bale"
	"github.com/RivnZero/telegram-bale-bridge/internal/bridge"
	"github.com/RivnZero/telegram-bale-bridge/internal/config"
	"github.com/RivnZero/telegram-bale-bridge/internal/storage"
	"github.com/RivnZero/telegram-bale-bridge/internal/telegram"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		slog.Error("bridge failed", "error", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, cfg *config.Config) error {
	logger := slog.Default()
	if err := os.RemoveAll(cfg.TempDir); err != nil {
		return fmt.Errorf("clean temp directory: %w", err)
	}
	if err := os.MkdirAll(cfg.TempDir, 0o755); err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}
	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if n, err := store.RecoverInterrupted(ctx); err != nil {
		return fmt.Errorf("recover interrupted deliveries: %w", err)
	} else if n > 0 {
		logger.Info("recovered interrupted deliveries", "count", n)
	}
	tracker := bridge.NewSentTracker(5 * time.Minute)
	retryDelays := []time.Duration{time.Second, 3 * time.Second, 10 * time.Second}
	var tgRecv *telegram.Receiver
	tgBot, err := telegram.NewBot(cfg.TelegramBotToken, cfg.TelegramAPIBaseURL, func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		if tgRecv != nil {
			tgRecv.Handle(ctx, update)
		}
	})
	if err != nil {
		return fmt.Errorf("telegram bot: %w", err)
	}
	if _, err := tgBot.GetMe(ctx); err != nil {
		return fmt.Errorf("telegram getMe: %w", err)
	}
	baleClient := bale.NewClient(cfg.BaleBotToken, cfg.BaleAPIBaseURL, &http.Client{Timeout: 2 * time.Minute})
	baleMe, err := baleClient.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("bale getMe: %w", err)
	}
	var (
		workers  []*bridge.Worker
		workersW sync.WaitGroup
		pollersW sync.WaitGroup
	)
	if cfg.Direction == config.TelegramToBale || cfg.Direction == config.Bidirectional {
		policy := bridge.RetryPolicy{Delays: retryDelays, IsTemporary: bale.IsTemporary, RetryAfter: bale.RetryAfterSeconds}
		baleSender := bale.NewSender(baleClient, cfg.BaleChatID)
		var submit func(context.Context, *bridge.BridgeMessage)
		tgRecv = telegram.NewReceiver(tgBot, cfg.TelegramChatID, tracker, func(ctx context.Context, m *bridge.BridgeMessage) {
			submit(ctx, m)
		})
		wTB := bridge.NewWorker(logger.With("direction", "telegram-to-bale"), store, baleSender, tgRecv, tracker,
			cfg.TempDir, cfg.QueueSize, cfg.AlbumDelay, policy)
		submit = func(ctx context.Context, m *bridge.BridgeMessage) { wTB.Submit(ctx, m) }
		workers = append(workers, wTB)
		workersW.Add(1)
		go func() {
			defer workersW.Done()
			wTB.Run()
		}()
		pollersW.Add(1)
		go func() {
			defer pollersW.Done()
			tgBot.Start(ctx)
		}()
	}
	if cfg.Direction == config.BaleToTelegram || cfg.Direction == config.Bidirectional {
		policy := bridge.RetryPolicy{Delays: retryDelays, IsTemporary: telegram.IsTemporary, RetryAfter: telegram.RetryAfterSeconds}
		tgSender := telegram.NewSender(tgBot, cfg.TelegramChatID)
		var baleSubmit func(context.Context, *bridge.BridgeMessage)
		baleRecv := bale.NewReceiver(baleClient, cfg.BaleChatID, baleMe.ID, tracker, func(ctx context.Context, m *bridge.BridgeMessage) {
			baleSubmit(ctx, m)
		})
		wBT := bridge.NewWorker(logger.With("direction", "bale-to-telegram"), store, tgSender, baleRecv, tracker,
			cfg.TempDir, cfg.QueueSize, cfg.AlbumDelay, policy)
		baleSubmit = func(ctx context.Context, m *bridge.BridgeMessage) { wBT.Submit(ctx, m) }
		workers = append(workers, wBT)
		workersW.Add(1)
		go func() {
			defer workersW.Done()
			wBT.Run()
		}()
		poller := bale.NewPoller(baleClient, logger.With("direction", "bale-to-telegram"), baleRecv.Handle)
		pollersW.Add(1)
		go func() {
			defer pollersW.Done()
			poller.Run(ctx)
		}()
	}
	logger.Info("bridge started",
		"direction", cfg.Direction,
		"telegram_chat", cfg.TelegramChatID,
		"bale_chat", cfg.BaleChatID,
		"bale_api", cfg.BaleAPIBaseURL)
	<-ctx.Done()
	logger.Info("shutdown requested")
	pollersW.Wait()
	for _, w := range workers {
		w.Shutdown()
	}
	workersW.Wait()
	logger.Info("shutdown complete")
	return nil
}
