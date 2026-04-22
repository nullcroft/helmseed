package filter

import (
	"strings"

	"github.com/nullcroft/helmseed/internal/provider"
)

// ByPrefix returns only repos whose name starts with the given prefix.
// If prefix is empty, all repos are returned unchanged.
func ByPrefix(repos []provider.Repo, prefix string) []provider.Repo {
	if prefix == "" {
		return repos
	}

	var out []provider.Repo
	for _, r := range repos {
		if strings.HasPrefix(r.Name, prefix) {
			out = append(out, r)
		}
	}
	return out
}
