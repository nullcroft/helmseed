package provider

import (
	"testing"
)

func TestNewGitHubProvider(t *testing.T) {
	// Without token
	p := newGitHubProvider("")
	if p.client == nil {
		t.Error("client should not be nil")
	}

	// With token
	p = newGitHubProvider("ghp_test")
	if p.client == nil {
		t.Error("client should not be nil")
	}
}

func TestNewGitLabProvider(t *testing.T) {
	// Without base URL
	p, err := newGitLabProvider("glpat-test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.client == nil {
		t.Error("client should not be nil")
	}
	if p.baseURL != "" {
		t.Error("baseURL should be empty")
	}

	// With base URL
	p, err = newGitLabProvider("glpat-test", "https://gitlab.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "https://gitlab.example.com" {
		t.Errorf("baseURL = %q, want https://gitlab.example.com", p.baseURL)
	}

	// Invalid base URL (causes client creation error) - actually gitlab.NewClient
	// doesn't validate the URL until use, so we skip that.
}

func TestGitHubProviderImplementsRater(t *testing.T) {
	p := newGitHubProvider("")
	if _, ok := interface{}(p).(Rater); !ok {
		t.Error("*gitHubProvider should implement Rater")
	}
}

func TestGitLabProviderImplementsRater(t *testing.T) {
	p, err := newGitLabProvider("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := interface{}(p).(Rater); !ok {
		t.Error("*gitLabProvider should implement Rater")
	}
}
