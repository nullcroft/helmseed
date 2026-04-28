package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
)

func TestList_PrintsRepos(t *testing.T) {
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: []provider.Repo{
				{Name: "chart-a", DefaultBranch: "main"},
				{Name: "chart-b", DefaultBranch: "release"},
			}}, nil
		},
	}
	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Found 2 repos in \"my-org\"") {
		t.Errorf("missing header in output: %q", got)
	}
	if !strings.Contains(got, "chart-a") || !strings.Contains(got, "[main]") {
		t.Errorf("missing repo a entry: %q", got)
	}
	if !strings.Contains(got, "chart-b") || !strings.Contains(got, "[release]") {
		t.Errorf("missing repo b entry: %q", got)
	}
}

func TestList_FilterByPrefix(t *testing.T) {
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org", Prefix: "chart-"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: []provider.Repo{
				{Name: "chart-keep", DefaultBranch: "main"},
				{Name: "infra-skip", DefaultBranch: "main"},
			}}, nil
		},
	}
	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Found 1 repos") {
		t.Errorf("expected one filtered repo, got: %q", got)
	}
	if strings.Contains(got, "infra-skip") {
		t.Errorf("non-matching repo leaked into output: %q", got)
	}
}

func TestList_NoReposFound(t *testing.T) {
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: nil}, nil
		},
	}
	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "No repos found") {
		t.Errorf("expected no-repos message, got %q", out.String())
	}
}

func TestList_ProviderError(t *testing.T) {
	wantErr := errors.New("upstream down")
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{err: wantErr}, nil
		},
	}
	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped upstream error, got %v", err)
	}
}

func TestList_NewProviderError(t *testing.T) {
	wantErr := errors.New("new provider boom")
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return nil, wantErr
		},
	}
	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("expected NewProvider error, got %v", err)
	}
}

func TestList_LoadConfigError(t *testing.T) {
	wantErr := errors.New("bad config")
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return nil, wantErr
		},
	}
	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("expected config load error, got %v", err)
	}
}
