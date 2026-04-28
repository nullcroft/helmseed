package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd(deps Dependencies, requireConfig func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all golden-image chart repositories from the configured git provider",
		Long: `List all repositories in the configured org/group.
Filter by prefix if configured.`,
		PersistentPreRunE: requireConfig,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromContext(cmd.Context())
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			p, err := deps.NewProvider(cfg)
			if err != nil {
				return err
			}

			repos, err := p.ListRepos(ctx, cfg.Group)
			if err != nil {
				return handleProviderError(err)
			}

			repos = filterByPrefix(repos, cfg.Prefix)
			if len(repos) == 0 {
				printNoRepos(cmd, cfg)
				return nil
			}

			_, _ = fmt.Fprintf(out, "Found %d repos in %q:\n\n", len(repos), cfg.Group)
			for _, r := range repos {
				_, _ = fmt.Fprintf(out, " %-40s  [%s]\n", r.Name, r.DefaultBranch)
			}

			return nil
		},
	}
}
