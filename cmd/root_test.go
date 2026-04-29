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

type staticProvider struct {
	repos []provider.Repo
	err   error
}

func (s staticProvider) ListRepos(context.Context, string) ([]provider.Repo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.repos, nil
}

func TestNewRootCommand_Help(t *testing.T) {
	cmd := NewRootCommand(Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
	})
	cmd.SetArgs([]string{})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
}

func TestFilterByPrefix(t *testing.T) {
	repos := []provider.Repo{
		{Name: "helm-app-frontend"},
		{Name: "helm-app-backend"},
		{Name: "infra-db"},
	}

	if got := filterByPrefix(repos, ""); len(got) != 3 {
		t.Fatalf("expected all repos, got %d", len(got))
	}
	if got := filterByPrefix(repos, "helm-app-"); len(got) != 2 {
		t.Fatalf("expected two prefixed repos, got %d", len(got))
	}
	if got := filterByPrefix(repos, "no-match"); len(got) != 0 {
		t.Fatalf("expected no repos, got %d", len(got))
	}
}

func TestConfigFromContext(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}
	ctx := context.WithValue(context.Background(), configContextKey, cfg)

	got, err := configFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cfg {
		t.Fatalf("unexpected config pointer returned")
	}
}

func TestConfigFromContext_Missing(t *testing.T) {
	if _, err := configFromContext(context.Background()); err == nil {
		t.Fatal("expected error when config is missing in context")
	}
}

func TestHandleProviderError(t *testing.T) {
	plainErr := errors.New("some error")
	if got := handleProviderError(plainErr); got != plainErr {
		t.Fatalf("expected same error, got %v", got)
	}

	reset := time.Now().Add(time.Hour).UTC()
	rateErr := &provider.RateLimitError{
		Provider:  "github",
		Limit:     60,
		Remaining: 0,
		Reset:     reset,
		Cause:     provider.ErrRateExhausted,
	}
	got := handleProviderError(rateErr)
	if got == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(got.Error(), reset.Format(time.RFC3339)) {
		t.Fatalf("expected reset timestamp in error, got %q", got.Error())
	}
}

func TestBootstrapDryRunUsesCommandWriters(t *testing.T) {
	var bootstrapCalled bool
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{
				Provider: config.ProviderGitHub,
				Group:    "my-org",
				Prefix:   "chart-",
			}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{
				repos: []provider.Repo{
					{Name: "chart-nginx", DefaultBranch: "main"},
				},
			}, nil
		},
		SelectRepos: func([]provider.Repo) ([]provider.Repo, error) {
			t.Fatal("SelectRepos should not be called with --all")
			return nil, nil
		},
		Bootstrap: func(context.Context, []provider.Repo, cache.BootstrapOptions) error {
			bootstrapCalled = true
			return nil
		},
	}

	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(bytes.NewBufferString("y\n"))
	cmd.SetArgs([]string{"bootstrap", "--all", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if bootstrapCalled {
		t.Fatal("bootstrap should not run in dry-run mode")
	}
	if !strings.Contains(out.String(), "Would bootstrap 1 repo(s)") {
		t.Fatalf("expected dry-run output, got %q", out.String())
	}
}

func TestBootstrapAbortsWithNegativeConfirmation(t *testing.T) {
	var bootstrapCalled bool
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		NewProvider: func(*config.Config) (provider.Provider, error) {
			return staticProvider{repos: []provider.Repo{{Name: "repo-a", DefaultBranch: "main"}}}, nil
		},
		Bootstrap: func(context.Context, []provider.Repo, cache.BootstrapOptions) error {
			bootstrapCalled = true
			return nil
		},
	}

	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(bytes.NewBufferString("n\n"))
	cmd.SetArgs([]string{"bootstrap", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if bootstrapCalled {
		t.Fatal("bootstrap should not be called on abort")
	}
	if !strings.Contains(out.String(), "Aborted.") {
		t.Fatalf("expected abort output, got %q", out.String())
	}
}

func TestUpdateAbortsWithNegativeConfirmation(t *testing.T) {
	var updateCalled bool
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		Update: func(context.Context, cache.BootstrapOptions) error {
			updateCalled = true
			return nil
		},
	}

	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(bytes.NewBufferString("n\n"))
	cmd.SetArgs([]string{"update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if updateCalled {
		t.Fatal("update should not be called on abort")
	}
	if !strings.Contains(out.String(), "Aborted.") {
		t.Fatalf("expected abort output, got %q", out.String())
	}
}

func TestUpdateDryRunDoesNotMutate(t *testing.T) {
	var updateCalled bool
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		Update: func(context.Context, cache.BootstrapOptions) error {
			updateCalled = true
			return nil
		},
	}

	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"update", "--dry-run", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if updateCalled {
		t.Fatal("update should not be called in dry-run mode")
	}
	if !strings.Contains(out.String(), "Would re-fetch all charts") {
		t.Fatalf("expected dry-run output, got %q", out.String())
	}
}

func TestUpdate_HappyPath(t *testing.T) {
	var capturedOpts cache.BootstrapOptions
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{
				Provider:         config.ProviderGitHub,
				Group:            "my-org",
				ChartName:        "my-app",
				ChartDescription: "umbrella",
				CacheTTL:         24 * time.Hour,
			}, nil
		},
		Update: func(_ context.Context, opts cache.BootstrapOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	cmd := NewRootCommand(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"update", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "Done.") {
		t.Errorf("expected Done message, got %q", out.String())
	}
	if capturedOpts.ChartName != "my-app" || capturedOpts.ChartDesc != "umbrella" {
		t.Errorf("update opts not propagated: %+v", capturedOpts)
	}
}

func TestUpdate_NonInteractiveSkipsPrompt(t *testing.T) {
	called := false
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{
				Provider:       config.ProviderGitHub,
				Group:          "my-org",
				NonInteractive: true,
			}, nil
		},
		Update: func(context.Context, cache.BootstrapOptions) error {
			called = true
			return nil
		},
	}

	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Fatal("update should run when non_interactive is configured")
	}
}

func TestUpdate_PropagatesError(t *testing.T) {
	wantErr := errors.New("cache boom")
	deps := Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
		Update: func(context.Context, cache.BootstrapOptions) error {
			return wantErr
		},
	}

	cmd := NewRootCommand(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"update", "--yes"})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped update error, got %v", err)
	}
}

func TestChartsDirForDisplay(t *testing.T) {
	if got := chartsDirForDisplay(""); got != ".helm" {
		t.Errorf("empty chartsDir = %q, want .helm", got)
	}
	if got := chartsDirForDisplay("custom"); got != "custom" {
		t.Errorf("custom chartsDir = %q, want custom", got)
	}
}

func TestVersionCommandOutput(t *testing.T) {
	version = "v0.1.0-test"

	cmd := NewRootCommand(Dependencies{
		LoadConfig: func(string) (*config.Config, error) {
			return &config.Config{Provider: config.ProviderGitHub, Group: "my-org"}, nil
		},
	})
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !strings.Contains(out.String(), "helmseed v0.1.0-test") {
		t.Fatalf("unexpected version output %q", out.String())
	}
}
