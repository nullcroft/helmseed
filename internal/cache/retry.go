package cache

import (
	"context"
	"fmt"
	"time"
)

type RetryableFunc func(ctx context.Context) error

func WithRetry(ctx context.Context, fn RetryableFunc) error {
	const maxRetries = 3

	var lastErr error
	delay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if attempt == maxRetries {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
	}

	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}