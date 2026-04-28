package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v71/github"
)

func newTestGitHubProvider(t *testing.T, handler http.Handler) *gitHubProvider {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	client.BaseURL = baseURL

	return &gitHubProvider{client: client}
}

func TestGitHubProviderListReposPagination(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/orgs/my-org/repos") {
			http.NotFound(w, r)
			return
		}

		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Link", fmt.Sprintf("<%s/orgs/my-org/repos?page=2>; rel=\"next\"", "http://example.test"))
			_, _ = w.Write([]byte(`[{"name":"chart-a","ssh_url":"git@github.com:my-org/chart-a.git","clone_url":"https://github.com/my-org/chart-a.git","default_branch":"main"}]`))
			return
		}

		if page == "2" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"chart-b","ssh_url":"git@github.com:my-org/chart-b.git","clone_url":"https://github.com/my-org/chart-b.git","default_branch":"master"}]`))
			return
		}

		http.Error(w, "unexpected page", http.StatusBadRequest)
	})

	p := newTestGitHubProvider(t, handler)

	repos, err := p.ListRepos(context.Background(), "my-org")
	if err != nil {
		t.Fatalf("ListRepos returned error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].Name != "chart-a" || repos[0].DefaultBranch != "main" {
		t.Fatalf("unexpected first repo: %+v", repos[0])
	}
	if repos[1].Name != "chart-b" || repos[1].DefaultBranch != "master" {
		t.Fatalf("unexpected second repo: %+v", repos[1])
	}
}

func TestGitHubProviderListReposRateLimit(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	})

	p := newTestGitHubProvider(t, handler)

	_, err := p.ListRepos(context.Background(), "my-org")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("expected rate-limit error, got %q", err.Error())
	}
}

func TestGitHubProviderRateLimitEndpoint(t *testing.T) {
	now := time.Now().UTC().Unix() + 60
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rate_limit" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"resources":{"core":{"limit":60,"remaining":12,"reset":%d}}}`, now)
	})

	p := newTestGitHubProvider(t, handler)

	limit, remaining, reset, err := p.RateLimit(context.Background())
	if err != nil {
		t.Fatalf("RateLimit returned error: %v", err)
	}
	if limit != 60 || remaining != 12 {
		t.Fatalf("unexpected limits: limit=%d remaining=%d", limit, remaining)
	}
	if reset.Unix() != now {
		t.Fatalf("unexpected reset time: %v", reset)
	}
}
