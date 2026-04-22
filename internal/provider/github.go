package provider

import (
	"context"
	"fmt"

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
