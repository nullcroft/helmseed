package provider

import (
	"testing"

	"github.com/nullcroft/helmseed/internal/config"
)

func TestNew_GitHub(t *testing.T) {
	p, err := New(&config.Config{Provider: config.ProviderGitHub})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*gitHubProvider); !ok {
		t.Errorf("expected *gitHubProvider, got %T", p)
	}
}

func TestNew_GitLab(t *testing.T) {
	p, err := New(&config.Config{Provider: config.ProviderGitLab})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*gitLabProvider); !ok {
		t.Errorf("expected *gitLabProvider, got %T", p)
	}
}

func TestNew_Unknown(t *testing.T) {
	_, err := New(&config.Config{Provider: "bitbucket"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
