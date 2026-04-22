package cmd

import (
	"fmt"
	"os"

	"github.com/nullcroft/helmseed/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "helmseed",
	Short: "Bootstrap helm charts for infrastructure components",
	Long:  `helmseed clones golden-image helm charts from you git provider into .helm/charts, ready to use in your application repo.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./helmseed.yaml)")
}

func requireConfig(cmd *cobra.Command, args []string) error {
	c, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	cfg = c
	return nil
}
