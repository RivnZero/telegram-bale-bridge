package bridge
import (
	"context"
	"time"
)
type RetryPolicy struct {
	Delays     []time.Duration
	IsTemporary func(error) bool
	RetryAfter  func(error) float64
}
func WithRetry[T any](ctx context.Context, policy RetryPolicy, fn func() (T, error)) (T, error) {
	var zero T
	for attempt := 0; ; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if attempt >= len(policy.Delays) || policy.IsTemporary == nil || !policy.IsTemporary(err) {
			return zero, err
		}
		wait := policy.Delays[attempt]
		if policy.RetryAfter != nil {
			if ra := policy.RetryAfter(err); ra > 0 {
				if w := time.Duration(ra * float64(time.Second)); w > wait {
					wait = w
				}
			}
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}
}
