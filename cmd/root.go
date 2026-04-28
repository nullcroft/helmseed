package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/nullcroft/helmseed/internal/tui"
	"github.com/spf13/cobra"
)

type contextKey string

const configContextKey contextKey = "config"

type commandFlags struct {
	cfgFile string
	yes     bool
	dryRun  bool
	quiet   bool
	verbose bool
}

type Dependencies struct {
	LoadConfig  func(cfgFile string) (*config.Config, error)
	NewProvider func(cfg *config.Config) (provider.Provider, error)
	SelectRepos func(repos []provider.Repo) ([]provider.Repo, error)
	Bootstrap   func(ctx context.Context, repos []provider.Repo, opts cache.BootstrapOptions) error
	Update      func(ctx context.Context, opts cache.BootstrapOptions) error
}

func (d Dependencies) withDefaults() Dependencies {
	if d.LoadConfig == nil {
		d.LoadConfig = config.Load
	}
	if d.NewProvider == nil {
		d.NewProvider = provider.New
	}
	if d.SelectRepos == nil {
		d.SelectRepos = tui.Select
	}
	if d.Bootstrap == nil {
		d.Bootstrap = cache.Bootstrap
	}
	if d.Update == nil {
		d.Update = cache.Update
	}
	return d
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	deps = deps.withDefaults()
	flags := &commandFlags{}

	rootCmd := &cobra.Command{
		Use:   "helmseed",
		Short: "Bootstrap helm charts for infrastructure components",
		Long:  `helmseed clones golden-image helm charts from your git provider into .helm/charts, ready to use in your application repo.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			configureLogger(flags.verbose, cmd.ErrOrStderr())
		},
	}

	rootCmd.PersistentFlags().StringVarP(&flags.cfgFile, "config", "c", "", "config file (default: ./helmseed.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&flags.yes, "yes", "y", false, "confirm destructive operations")
	rootCmd.PersistentFlags().BoolVarP(&flags.dryRun, "dry-run", "d", false, "show what would be done without executing")
	rootCmd.PersistentFlags().BoolVarP(&flags.quiet, "quiet", "q", false, "suppress non-essential output")
	rootCmd.PersistentFlags().BoolVarP(&flags.verbose, "verbose", "v", false, "enable debug-level logging to stderr")

	requireConfig := func(cmd *cobra.Command, args []string) error {
		cfg, err := deps.LoadConfig(flags.cfgFile)
		if err != nil {
			return err
		}
		cmd.SetContext(context.WithValue(cmd.Context(), configContextKey, cfg))
		return nil
	}

	rootCmd.AddCommand(
		newBootstrapCmd(flags, deps, requireConfig),
		newUpdateCmd(flags, deps, requireConfig),
		newListCmd(deps, requireConfig),
		newVersionCmd(),
	)

	return rootCmd
}

var rootCmd = NewRootCommand(Dependencies{})

func Execute() error {
	return rootCmd.Execute()
}

func configureLogger(verbose bool, w io.Writer) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	out := w
	if out == nil {
		out = os.Stderr
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})))
}

func configFromContext(ctx context.Context) (*config.Config, error) {
	cfg, ok := ctx.Value(configContextKey).(*config.Config)
	if !ok || cfg == nil {
		return nil, errors.New("internal error: missing config in command context")
	}
	return cfg, nil
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

func chartsDirForDisplay(chartsDir string) string {
	if chartsDir == "" {
		return ".helm"
	}
	return chartsDir
}

func printNoRepos(cmd *cobra.Command, cfg *config.Config) {
	if cfg.Prefix != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No repos found in %s matching prefix %q.\n", cfg.Group, cfg.Prefix)
		return
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No repos found in %s.\n", cfg.Group)
}
