package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
)

type probeFailingProvider struct {
	staticProvider
}

func (probeFailingProvider) RateLimit(context.Context) (int, int, time.Time, error) {
	return 0, 0, time.Time{}, errors.New("headers unavailable")
}

func TestBootstrap_LocalAndRemoteMutuallyExclusive(t *testing.T) {
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{}, nil
		},
	}
	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--local", "--remote"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive error, got %v", err)
	}
}

func TestBootstrap_LocalFlagPropagatesMode(t *testing.T) {
	var capturedMode cache.RepoRefMode
	var bootstrapCalled bool
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: []provider.Repo{{Name: "repo-a", DefaultBranch: "main"}}}, nil
		},
		Bootstrap: func(_ context.Context, _ []provider.Repo, opts cache.BootstrapOptions) error {
			capturedMode = opts.Mode
			bootstrapCalled = true
			return nil
		},
	}
	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(bytes.NewBufferString("y\n"))
	cmd.SetArgs([]string{"bootstrap", "--all", "--local"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bootstrapCalled {
		t.Fatal("bootstrap should be called")
	}
	if capturedMode != cache.LocalRef {
		t.Fatalf("mode = %v, want LocalRef", capturedMode)
	}
}

func TestBootstrap_NoSelectionExits(t *testing.T) {
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: []provider.Repo{{Name: "repo-a", DefaultBranch: "main"}}}, nil
		},
		SelectRepos: func([]provider.Repo) ([]provider.Repo, error) {
			return nil, nil
		},
		Bootstrap: func(context.Context, []provider.Repo, cache.BootstrapOptions) error {
			t.Fatal("bootstrap should not be called when nothing is selected")
			return nil
		},
	}
	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "No repos selected") {
		t.Fatalf("expected 'No repos selected', got %q", out.String())
	}
}

func TestBootstrap_NoReposFound(t *testing.T) {
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org", Prefix: "chart-"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: []provider.Repo{{Name: "infra-db", DefaultBranch: "main"}}}, nil
		},
	}
	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "matching prefix") {
		t.Fatalf("expected prefix-aware no-repos message, got %q", out.String())
	}
}

func TestBootstrap_ProviderListError(t *testing.T) {
	wantErr := errors.New("upstream boom")
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
	cmd.SetArgs([]string{"bootstrap"})

	err := cmd.Execute()
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped upstream error, got %v", err)
	}
}

func TestWarnIfRateLimitedProbeFailureWarnsOnly(t *testing.T) {
	var errOut bytes.Buffer
	p := probeFailingProvider{}

	if err := warnIfRateLimited(context.Background(), p, "gitlab", &errOut); err != nil {
		t.Fatalf("warnIfRateLimited should not block on probe failure: %v", err)
	}
	if !strings.Contains(errOut.String(), "Warning:") || !strings.Contains(errOut.String(), "headers unavailable") {
		t.Fatalf("expected warning output, got %q", errOut.String())
	}
}

func TestConfirmYes(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", false},
		{"n\n", false},
		{"\n", false},
		{"", false},
		{"  y  \n", true},
	}
	for _, c := range cases {
		got := confirmYes(strings.NewReader(c.input))
		if got != c.want {
			t.Errorf("confirmYes(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}
