package cache

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetry_Success(t *testing.T) {
	called := 0
	fn := func(ctx context.Context) error {
		called++
		return nil
	}
	err := WithRetry(context.Background(), fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 1 {
		t.Errorf("called %d times, want 1", called)
	}
}

func TestWithRetry_Failure(t *testing.T) {
	called := 0
	fn := func(ctx context.Context) error {
		called++
		return errors.New("always fails")
	}
	err := WithRetry(context.Background(), fn)
	if err == nil {
		t.Fatal("expected error")
	}
	// maxRetries = 3, so attempts = 0,1,2,3 = 4 total
	if called != 4 {
		t.Errorf("called %d times, want 4", called)
	}
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fn := func(ctx context.Context) error {
		return errors.New("fail")
	}
	err := WithRetry(ctx, fn)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWithRetry_EventualSuccess(t *testing.T) {
	called := 0
	fn := func(ctx context.Context) error {
		called++
		if called < 3 {
			return errors.New("temporary failure")
		}
		return nil
	}
	err := WithRetry(context.Background(), fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 3 {
		t.Errorf("called %d times, want 3", called)
	}
}
