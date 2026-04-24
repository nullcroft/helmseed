package provider

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type gitLabProvider struct {
	client *gitlab.Client
	baseURL string
}

func newGitLabProvider(token, baseURL string) (*gitLabProvider, error) {
	opts := []gitlab.ClientOptionFunc{}

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

	for {
		projects, resp, err := p.client.Groups.ListGroupProjects(group, opts)
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

	return all, nil
}

