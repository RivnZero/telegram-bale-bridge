package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const MaxFileSizeBytes = 20 * 1024 * 1024

type Sender interface {
	Platform() Platform
	ChatID() int64
	Send(ctx context.Context, msg *BridgeMessage, path string) (int64, error)
	SendAlbum(ctx context.Context, unit *AlbumUnit, paths []string) ([]int64, error)
}
type Store interface {
	Delivered(ctx context.Context, src Platform, srcChatID int64, srcKey string, dest Platform) (bool, error)
	MarkProcessing(ctx context.Context, src Platform, srcChatID int64, srcKey string, dest Platform) (bool, error)
	MarkDelivered(ctx context.Context, src Platform, srcChatID int64, srcKey string, dest Platform, destMessageID int64) error
	MarkFailed(ctx context.Context, src Platform, srcChatID int64, srcKey string, dest Platform, errMsg string) error
	RecoverInterrupted(ctx context.Context) (int64, error)
	Close() error
}
type Downloader interface {
	Download(ctx context.Context, fileID, dest string) error
}
type Worker struct {
	logger     *slog.Logger
	store      Store
	sender     Sender
	downloader Downloader
	tracker    *SentTracker
	tempDir    string
	queue      chan any
	albums     *AlbumBuffer
	policy     RetryPolicy
	mu         sync.Mutex
	closed     bool
}

