package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type gitLabProvider struct {
	client  *gitlab.Client
	baseURL string
}

func newGitLabProvider(token, baseURL string) (*gitLabProvider, error) {
	opts := []gitlab.ClientOptionFunc{
		gitlab.WithHTTPClient(&http.Client{Timeout: httpClientTimeout}),
	}

	if baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}

	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("gitlab: failed to create client: %w", err)
	}

	return &gitLabProvider{client: client, baseURL: baseURL}, nil
}

func (p *gitLabProvider) ListRepos(ctx context.Context, group string) ([]Repo, error) {
	var all []Repo

	opts := &gitlab.ListGroupProjectsOptions{
		IncludeSubGroups: gitlab.Ptr(true),
		ListOptions:      gitlab.ListOptions{PerPage: 100},
	}

	pages := 0
	for {
		pages++
		slog.Debug("gitlab: list projects page", "group", group, "page", opts.Page)
		projects, resp, err := p.client.Groups.ListGroupProjects(group, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list projects for group %q: %w", group, err)
		}

		for _, p := range projects {
			all = append(all, Repo{
				Name:          p.Name,
				CloneURL:      p.SSHURLToRepo,
				HTTPSURL:      p.HTTPURLToRepo,
				DefaultBranch: p.DefaultBranch,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	slog.Debug("gitlab: list projects done", "group", group, "pages", pages, "projects", len(all))
	return all, nil
}

// RateLimit reads RateLimit-* headers from a lightweight /version call.
// Returns an error if the GitLab instance does not expose those headers
// (some self-hosted deployments disable rate limiting).
func (p *gitLabProvider) RateLimit(ctx context.Context) (int, int, time.Time, error) {
	_, resp, err := p.client.Version.GetVersion(gitlab.WithContext(ctx))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("gitlab: probe rate limit: %w", err)
	}
	if resp == nil || resp.Response == nil {
		return 0, 0, time.Time{}, fmt.Errorf("gitlab: empty response when probing rate limit")
	}
	limit, errL := parseHeaderInt(resp.Header, "RateLimit-Limit")
	remaining, errR := parseHeaderInt(resp.Header, "RateLimit-Remaining")
	resetUnix, errT := parseHeaderInt(resp.Header, "RateLimit-Reset")
	if errL != nil || errR != nil || errT != nil {
		return 0, 0, time.Time{}, fmt.Errorf("gitlab: rate-limit headers not exposed by this instance")
	}
	return limit, remaining, time.Unix(int64(resetUnix), 0), nil
}

func parseHeaderInt(h http.Header, key string) (int, error) {
	v := h.Get(key)
	if v == "" {
		return 0, fmt.Errorf("missing header %s", key)
	}
	return strconv.Atoi(v)
}
