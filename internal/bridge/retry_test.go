package bridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

type tempErr struct{ temporary bool }

func (e tempErr) Error() string { return "test error" }

type retryErr struct{ seconds int }

func (e retryErr) Error() string { return "retry_after" }

func alwaysTemp(err error) bool { return err != nil }

func TestRetrySucceedsAfterTransientFailures(t *testing.T) {
	attempts := 0
	policy := RetryPolicy{Delays: []time.Duration{time.Millisecond, 2 * time.Millisecond}, IsTemporary: alwaysTemp}
	got, err := WithRetry(context.Background(), policy, func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, tempErr{true}
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("WithRetry: %v", err)
	}
	if got != 42 {
		t.Errorf("result = %d, want 42", got)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetryGivesUpAfterDelaysExhausted(t *testing.T) {
	attempts := 0
	policy := RetryPolicy{Delays: []time.Duration{time.Millisecond}, IsTemporary: alwaysTemp}
	_, err := WithRetry(context.Background(), policy, func() (int, error) {
		attempts++
		return 0, tempErr{true}
	})
	if err == nil {
		t.Fatal("expected error after delays exhausted")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (one initial + one retry)", attempts)
	}
}

func TestNoRetryOnPermanentError(t *testing.T) {
	attempts := 0
	policy := RetryPolicy{Delays: []time.Duration{time.Millisecond}, IsTemporary: func(err error) bool { return false }}
	_, err := WithRetry(context.Background(), policy, func() (int, error) {
		attempts++
		return 0, tempErr{false}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on permanent error)", attempts)
	}
}

func TestRetryAfterIsHonored(t *testing.T) {
	policy := RetryPolicy{
		Delays:      []time.Duration{time.Millisecond},
		IsTemporary: alwaysTemp,
		RetryAfter: func(err error) float64 {
			var re retryErr
			if errors.As(err, &re) {
				return float64(re.seconds)
			}
			return 0
		},
	}
	attempts := 0
	start := time.Now()
	_, err := WithRetry(context.Background(), policy, func() (int, error) {
		attempts++
		if attempts == 1 {
			return 0, retryErr{seconds: 1}
		}
		return 1, nil
	})
	if err != nil {
		t.Fatalf("WithRetry: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("returned after %v, want >= 1s from retry_after", elapsed)
	}
}

func TestCanceledContextAbortsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	policy := RetryPolicy{Delays: []time.Duration{time.Hour}, IsTemporary: alwaysTemp}
	_, err := WithRetry(ctx, policy, func() (int, error) { return 0, tempErr{true} })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
