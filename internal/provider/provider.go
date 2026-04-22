package provider

import "context"

type Repo struct {
	Name          string
	CloneURL      string
	HTTPSURL      string
	DefaultBranch string
}

type Provider interface {
	ListRepos(ctx context.Context, group string) ([]Repo, error)
}
