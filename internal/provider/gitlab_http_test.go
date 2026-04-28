package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitLabProviderListReposPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/my-group/projects" {
			http.NotFound(w, r)
			return
		}

		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "" || page == "1" {
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"name":"chart-a","ssh_url_to_repo":"git@gitlab.example.com:my-group/chart-a.git","http_url_to_repo":"https://gitlab.example.com/my-group/chart-a.git","default_branch":"main"}]`))
			return
		}
		if page == "2" {
			_, _ = w.Write([]byte(`[{"name":"chart-b","ssh_url_to_repo":"git@gitlab.example.com:my-group/chart-b.git","http_url_to_repo":"https://gitlab.example.com/my-group/chart-b.git","default_branch":"master"}]`))
			return
		}

		http.Error(w, "unexpected page", http.StatusBadRequest)
	}))
	defer server.Close()

	p, err := newGitLabProvider("token", server.URL+"/api/v4")
	if err != nil {
		t.Fatalf("newGitLabProvider returned error: %v", err)
	}

	repos, err := p.ListRepos(context.Background(), "my-group")
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

func TestGitLabProviderListReposError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	p, err := newGitLabProvider("token", server.URL+"/api/v4")
	if err != nil {
		t.Fatalf("newGitLabProvider returned error: %v", err)
	}

	_, err = p.ListRepos(context.Background(), "my-group")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `list projects for group "my-group"`) {
		t.Fatalf("unexpected error %q", err.Error())
	}
}
