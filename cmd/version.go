package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = ""

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of helmseed",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "helmseed %s\n", version)
		},
	}
}
