package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of helmseed",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("helmseed v%s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