func NewWorker(
	logger *slog.Logger,
	store Store,
	sender Sender,
	downloader Downloader,
	tracker *SentTracker,
	tempDir string,
	queueSize int,
	albumDelay time.Duration,
	policy RetryPolicy,
) *Worker {
	w := &Worker{
		logger:     logger,
		store:      store,
		sender:     sender,
		downloader: downloader,
		tracker:    tracker,
		tempDir:    tempDir,
		queue:      make(chan any, queueSize),
		policy:     policy,
	}
	w.albums = NewAlbumBuffer(albumDelay, w.enqueueAlbum)
	return w
}
func (w *Worker) Submit(ctx context.Context, msg *BridgeMessage) bool {
	if delivered, err := w.store.Delivered(ctx, msg.SourcePlatform, msg.SourceChatID, msg.DedupeKey(), w.sender.Platform()); err == nil && delivered {
		w.logger.Debug("already delivered, skipping", "source", msg.SourcePlatform, "key", msg.DedupeKey())
		return true
	}
	if msg.MediaGroupID != "" {
		w.albums.Add(msg)
		return true
	}
	return w.enqueue(msg)
}
func (w *Worker) enqueueAlbum(unit *AlbumUnit) {
	if len(unit.Items) == 0 {
		return
	}
	w.enqueue(unit)
}
func (w *Worker) enqueue(item any) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	select {
	case w.queue <- item:
		return true
	default:
		w.reject(item, "queue_full")
		return false
	}
}
func (w *Worker) reject(item any, reason string) {
	switch v := item.(type) {
	case *BridgeMessage:
		w.logger.Warn("queue full, rejecting", "key", v.DedupeKey())
		_ = w.store.MarkFailed(context.Background(), v.SourcePlatform, v.SourceChatID, v.DedupeKey(), w.sender.Platform(), reason)
	case *AlbumUnit:
		if len(v.Items) > 0 {
			m := v.Items[0]
			w.logger.Warn("queue full, rejecting album", "key", m.DedupeKey())
			_ = w.store.MarkFailed(context.Background(), m.SourcePlatform, m.SourceChatID, m.DedupeKey(), w.sender.Platform(), reason)
		}
	}
}
func (w *Worker) Run() {
	for item := range w.queue {
		switch v := item.(type) {
		case *BridgeMessage:
			w.processSingle(v)
		case *AlbumUnit:
			w.processAlbum(v)
		}
	}
	w.logger.Info("worker drained and stopped")
}
func (w *Worker) Shutdown() {
	w.albums.FlushAll()
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()
	close(w.queue)
}
func (w *Worker) processSingle(msg *BridgeMessage) {
	key := msg.DedupeKey()
	dest := w.sender.Platform()
	if ok, err := w.store.MarkProcessing(context.Background(), msg.SourcePlatform, msg.SourceChatID, key, dest); err != nil {
		w.logger.Error("mark processing failed", "error", err)
		return
	} else if !ok {
		w.logger.Debug("duplicate, skipping", "key", key)
		return
	}
	tmpDir := w.dirFor(msg.SourcePlatform, msg.SourceChatID, key)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		w.fail(msg, key, err)
		return
	}
	defer os.RemoveAll(tmpDir)
	var (
		destID int64
		err    error
	)
	if msg.Kind == KindText {
		destID, err = WithRetry(context.Background(), w.policy, func() (int64, error) {
			return w.sender.Send(context.Background(), msg, "")
		})
	} else {
		if msg.FileSize > MaxFileSizeBytes {
			w.fail(msg, key, errors.New("file_too_large"))
			return
		}
		path := filepath.Join(tmpDir, safeName(msg))
		if err := w.retryErr(func() error { return w.downloader.Download(context.Background(), msg.FileID, path) }); err != nil {
			w.fail(msg, key, err)
			return
		}
		destID, err = WithRetry(context.Background(), w.policy, func() (int64, error) {
			return w.sender.Send(context.Background(), msg, path)
		})
	}
	if err != nil {
		w.fail(msg, key, err)
		return
	}
	w.tracker.Mark(dest, w.sender.ChatID(), destID)
	if err := w.store.MarkDelivered(context.Background(), msg.SourcePlatform, msg.SourceChatID, key, dest, destID); err != nil {
		w.logger.Error("mark delivered failed", "error", err)
	}
	w.logger.Info("forwarded",
		"source", msg.SourcePlatform, "key", key,
		"kind", msg.Kind, "dest", dest, "dest_id", destID)
}
func (w *Worker) processAlbum(unit *AlbumUnit) {
	if len(unit.Items) == 0 {
		return
	}
	first := unit.Items[0]
	key := first.DedupeKey()
	dest := w.sender.Platform()
	if ok, err := w.store.MarkProcessing(context.Background(), first.SourcePlatform, first.SourceChatID, key, dest); err != nil {
		w.logger.Error("mark processing failed", "error", err)
		return
	} else if !ok {
		w.logger.Debug("duplicate album, skipping", "key", key)
		return
	}
	tmpDir := w.dirFor(first.SourcePlatform, first.SourceChatID, key)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		w.fail(first, key, err)
		return
	}
	defer os.RemoveAll(tmpDir)
	paths := make([]string, len(unit.Items))
	for i, m := range unit.Items {
		if m.FileSize > MaxFileSizeBytes {
			w.fail(first, key, errors.New("file_too_large"))
			return
		}
		path := filepath.Join(tmpDir, fmt.Sprintf("%02d_%s", i, safeName(m)))
		if err := w.retryErr(func() error { return w.downloader.Download(context.Background(), m.FileID, path) }); err != nil {
			w.fail(first, key, err)
			return
		}
		paths[i] = path
	}
	destIDs, err := WithRetry(context.Background(), w.policy, func() ([]int64, error) {
		return w.sender.SendAlbum(context.Background(), unit, paths)
	})
	if err != nil {
		w.fail(first, key, err)
		return
	}
	for _, id := range destIDs {
		w.tracker.Mark(dest, w.sender.ChatID(), id)
	}
	var firstID int64
	if len(destIDs) > 0 {
		firstID = destIDs[0]
	}
	if err := w.store.MarkDelivered(context.Background(), first.SourcePlatform, first.SourceChatID, key, dest, firstID); err != nil {
		w.logger.Error("mark delivered failed", "error", err)
	}
	w.logger.Info("forwarded album",
		"source", first.SourcePlatform, "key", key,
		"items", len(unit.Items), "dest", dest)
}
func (w *Worker) retryErr(fn func() error) error {
	_, err := WithRetry(context.Background(), w.policy, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
func (w *Worker) fail(msg *BridgeMessage, key string, err error) {
	_ = w.store.MarkFailed(context.Background(), msg.SourcePlatform, msg.SourceChatID, key, w.sender.Platform(), err.Error())
	w.logger.Error("delivery failed", "source", msg.SourcePlatform, "key", key, "error", err)
}
func (w *Worker) dirFor(platform Platform, chatID int64, key string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_").Replace(key)
	return filepath.Join(w.tempDir, fmt.Sprintf("%s-%d-%s", platform, chatID, safe))
}
func safeName(m *BridgeMessage) string {
	name := filepath.Base(m.FileName)
	if name == "" || name == "." || name == ".." {
		name = fmt.Sprintf("%s_%d.bin", m.Kind, m.SourceMessageID)
	}
	return strings.NewReplacer("..", "_", "/", "_", "\\", "_").Replace(name)
}
