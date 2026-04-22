package cmd

import (
	"fmt"

	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all golden-image chart repositories from the configured git provider",
	Long: `List all repositories in the configured org/group.
Filter by prefix if configured.`,
	PersistentPreRunE: requireConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getConfig(cmd.Context())
		ctx := cmd.Context()

		p, err := provider.New(cfg)
		if err != nil {
			return err
		}

		repos, err := p.ListRepos(ctx, cfg.Group)
		if err != nil {
			return err
		}

		repos = filterByPrefix(repos, cfg.Prefix)

		if len(repos) == 0 {
			if cfg.Prefix != "" {
				fmt.Printf("No repos found in %s matching prefix %q.\n", cfg.Group, cfg.Prefix)
			} else {
				fmt.Printf("No repos found in %s.\n", cfg.Group)
			}
			return nil
		}

		fmt.Printf("Found %d repos in %q:\n\n", len(repos), cfg.Group)

		for _, r := range repos {
			fmt.Printf(" %-40s  [%s]\n", r.Name, r.DefaultBranch)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}