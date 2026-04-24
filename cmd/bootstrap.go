package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/nullcroft/helmseed/internal/tui"
	"github.com/spf13/cobra"
)

var (
	flagLocal  bool
	flagRemote bool
	flagAll   bool
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Interactively pick repos from the configured git provider and clone them to the .helm directory",
	Long: `Interactively select repos from your git provider and bootstrap them into .helm/charts.
Use --all to select all repos without interaction, or --dry-run to preview.`,
	PersistentPreRunE: requireConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getConfig(cmd.Context())
		ctx := cmd.Context()

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

		if rater, ok := p.(provider.Rater); ok {
			err := provider.CheckRateLimit(ctx, rater, string(cfg.Provider), 20)
			if err != nil {
				var rateErr *provider.RateLimitError
				if errors.As(err, &rateErr) {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Warning: %v\n", err)
				} else {
					return err
				}
			}
		}

		repos, err := p.ListRepos(ctx, cfg.Group)
		if err != nil {
			return handleProviderError(err)
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

		fmt.Printf("Found %d repos in %s\n", len(repos), cfg.Group)

		var selected []provider.Repo
		if flagAll {
			selected = repos
		} else {
			sel, err := tui.Select(repos)
			if err != nil {
				if errors.Is(err, tui.ErrAborted) {
					fmt.Println("Aborted.")
					return nil
				}
				return err
			}
			selected = sel
		}

		if len(selected) == 0 {
			fmt.Println("No repos selected.")
			return nil
		}

		opts := cache.BootstrapOptions{
			TTL:        cfg.CacheTTL,
			Mode:       mode,
			Prefix:     cfg.Prefix,
			ChartsDir:  cfg.ChartsDir,
			CacheDir:   cfg.CacheDir,
			ChartName:  cfg.ChartName,
			ChartDesc: cfg.ChartDescription,
		}

		if flagDryRun {
			fmt.Printf("Would bootstrap %d repo(s) into %s/charts/:\n", len(selected), opts.ChartsDir)
			for _, r := range selected {
				fmt.Printf("  - %s\n", r.Name)
			}
			return nil
		}

		if !IsConfirm() && !cfg.NonInteractive {
			fmt.Printf("Bootstrap %d repo(s) into %s/charts/ [y/N]? ", len(selected), opts.ChartsDir)
			var confirm string
			if _, err := fmt.Fscan(os.Stdin, &confirm); err != nil || (confirm != "y" && confirm != "Y") {
				fmt.Println("Aborted.")
				return nil
			}
		}

		fmt.Printf("Bootstrapping %d repo(s) into %s/charts/\n", len(selected), opts.ChartsDir)
		if err := cache.Bootstrap(ctx, selected, opts); err != nil {
			return err
		}

		fmt.Println("Done.")
		return nil
	},
}

func init() {
	bootstrapCmd.Flags().BoolVar(&flagLocal, "local", false, "use local file:// paths in Chart.lock")
	bootstrapCmd.Flags().BoolVar(&flagRemote, "remote", false, "use remote repository URLs in Chart.lock (default)")
	bootstrapCmd.Flags().BoolVarP(&flagAll, "all", "a", false, "select all matching repos and skip interactive selection")
	bootstrapCmd.Flags().SortFlags = false
	rootCmd.AddCommand(bootstrapCmd)
}

func handleProviderError(err error) error {
	var rateErr *provider.RateLimitError
	if errors.As(err, &rateErr) {
		return fmt.Errorf("%v\nHint: Add a token to increase rate limits, or wait until %s", err, rateErr.Reset.Format(time.RFC3339))
	}
	return err
}