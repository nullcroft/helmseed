package cmd

import (
	"context"
	"fmt"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/filter"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/nullcroft/helmseed/internal/tui"
	"github.com/spf13/cobra"
)

var (
	flagLocal  bool
	flagRemote bool
)

var bootstrapCmd = &cobra.Command{
	Use:               "bootstrap",
	Short:             "Interactively pick repos from the configured git provider and clone them to the .helm directory",
	PersistentPreRunE: requireConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagLocal && flagRemote {
			return fmt.Errorf("--local and --remote are mutually exclusive")
		}

		mode := cache.RemoteRef
		if flagLocal {
			mode = cache.LocalRef
		}

		p, err := provider.New(cfg)
		if err != nil {
			return err
		}

		repos, err := p.ListRepos(context.Background(), cfg.Group)
		if err != nil {
			return err
		}

		repos = filter.ByPrefix(repos, cfg.Prefix)

		if len(repos) == 0 {
			fmt.Println("No repos found after filtering.")
			return nil
		}

		selected, err := tui.Select(repos)
		if err != nil {
			return err
		}

		if selected == nil {
			fmt.Println("Aborted.")
			return nil
		}

		if len(selected) == 0 {
			fmt.Println("No repos selected.")
			return nil
		}

		opts := cache.BootstrapOptions{
			TTL:    cfg.CacheTTL,
			Mode:   mode,
			Prefix: cfg.Prefix,
		}

		fmt.Printf("\nBootstrapping %d repo(s) into .helm/charts/\n", len(selected))
		if err := cache.Bootstrap(cmd.Context(), selected, opts); err != nil {
			return err
		}

		fmt.Println("Done.")
		return nil
	},
}

func init() {
	bootstrapCmd.Flags().BoolVar(&flagLocal, "local", false, "use local file:// paths in Chart.lock")
	bootstrapCmd.Flags().BoolVar(&flagRemote, "remote", false, "use remote repository URLs in Chart.lock (default)")
	rootCmd.AddCommand(bootstrapCmd)
}
