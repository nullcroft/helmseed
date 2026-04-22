package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v71/github"
)

type gitHubProvider struct {
	client *github.Client
}

func newGitHubProvider(token string) *gitHubProvider {
	client := github.NewClient(nil)
	if token != "" {
		client = client.WithAuthToken(token)
	}

	return &gitHubProvider{client: client}
}

func (p *gitHubProvider) ListRepos(ctx context.Context, group string) ([]Repo, error) {
	var all []Repo

	opts := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := p.client.Repositories.ListByOrg(ctx, group, opts)

		if err != nil {
			if isRateLimitError(err) {
				return nil, fmt.Errorf("github: rate limit exceeded - try adding a token or wait before retrying: %w", err)
			}
			return nil, fmt.Errorf("github: list repos for org %q: %w", group, err)
		}

		for _, r := range repos {
			all = append(all, Repo{
				Name:          r.GetName(),
				CloneURL:      r.GetSSHURL(),
				HTTPSURL:      r.GetCloneURL(),
				DefaultBranch: r.GetDefaultBranch(),
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return all, nil
}

func isRateLimitError(err error) bool {
	ue, ok := err.(*github.ErrorResponse)
	if !ok {
		return false
	}
	for _, e := range ue.Errors {
		if e.Message == "API rate limit exceeded" || e.Message == "You have exceeded a rate limit" {
			return true
		}
	}
	return ue.Response != nil && ue.Response.StatusCode == http.StatusTooManyRequests
}

func (p *gitHubProvider) RateLimit(ctx context.Context) (int, int, time.Time, error) {
	rate, _, err := p.client.RateLimit.Get(ctx)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("github: get rate limit: %w", err)
	}
	limit := rate.Core.Limit
	remaining := rate.Core.Remaining
	reset := rate.Core.Reset.Time
	return limit, remaining, reset, nil
}
