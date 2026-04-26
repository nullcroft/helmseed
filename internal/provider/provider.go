package provider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Repo struct {
	Name          string
	CloneURL      string
	HTTPSURL      string
	DefaultBranch string
}

type Provider interface {
	ListRepos(ctx context.Context, group string) ([]Repo, error)
}

var (
	ErrRateLimited   = errors.New("rate limited by API provider")
	ErrRateExhausted = errors.New("API rate limit exhausted")
)

type Rater interface {
	RateLimit(ctx context.Context) (int, int, time.Time, error)
}

func CheckRateLimit(ctx context.Context, r Rater, provider string, warnThreshold int) error {
	limit, remaining, reset, err := r.RateLimit(ctx)
	if err != nil {
		return fmt.Errorf("failed to check %s rate limit: %w (operations may be rate limited)", provider, err)
	}

	if remaining <= 0 {
		return &RateLimitError{
			Provider:  provider,
			Limit:     limit,
			Remaining: remaining,
			Reset:     reset,
			Cause:     ErrRateExhausted,
		}
	}

	if remaining <= warnThreshold {
		return &RateLimitError{
			Provider:  provider,
			Limit:     limit,
			Remaining: remaining,
			Reset:     reset,
			Cause:     fmt.Errorf("low rate limit (remaining=%d, reset=%s)", remaining, reset.Format(time.RFC3339)),
		}
	}

	return nil
}

type RateLimitError struct {
	Provider  string
	Limit     int
	Remaining int
	Reset     time.Time
	Cause     error
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s: rate limit (limit=%d, remaining=%d, reset=%s)",
		e.Provider, e.Limit, e.Remaining, e.Reset.Format(time.RFC3339))
}

func (e *RateLimitError) Unwrap() error {
	return e.Cause
}
