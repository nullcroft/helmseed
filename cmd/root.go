package cmd

import (
	"context"
	"strings"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type contextKey string

const configContextKey contextKey = "config"

var (
	cfgFile    string
	flagYes    bool
	flagDryRun bool
	quiet      bool
)

var rootCmd = &cobra.Command{
	Use:   "helmseed",
	Short: "Bootstrap helm charts for infrastructure components",
	Long:  `helmseed clones golden-image helm charts from your git provider into .helm/charts, ready to use in your application repo.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./helmseed.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "confirm destructive operations")
	rootCmd.PersistentFlags().BoolVarP(&flagDryRun, "dry-run", "d", false, "show what would be done without executing")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")
	_ = viper.BindPFlag("HELMSEED_CONFIG", rootCmd.PersistentFlags().Lookup("config"))
}

func IsQuiet() bool   { return quiet }
func IsConfirm() bool { return flagYes }

func requireConfig(cmd *cobra.Command, args []string) error {
	cache.SetQuiet(quiet)

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
