package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type contextKey string

const configContextKey contextKey = "config"

var (
	cfgFile    string
	flagYes   bool
	flagDryRun bool
	verbose   bool
	quiet     bool
)

var rootCmd = &cobra.Command{
	Use:   "helmseed",
	Short: "Bootstrap helm charts for infrastructure components",
	Long:  `helmseed clones golden-image helm charts from your git provider into .helm/charts, ready to use in your application repo.`,
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./helmseed.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "confirm destructive operations")
	rootCmd.PersistentFlags().BoolVarP(&flagDryRun, "dry-run", "d", false, "show what would be done without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")
	_ = viper.BindPFlag("HELMSEED_CONFIG", rootCmd.PersistentFlags().Lookup("config"))
}

func IsVerbose() bool  { return verbose }
func IsQuiet() bool   { return quiet }
func IsConfirm() bool { return flagYes }
func IsDryRun() bool  { return flagDryRun }

func requireConfig(cmd *cobra.Command, args []string) error {
	c, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	cmd.SetContext(context.WithValue(cmd.Context(), configContextKey, c))
	return nil
}

func getConfig(ctx context.Context) *config.Config {
	return ctx.Value(configContextKey).(*config.Config)
}

func filterByPrefix(repos []provider.Repo, prefix string) []provider.Repo {
	if prefix == "" {
		return repos
	}
	var out []provider.Repo
	for _, r := range repos {
		if strings.HasPrefix(r.Name, prefix) {
			out = append(out, r)
		}
	}
	return out
}
