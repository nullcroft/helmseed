package cmd

import (
	"fmt"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:               "update",
	Short:             "Force re-fetch all cached charts and update .helm/charts and the lock file",
	PersistentPreRunE: requireConfig,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Updating charts ...")
		if err := cache.Update(cmd.Context(), cfg.CacheTTL); err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
