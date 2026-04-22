package filter

import (
	"testing"

	"github.com/nullcroft/helmseed/internal/provider"
)

func TestByPrefix(t *testing.T) {
	repos := []provider.Repo{
		{Name: "chart-nginx", CloneURL: "git@github.com:org/chart-nginx.git", DefaultBranch: "main"},
		{Name: "chart-redis", CloneURL: "git@github.com:org/chart-redis.git", DefaultBranch: "main"},
		{Name: "app-backend", CloneURL: "git@github.com:org/app-backend.git", DefaultBranch: "main"},
		{Name: "chart-postgres", CloneURL: "git@github.com:org/chart-postgres.git", DefaultBranch: "develop"},
	}

	tests := []struct {
		name   string
		prefix string
		want   int
	}{
		{"empty prefix returns all", "", 4},
		{"matching prefix filters", "chart-", 3},
		{"no matches returns empty", "zzz-", 0},
		{"exact name match", "app-backend", 1},
		{"partial prefix", "chart-r", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ByPrefix(repos, tt.prefix)
			if len(got) != tt.want {
				t.Errorf("ByPrefix(prefix=%q) returned %d repos, want %d", tt.prefix, len(got), tt.want)
			}
		})
	}
}

func TestByPrefix_EmptySlice(t *testing.T) {
	got := ByPrefix(nil, "chart-")
	if len(got) != 0 {
		t.Errorf("ByPrefix on nil slice returned %d repos, want 0", len(got))
	}
}

func TestByPrefix_PreservesOrder(t *testing.T) {
	repos := []provider.Repo{
		{Name: "chart-b"},
		{Name: "chart-a"},
		{Name: "chart-c"},
	}

	got := ByPrefix(repos, "chart-")
	if len(got) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(got))
	}
	if got[0].Name != "chart-b" || got[1].Name != "chart-a" || got[2].Name != "chart-c" {
		t.Errorf("ByPrefix did not preserve order: %v", got)
	}
}
