package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v71/github"
)

type mockRater struct {
	limit     int
	remaining int
	reset     time.Time
	err       error
}

func (m *mockRater) RateLimit(_ context.Context) (int, int, time.Time, error) {
	return m.limit, m.remaining, m.reset, m.err
}

func TestCheckRateLimit_Success(t *testing.T) {
	r := &mockRater{limit: 100, remaining: 50, reset: time.Now().Add(time.Hour)}
	err := CheckRateLimit(context.Background(), r, "github", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckRateLimit_Exhausted(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	r := &mockRater{limit: 100, remaining: 0, reset: reset}
	err := CheckRateLimit(context.Background(), r, "github", 20)
	if err == nil {
		t.Fatal("expected error for exhausted rate limit")
	}
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatal("expected RateLimitError")
	}
	if rateErr.Provider != "github" {
		t.Errorf("provider = %q, want github", rateErr.Provider)
	}
	if !errors.Is(err, ErrRateExhausted) {
		t.Error("expected ErrRateExhausted")
	}
}

func TestCheckRateLimit_Warning(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	r := &mockRater{limit: 100, remaining: 5, reset: reset}
	err := CheckRateLimit(context.Background(), r, "gitlab", 20)
	if err == nil {
		t.Fatal("expected warning error for low rate limit")
	}
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatal("expected RateLimitError")
	}
	if rateErr.Provider != "gitlab" {
		t.Errorf("provider = %q, want gitlab", rateErr.Provider)
	}
}

func TestCheckRateLimit_RateLimitError(t *testing.T) {
	r := &mockRater{err: errors.New("network failure")}
	err := CheckRateLimit(context.Background(), r, "github", 20)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && err.Error() == "" {
		t.Error("expected error message")
	}
}

func TestRateLimitError_Error(t *testing.T) {
	reset := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	e := &RateLimitError{
		Provider:  "github",
		Limit:     100,
		Remaining: 0,
		Reset:     reset,
		Cause:     ErrRateExhausted,
	}
	expected := fmt.Sprintf("github: rate limit (limit=100, remaining=0, reset=%s)", reset.Format(time.RFC3339))
	if e.Error() != expected {
		t.Errorf("error = %q, want %q", e.Error(), expected)
	}
}

func TestRateLimitError_Unwrap(t *testing.T) {
	e := &RateLimitError{Cause: ErrRateExhausted}
	if !errors.Is(e, ErrRateExhausted) {
		t.Error("expected Unwrap to return ErrRateExhausted")
	}
}

func TestIsRateLimitError(t *testing.T) {
	// Non-errorResponse
	if isRateLimitError(errors.New("random")) {
		t.Error("random error should not be rate limit")
	}

	// ErrorResponse with rate limit message
	ger := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusTooManyRequests},
		Errors: []github.Error{
			{Message: "API rate limit exceeded"},
		},
	}
	if !isRateLimitError(ger) {
		t.Error("expected rate limit error for API rate limit exceeded")
	}

	// ErrorResponse with "You have exceeded a rate limit"
	ger2 := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusOK},
		Errors: []github.Error{
			{Message: "You have exceeded a rate limit"},
		},
	}
	if !isRateLimitError(ger2) {
		t.Error("expected rate limit error for exceeded rate limit")
	}

	// ErrorResponse without matching message but with 429 status
	ger3 := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusTooManyRequests},
	}
	if !isRateLimitError(ger3) {
		t.Error("expected rate limit error for 429 status")
	}

	// ErrorResponse without match
	ger4 := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusOK},
		Errors:   []github.Error{{Message: "not found"}},
	}
	if isRateLimitError(ger4) {
		t.Error("unexpected rate limit error")
	}
}
