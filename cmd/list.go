package cmd

import (
	"context"
	"fmt"

	"github.com/nullcroft/helmseed/internal/filter"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:               "list",
	Short:             "List all golden-image chart repositories from the configured git provider",
	PersistentPreRunE: requireConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := provider.New(cfg)
		if err != nil {
			return err
		}

		repos, err := p.ListRepos(context.Background(), cfg.Group)
		if err != nil {
			return err
		}

		repos = filter.ByPrefix(repos, cfg.Prefix)

		fmt.Printf("Found %d repos in %q:\n\n", len(repos), cfg.Group)

		for _, r := range repos {
			fmt.Printf(" %-40s  %s  [%s]\n", r.Name, r.CloneURL, r.DefaultBranch)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
