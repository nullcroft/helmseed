package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/nullcroft/helmseed/internal/tui"
	"github.com/spf13/cobra"
)

func newBootstrapCmd(flags *commandFlags, deps Dependencies, requireConfig func(*cobra.Command, []string) error) *cobra.Command {
	var (
		flagLocal  bool
		flagRemote bool
		flagAll    bool
	)

	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Interactively pick repos from the configured git provider and clone them to the .helm directory",
		Long: `Interactively select repos from your git provider and bootstrap them into .helm/charts.
Use --all to select all repos without interaction, or --dry-run to preview.`,
		PersistentPreRunE: requireConfig,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromContext(cmd.Context())
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			if flagLocal && flagRemote {
				return fmt.Errorf("--local and --remote are mutually exclusive")
			}

			mode := cache.RemoteRef
			if flagLocal {
				mode = cache.LocalRef
			}

			p, err := deps.NewProvider(cfg)
			if err != nil {
				return err
			}

			if rater, ok := p.(provider.Rater); ok {
				if err := provider.CheckRateLimit(ctx, rater, string(cfg.Provider), 20); err != nil {
					var rateErr *provider.RateLimitError
					if errors.As(err, &rateErr) {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", err)
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
				printNoRepos(cmd, cfg)
				return nil
			}

			_, _ = fmt.Fprintf(out, "Found %d repos in %s\n", len(repos), cfg.Group)

			var selected []provider.Repo
			if flagAll {
				selected = repos
			} else {
				selected, err = deps.SelectRepos(repos)
				if err != nil {
					if errors.Is(err, tui.ErrAborted) {
						_, _ = fmt.Fprintln(out, "Aborted.")
						return nil
					}
					return err
				}
			}

			if len(selected) == 0 {
				_, _ = fmt.Fprintln(out, "No repos selected.")
				return nil
			}

			opts := cache.BootstrapOptions{
				TTL:       cfg.CacheTTL,
				Mode:      mode,
				Prefix:    cfg.Prefix,
				ChartsDir: cfg.ChartsDir,
				CacheDir:  cfg.CacheDir,
				ChartName: cfg.ChartName,
				ChartDesc: cfg.ChartDescription,
				Out:       out,
				Quiet:     flags.quiet,
			}

			displayChartsDir := chartsDirForDisplay(opts.ChartsDir)
			if flags.dryRun {
				_, _ = fmt.Fprintf(out, "Would bootstrap %d repo(s) into %s/charts/:\n", len(selected), displayChartsDir)
				for _, r := range selected {
					_, _ = fmt.Fprintf(out, "  - %s\n", r.Name)
				}
				return nil
			}

			if !flags.yes && !cfg.NonInteractive {
				_, _ = fmt.Fprintf(out, "Bootstrap %d repo(s) into %s/charts/ [y/N]? ", len(selected), displayChartsDir)
				if !confirmYes(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, "Aborted.")
					return nil
				}
			}

			_, _ = fmt.Fprintf(out, "Bootstrapping %d repo(s) into %s/charts/\n", len(selected), displayChartsDir)
			if err := deps.Bootstrap(ctx, selected, opts); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(out, "Done.")
			return nil
		},
	}

	bootstrapCmd.Flags().BoolVar(&flagLocal, "local", false, "use local file:// paths in Chart.lock")
	bootstrapCmd.Flags().BoolVar(&flagRemote, "remote", false, "use remote repository URLs in Chart.lock (default)")
	bootstrapCmd.Flags().BoolVarP(&flagAll, "all", "a", false, "select all matching repos and skip interactive selection")
	bootstrapCmd.Flags().SortFlags = false

	return bootstrapCmd
}

func confirmYes(in io.Reader) bool {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.TrimSpace(line)
	return answer == "y" || answer == "Y"
}

func handleProviderError(err error) error {
	var rateErr *provider.RateLimitError
	if errors.As(err, &rateErr) {
		return fmt.Errorf("%v\nHint: Add a token to increase rate limits, or wait until %s", err, rateErr.Reset.Format(time.RFC3339))
	}
	return err
}
