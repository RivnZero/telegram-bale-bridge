package storage
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"telegram-bale-bridge/internal/bridge"
	"telegram-bale-bridge/migrations"
)
const (
	statusProcessing = "processing"
	statusDelivered  = "delivered"
	statusFailed     = "failed"
)
type SQLite struct {
	db *sql.DB
}
func Open(path string) (*SQLite, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	schema, err := migrations.FS.ReadFile("001_init.sql")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("read migration: %w", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply migration: %w", err)
	}
	return &SQLite{db: db}, nil
}
func (s *SQLite) Close() error { return s.db.Close() }
func (s *SQLite) Delivered(ctx context.Context, src bridge.Platform, srcChatID int64, srcKey string, dest bridge.Platform) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM deliveries
		 WHERE source_platform=? AND source_chat_id=? AND source_key=? AND destination_platform=? AND status=?`,
		src, srcChatID, srcKey, dest, statusDelivered).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
func (s *SQLite) MarkProcessing(ctx context.Context, src bridge.Platform, srcChatID int64, srcKey string, dest bridge.Platform) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO deliveries(source_platform, source_chat_id, source_key, destination_platform, status)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT(source_platform, source_chat_id, source_key, destination_platform) DO NOTHING`,
		src, srcChatID, srcKey, dest, statusProcessing)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
func (s *SQLite) MarkDelivered(ctx context.Context, src bridge.Platform, srcChatID int64, srcKey string, dest bridge.Platform, destMessageID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE deliveries SET status=?, destination_message_id=?, error_message=NULL
		 WHERE source_platform=? AND source_chat_id=? AND source_key=? AND destination_platform=?`,
		statusDelivered, destMessageID, src, srcChatID, srcKey, dest)
	return err
}
func (s *SQLite) MarkFailed(ctx context.Context, src bridge.Platform, srcChatID int64, srcKey string, dest bridge.Platform, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deliveries(source_platform, source_chat_id, source_key, destination_platform, status, error_message)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(source_platform, source_chat_id, source_key, destination_platform)
		 DO UPDATE SET status=excluded.status, error_message=excluded.error_message`,
		src, srcChatID, srcKey, dest, statusFailed, errMsg)
	return err
}
func (s *SQLite) RecoverInterrupted(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE deliveries SET status=?, error_message=?
		 WHERE status=?`,
		statusFailed, "interrupted", statusProcessing)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
