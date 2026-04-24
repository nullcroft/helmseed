package cmd

import (
	"fmt"
	"os"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Force re-fetch all cached charts and update .helm/charts and the lock file",
	Long: `Force re-fetch all locked charts from Chart.lock, overwriting .helm/charts/ contents.
This ignores the cache TTL and always re-clones from the remote.`,
	PersistentPreRunE: requireConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getConfig(cmd.Context())
		ctx := cmd.Context()

		if !IsConfirm() && !cfg.NonInteractive {
			fmt.Printf("This will re-fetch all charts from %s/Chart.lock into %s/charts/ [y/N]? ", cfg.ChartsDir, cfg.ChartsDir)
			var confirm string
			if _, err := fmt.Fscan(os.Stdin, &confirm); err != nil || (confirm != "y" && confirm != "Y") {
				fmt.Println("Aborted.")
				return nil
			}
		}

		opts := cache.BootstrapOptions{
			TTL:       cfg.CacheTTL,
			ChartsDir: cfg.ChartsDir,
			CacheDir:  cfg.CacheDir,
		}
		if err := cache.Update(ctx, opts); err != nil {
			return err
		}

		fmt.Println("Done.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}