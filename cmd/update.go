package cmd

import (
	"fmt"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/spf13/cobra"
)

func newUpdateCmd(flags *commandFlags, deps Dependencies, requireConfig func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Force re-fetch all cached charts and update .helm/charts and the lock file",
		Long: `Force re-fetch all locked charts from Chart.lock, overwriting .helm/charts/ contents.
This ignores the cache TTL and always re-clones from the remote.`,
		PersistentPreRunE: requireConfig,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromContext(cmd.Context())
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			displayChartsDir := chartsDirForDisplay(cfg.ChartsDir)
			if flags.dryRun {
				_, _ = fmt.Fprintf(out, "Would re-fetch all charts from %s/Chart.lock into %s/charts/\n", displayChartsDir, displayChartsDir)
				return nil
			}
			if !flags.yes && !cfg.NonInteractive {
				_, _ = fmt.Fprintf(out, "This will re-fetch all charts from %s/Chart.lock into %s/charts/ [y/N]? ", displayChartsDir, displayChartsDir)
				if !confirmYes(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, "Aborted.")
					return nil
				}
			}

			opts := cache.BootstrapOptions{
				TTL:       cfg.CacheTTL,
				ChartsDir: cfg.ChartsDir,
				CacheDir:  cfg.CacheDir,
				ChartName: cfg.ChartName,
				ChartDesc: cfg.ChartDescription,
				Out:       out,
				Quiet:     flags.quiet,
			}
			if err := deps.Update(ctx, opts); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(out, "Done.")
			return nil
		},
	}
}
