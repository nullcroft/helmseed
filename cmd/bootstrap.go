package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nullcroft/helmseed/internal/cache"
	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/nullcroft/helmseed/internal/tui"
	"github.com/spf13/cobra"
)

type bootstrapFlags struct {
	local  bool
	remote bool
	all    bool
}

func newBootstrapCmd(flags *commandFlags, deps Dependencies, requireConfig func(*cobra.Command, []string) error) *cobra.Command {
	bf := &bootstrapFlags{}

	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Interactively pick repos from the configured git provider and clone them to the .helm directory",
		Long: `Interactively select repos from your git provider and bootstrap them into .helm/charts.
Use --all to select all repos without interaction, or --dry-run to preview.`,
		PersistentPreRunE: requireConfig,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBootstrap(cmd, flags, bf, deps)
		},
	}

	bootstrapCmd.Flags().BoolVar(&bf.local, "local", false, "use local file:// paths in Chart.lock")
	bootstrapCmd.Flags().BoolVar(&bf.remote, "remote", false, "use remote repository URLs in Chart.lock (default)")
	bootstrapCmd.Flags().BoolVarP(&bf.all, "all", "a", false, "select all matching repos and skip interactive selection")
	bootstrapCmd.Flags().SortFlags = false

	return bootstrapCmd
}

func runBootstrap(cmd *cobra.Command, flags *commandFlags, bf *bootstrapFlags, deps Dependencies) error {
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return err
	}
	mode, err := resolveBootstrapMode(bf.local, bf.remote)
	if err != nil {
		return err
	}
	p, err := deps.NewProvider(cfg)
	if err != nil {
		return err
	}
	if err := warnIfRateLimited(cmd.Context(), p, string(cfg.Provider), cmd.ErrOrStderr()); err != nil {
		return err
	}

	repos, err := discoverRepos(cmd.Context(), p, cfg)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(repos) == 0 {
		printNoRepos(cmd, cfg)
		return nil
	}
	_, _ = fmt.Fprintf(out, "Found %d repos in %s\n", len(repos), cfg.Group)

	selected, aborted, err := chooseRepos(repos, bf.all, deps.SelectRepos)
	if err != nil {
		return err
	}
	if aborted {
		_, _ = fmt.Fprintln(out, "Aborted.")
		return nil
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
		printBootstrapDryRun(out, selected, displayChartsDir)
		return nil
	}
	if !confirmBootstrap(cmd, len(selected), displayChartsDir, flags, cfg) {
		_, _ = fmt.Fprintln(out, "Aborted.")
		return nil
	}
	_, _ = fmt.Fprintf(out, "Bootstrapping %d repo(s) into %s/charts/\n", len(selected), displayChartsDir)
	if err := deps.Bootstrap(cmd.Context(), selected, opts); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Done.")
	return nil
}

func resolveBootstrapMode(local, remote bool) (cache.RepoRefMode, error) {
	if local && remote {
		return 0, fmt.Errorf("--local and --remote are mutually exclusive")
	}
	if local {
		return cache.LocalRef, nil
	}
	return cache.RemoteRef, nil
}

// warnIfRateLimited prints a warning when CheckRateLimit reports a
// RateLimitError; any other error is propagated to the caller.
func warnIfRateLimited(ctx context.Context, p provider.Provider, providerName string, errOut io.Writer) error {
	rater, ok := p.(provider.Rater)
	if !ok {
		return nil
	}
	err := provider.CheckRateLimit(ctx, rater, providerName, 20)
	if err == nil {
		return nil
	}
	var rateErr *provider.RateLimitError
	if errors.As(err, &rateErr) {
		_, _ = fmt.Fprintf(errOut, "Warning: %v\n", err)
		return nil
	}
	return err
}

func discoverRepos(ctx context.Context, p provider.Provider, cfg *config.Config) ([]provider.Repo, error) {
	repos, err := p.ListRepos(ctx, cfg.Group)
	if err != nil {
		return nil, handleProviderError(err)
	}
	return filterByPrefix(repos, cfg.Prefix), nil
}

// chooseRepos returns the user's selection. The aborted flag distinguishes
// a user-cancelled selection from a real error so the caller can print the
// "Aborted." message without leaking the sentinel error upward.
func chooseRepos(repos []provider.Repo, all bool, selectFn func([]provider.Repo) ([]provider.Repo, error)) ([]provider.Repo, bool, error) {
	if all {
		return repos, false, nil
	}
	selected, err := selectFn(repos)
	if errors.Is(err, tui.ErrAborted) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return selected, false, nil
}

func printBootstrapDryRun(out io.Writer, selected []provider.Repo, displayChartsDir string) {
	_, _ = fmt.Fprintf(out, "Would bootstrap %d repo(s) into %s/charts/:\n", len(selected), displayChartsDir)
	for _, r := range selected {
		_, _ = fmt.Fprintf(out, "  - %s\n", r.Name)
	}
}

func confirmBootstrap(cmd *cobra.Command, count int, displayChartsDir string, flags *commandFlags, cfg *config.Config) bool {
	if flags.yes || cfg.NonInteractive {
		return true
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bootstrap %d repo(s) into %s/charts/ [y/N]? ", count, displayChartsDir)
	return confirmYes(cmd.InOrStdin())
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
