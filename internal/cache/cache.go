package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nullcroft/helmseed/internal/provider"
	"go.yaml.in/yaml/v3"
)

const (
	defaultChartsDir      = ".helm"
	chartFileName         = "Chart.yaml"
	lockFileName          = "Chart.lock"
	metaFileName          = "meta.json"
	localChartRepoPrefix  = "file://charts/"
	maxWorkers            = 5
	cloneTimeout          = 5 * time.Minute
	errCopyFmt            = "copy %s from cache: %w"
	msgCopiedFromCacheFmt = "%s copied from cache\n"

	cacheDirPerm  os.FileMode = 0o750
	cacheFilePerm os.FileMode = 0o600
)

// readFileInDir opens dir as an os.Root and reads name within it.
// Path access is scoped: symlinks that resolve outside dir are refused
// by the kernel, eliminating the TOCTOU traversal class addressed by
// gosec G304/G122.
func readFileInDir(dir, name string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// writeFileInDir opens dir as an os.Root and writes name within it.
func writeFileInDir(dir, name string, data []byte, perm os.FileMode) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func isValidRepoName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	return !strings.HasPrefix(name, ".")
}

// ChartLock mirrors Helm's Chart.lock structure.
type ChartLock struct {
	Generated    time.Time        `yaml:"generated"`
	Digest       string           `yaml:"digest"`
	Dependencies []LockDependency `yaml:"dependencies"`
}

// LockDependency represents a single pinned dependency in Chart.lock.
type LockDependency struct {
	Name       string `yaml:"name"`
	Repository string `yaml:"repository"`
	Version    string `yaml:"version"`
}

// ChartFile mirrors Helm's Chart.yaml structure (umbrella chart).
type ChartFile struct {
	APIVersion   string            `yaml:"apiVersion"`
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Version      string            `yaml:"version"`
	Type         string            `yaml:"type"`
	Dependencies []ChartDependency `yaml:"dependencies"`
}

// ChartDependency represents a single dependency entry in Chart.yaml.
type ChartDependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
}

type chartMeta struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// Meta holds cache metadata for a single repo entry.
type Meta struct {
	ClonedAt      time.Time `json:"cloned_at"`
	CloneURL      string    `json:"clone_url"`
	HTTPSURL      string    `json:"https_url"`
	DefaultBranch string    `json:"default_branch"`
	Commit        string    `json:"commit"`
}

// RepoRefMode controls how dependencies are referenced in Chart.lock.
type RepoRefMode int

const (
	RemoteRef RepoRefMode = iota
	LocalRef
)

// BootstrapOptions groups configuration for a bootstrap operation.
type BootstrapOptions struct {
	TTL       time.Duration
	Mode      RepoRefMode
	Prefix    string
	ChartsDir string
	CacheDir  string
	ChartName string
	ChartDesc string
	Out       io.Writer
	Quiet     bool
}

func cacheDir(custom string) (string, error) {
	if custom != "" {
		return validateAbsCachePath(custom, "cache_dir")
	}

	if xdgDir, found := os.LookupEnv("XDG_CACHE_HOME"); found && xdgDir != "" {
		base, err := validateAbsCachePath(xdgDir, "XDG_CACHE_HOME")
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "helmseed"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "helmseed"), nil
}

func validateAbsCachePath(path, label string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", label, path)
	}
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("%s must not contain '..', got %q", label, path)
	}
	return cleaned, nil
}

func readMeta(dir string) (Meta, error) {
	data, err := readFileInDir(dir, metaFileName)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func writeMeta(dir string, m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileInDir(dir, metaFileName, data, cacheFilePerm)
}

func isFresh(entryDir string, ttl time.Duration) bool {
	m, err := readMeta(entryDir)
	if err != nil {
		return false
	}
	return time.Since(m.ClonedAt) < ttl
}

// Bootstrap ensures repos are cached (cloning if stale/missing) then copies
// from cache into .helm/charts/. Repos already present in .helm/charts/ are skipped.
func Bootstrap(ctx context.Context, repos []provider.Repo, opts BootstrapOptions) error {
	base, err := cacheDir(opts.CacheDir)
	if err != nil {
		return err
	}
	chartsDir, err := normalizeChartsDir(opts.ChartsDir)
	if err != nil {
		return err
	}
	return bootstrap(ctx, repos, base, chartsDir, opts)
}

func bootstrap(ctx context.Context, repos []provider.Repo, cacheBase, chartsDir string, opts BootstrapOptions) error {
	repos, err := prepareRepos(repos, opts.Prefix)
	if err != nil {
		return err
	}
	if err := setupDirs(cacheBase, chartsDir); err != nil {
		return err
	}

	existing, pending, err := splitExistingRepos(repos, chartsDir)
	if err != nil {
		return err
	}

	cached, stale := partitionRepos(pending, cacheBase, opts.TTL)
	if err := copyRepos(cached, cacheBase, chartsDir, opts); err != nil {
		return err
	}
	if err := cloneAndCopyStale(ctx, stale, cacheBase, chartsDir, opts); err != nil {
		return err
	}

	warnSkippedExisting(existing, opts)
	return writeHelmFiles(repos, chartsDir, opts.Mode, opts.ChartName, opts.ChartDesc)
}

func prepareRepos(repos []provider.Repo, prefix string) ([]provider.Repo, error) {
	stripped := make([]provider.Repo, 0, len(repos))
	seen := make(map[string]struct{}, len(repos))

	for _, r := range repos {
		originalName := r.Name
		if !isValidRepoName(originalName) {
			return nil, fmt.Errorf("invalid repo name %q: must not contain '..', '/', or '\\'", originalName)
		}

		if prefix != "" {
			r.Name = strings.TrimPrefix(r.Name, prefix)
		}

		if r.Name == "" {
			return nil, fmt.Errorf("invalid repo name %q after prefix normalization", originalName)
		}
		if !isValidRepoName(r.Name) {
			return nil, fmt.Errorf("invalid normalized repo name %q from %q", r.Name, originalName)
		}
		if _, exists := seen[r.Name]; exists {
			return nil, fmt.Errorf("duplicate repo name %q after prefix normalization", r.Name)
		}

		seen[r.Name] = struct{}{}
		stripped = append(stripped, r)
	}
	return stripped, nil
}

func setupDirs(cacheBase, chartsDir string) error {
	if err := os.MkdirAll(cacheBase, cacheDirPerm); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(chartsDir, "charts"), cacheDirPerm); err != nil {
		return fmt.Errorf("create charts dir: %w", err)
	}
	return nil
}

func partitionRepos(repos []provider.Repo, cacheBase string, ttl time.Duration) (cached, stale []provider.Repo) {
	for _, r := range repos {
		entry := filepath.Join(cacheBase, r.Name)
		if isFresh(entry, ttl) {
			slog.Debug("cache hit", "repo", r.Name, "entry", entry)
			cached = append(cached, r)
		} else {
			slog.Debug("cache miss", "repo", r.Name, "entry", entry)
			stale = append(stale, r)
		}
	}
	return
}

func splitExistingRepos(repos []provider.Repo, chartsDir string) (existing, missing []provider.Repo, err error) {
	for _, r := range repos {
		dest := filepath.Join(chartsDir, "charts", r.Name)
		_, statErr := os.Stat(dest)
		switch {
		case statErr == nil:
			existing = append(existing, r)
		case os.IsNotExist(statErr):
			missing = append(missing, r)
		default:
			return nil, nil, fmt.Errorf("stat %s: %w", dest, statErr)
		}
	}
	return existing, missing, nil
}

func copyRepos(repos []provider.Repo, cacheBase, chartsDir string, opts BootstrapOptions) error {
	for _, r := range repos {
		dest := filepath.Join(chartsDir, "charts", r.Name)
		if _, err := os.Stat(dest); err == nil {
			logf(opts, "%s already bootstrapped, skipping...\n", r.Name)
			continue
		}
		if err := copyDir(filepath.Join(cacheBase, r.Name), dest); err != nil {
			return fmt.Errorf(errCopyFmt, r.Name, err)
		}
		logf(opts, msgCopiedFromCacheFmt, r.Name)
	}
	return nil
}

func cloneAndCopyStale(ctx context.Context, stale []provider.Repo, cacheBase, chartsDir string, opts BootstrapOptions) error {
	if len(stale) == 0 {
		return nil
	}
	if err := cloneRepos(ctx, stale, cacheBase, opts); err != nil {
		return err
	}
	for _, r := range stale {
		dest := filepath.Join(chartsDir, "charts", r.Name)
		if _, err := os.Stat(dest); err == nil {
			logf(opts, "%s already bootstrapped, skipping...\n", r.Name)
			continue
		}
		if err := copyDir(filepath.Join(cacheBase, r.Name), dest); err != nil {
			return fmt.Errorf(errCopyFmt, r.Name, err)
		}
		logf(opts, msgCopiedFromCacheFmt, r.Name)
	}
	return nil
}

func warnSkippedExisting(existing []provider.Repo, opts BootstrapOptions) {
	if opts.Quiet || len(existing) == 0 {
		return
	}

	names := make([]string, 0, len(existing))
	for _, r := range existing {
		names = append(names, r.Name)
	}
	slices.Sort(names)
	_, _ = fmt.Fprintf(outWriter(opts), "Warning: charts (%s) already exist and were left unchanged, run helmseed update to refresh\n",
		strings.Join(names, ", "))
}

func cloneRepos(ctx context.Context, repos []provider.Repo, base string, opts BootstrapOptions) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, maxWorkers)
	)
	p := NewProgress("cloning", len(repos), outWriter(opts), opts.Quiet)
	p.Start()
	for _, r := range repos {
		entryDir := filepath.Join(base, r.Name)
		wg.Add(1)
		sem <- struct{}{}
		go func(r provider.Repo, entryDir string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := os.RemoveAll(entryDir); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("clean cache entry %s: %w", r.Name, err))
				mu.Unlock()
				return
			}
			if err := cloneOne(ctx, r, entryDir); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("clone %s: %w", r.Name, err))
				if cleanupErr := os.RemoveAll(entryDir); cleanupErr != nil {
					errs = append(errs, fmt.Errorf("cleanup partial cache for %s: %w", r.Name, cleanupErr))
				}
				mu.Unlock()
			} else {
				p.Add()
			}
		}(r, entryDir)
	}
	wg.Wait()
	p.Finish()
	return errors.Join(errs...)
}

func cloneOne(ctx context.Context, r provider.Repo, entryDir string) error {
	return WithRetry(ctx, func(ctx context.Context) error {
		start := time.Now()
		slog.Debug("clone start", "repo", r.Name, "url", r.CloneURL, "branch", r.DefaultBranch)
		cloneCtx, cancel := context.WithTimeout(ctx, cloneTimeout)
		defer cancel()
		cloneOpts := &git.CloneOptions{
			URL:   r.CloneURL,
			Depth: 1,
		}
		if r.DefaultBranch != "" {
			cloneOpts.SingleBranch = true
			cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(r.DefaultBranch)
		}
		repo, err := git.PlainCloneContext(cloneCtx, entryDir, false, cloneOpts)
		if err != nil {
			slog.Debug("clone failed", "repo", r.Name, "duration_ms", time.Since(start).Milliseconds(), "error", err)
			return err
		}
		head, err := repo.Head()
		if err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(entryDir, ".git")); err != nil {
			return err
		}
		defaultBranch := r.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = head.Name().Short()
		}
		if err := writeMeta(entryDir, Meta{
			ClonedAt:      time.Now(),
			CloneURL:      r.CloneURL,
			HTTPSURL:      r.HTTPSURL,
			DefaultBranch: defaultBranch,
			Commit:        head.Hash().String(),
		}); err != nil {
			return err
		}
		slog.Debug("clone done", "repo", r.Name, "commit", head.Hash().String(), "duration_ms", time.Since(start).Milliseconds())
		return nil
	})
}

func readDependencyChartMetadata(dir string) (chartMeta, error) {
	data, err := readFileInDir(dir, chartFileName)
	if err != nil {
		return chartMeta{}, fmt.Errorf("read Chart.yaml: %w", err)
	}
	var cm chartMeta
	if err := yaml.Unmarshal(data, &cm); err != nil {
		return chartMeta{}, fmt.Errorf("parse Chart.yaml: %w", err)
	}
	if cm.Version == "" {
		return chartMeta{}, errors.New("chart.yaml has empty version")
	}
	return cm, nil
}

func readChartVersion(dir string) (string, error) {
	cm, err := readDependencyChartMetadata(dir)
	if err != nil {
		return "", err
	}
	return cm.Version, nil
}

func digestDependencies(deps []LockDependency) string {
	sorted := make([]LockDependency, len(deps))
	copy(sorted, deps)
	slices.SortFunc(sorted, func(a, b LockDependency) int {
		return strings.Compare(a.Name, b.Name)
	})

	h := sha256.New()
	for _, d := range sorted {
		_, _ = fmt.Fprintf(h, "%s|%s|%s\n", d.Name, d.Repository, d.Version)
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func writeHelmFiles(repos []provider.Repo, chartsDir string, mode RepoRefMode, chartName, chartDesc string) error {
	var (
		lockDeps  []LockDependency
		chartDeps []ChartDependency
	)

	chart, err := loadChartMetadata(chartsDir, chartName, chartDesc)
	if err != nil {
		return err
	}

	for _, r := range repos {
		meta, err := readDependencyChartMetadata(filepath.Join(chartsDir, "charts", r.Name))
		if err != nil {
			return fmt.Errorf("read chart version for %s: %w", r.Name, err)
		}
		depName := meta.Name
		if depName == "" {
			depName = r.Name
		}

		repo := strings.TrimSuffix(r.HTTPSURL, ".git")
		if mode == LocalRef {
			repo = localChartRepoPrefix + r.Name
		}

		lockDeps = append(lockDeps, LockDependency{
			Name:       depName,
			Repository: repo,
			Version:    meta.Version,
		})
		chartDeps = append(chartDeps, ChartDependency{
			Name:       depName,
			Version:    meta.Version,
			Repository: repo,
		})
	}

	chart.Dependencies = chartDeps
	chartData, err := yaml.Marshal(chart)
	if err != nil {
		return fmt.Errorf("marshal chart file: %w", err)
	}
	if err := writeFileInDir(chartsDir, chartFileName, chartData, cacheFilePerm); err != nil {
		return fmt.Errorf("write chart file: %w", err)
	}
	lock := ChartLock{
		Generated:    time.Now().UTC(),
		Digest:       digestDependencies(lockDeps),
		Dependencies: lockDeps,
	}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}
	if err := writeFileInDir(chartsDir, lockFileName, lockData, cacheFilePerm); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}

func loadChartMetadata(chartsDir, chartName, chartDesc string) (ChartFile, error) {
	chart := ChartFile{
		APIVersion:  "v2",
		Name:        "placeholder",
		Description: "Auto-generated by helmseed",
		Version:     "0.1.0",
		Type:        "application",
	}

	data, err := readFileInDir(chartsDir, chartFileName)
	switch {
	case err == nil:
		var current ChartFile
		if err := yaml.Unmarshal(data, &current); err != nil {
			return ChartFile{}, fmt.Errorf("parse existing chart file: %w", err)
		}
		if current.APIVersion != "" {
			chart.APIVersion = current.APIVersion
		}
		if current.Name != "" {
			chart.Name = current.Name
		}
		if current.Description != "" {
			chart.Description = current.Description
		}
		if current.Version != "" {
			chart.Version = current.Version
		}
		if current.Type != "" {
			chart.Type = current.Type
		}
	case !os.IsNotExist(err):
		return ChartFile{}, fmt.Errorf("read existing chart file: %w", err)
	}

	if chartName != "" {
		chart.Name = chartName
	}
	if chartDesc != "" {
		chart.Description = chartDesc
	}

	return chart, nil
}

// Update re-fetches all repos listed in .helm/Chart.lock,
// replaces their .helm/charts/ entries, and rewrites the lock file.
func Update(ctx context.Context, opts BootstrapOptions) error {
	base, err := cacheDir(opts.CacheDir)
	if err != nil {
		return err
	}
	chartsDir, err := normalizeChartsDir(opts.ChartsDir)
	if err != nil {
		return err
	}
	return updateWithCacheDir(ctx, base, chartsDir, opts)
}

func updateWithCacheDir(ctx context.Context, base, chartsDir string, opts BootstrapOptions) error {
	lock, err := readLockFile(chartsDir)
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}
	if len(lock.Dependencies) == 0 {
		return fmt.Errorf("no entries in %s — run bootstrap first", filepath.Join(chartsDir, lockFileName))
	}
	if err := os.MkdirAll(base, cacheDirPerm); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	repos, err := reposFromLock(lock, base)
	if err != nil {
		return err
	}

	if err := cloneRepos(ctx, repos, base, opts); err != nil {
		return err
	}

	for _, r := range repos {
		if err := replaceChartDir(filepath.Join(base, r.Name), filepath.Join(chartsDir, "charts", r.Name)); err != nil {
			return fmt.Errorf(errCopyFmt, r.Name, err)
		}
		logf(opts, msgCopiedFromCacheFmt, r.Name)
	}

	return writeHelmFiles(repos, chartsDir, detectRefMode(lock), opts.ChartName, opts.ChartDesc)
}

func reposFromLock(lock *ChartLock, base string) ([]provider.Repo, error) {
	repos := make([]provider.Repo, 0, len(lock.Dependencies))
	seen := make(map[string]struct{}, len(lock.Dependencies))

	for _, dep := range lock.Dependencies {
		repoName, isLocalRef, err := repoNameFromLockDependency(dep)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[repoName]; exists {
			return nil, fmt.Errorf("duplicate dependency path %q in lock file", repoName)
		}

		m, err := readMeta(filepath.Join(base, repoName))
		if err == nil {
			repos = append(repos, provider.Repo{
				Name:          repoName,
				CloneURL:      m.CloneURL,
				HTTPSURL:      m.HTTPSURL,
				DefaultBranch: m.DefaultBranch,
			})
			seen[repoName] = struct{}{}
			continue
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read cache meta for %s: %w", repoName, err)
		}
		if isLocalRef {
			return nil, fmt.Errorf("read cache meta for %s: %w", repoName, err)
		}

		cloneURL, err := cloneURLFromRepository(dep.Repository)
		if err != nil {
			return nil, fmt.Errorf("dependency %s: %w", dep.Name, err)
		}
		repos = append(repos, provider.Repo{
			Name:     repoName,
			CloneURL: cloneURL,
			HTTPSURL: dep.Repository,
		})
		seen[repoName] = struct{}{}
	}
	return repos, nil
}

func repoNameFromLockDependency(dep LockDependency) (string, bool, error) {
	if !isValidRepoName(dep.Name) {
		return "", false, fmt.Errorf("invalid dependency name %q in lock file", dep.Name)
	}
	if strings.HasPrefix(dep.Repository, localChartRepoPrefix) {
		repoName := strings.TrimPrefix(dep.Repository, localChartRepoPrefix)
		if !isValidRepoName(repoName) {
			return "", true, fmt.Errorf("invalid local dependency path %q in lock file", dep.Repository)
		}
		return repoName, true, nil
	}
	return dep.Name, false, nil
}

func cloneURLFromRepository(repository string) (string, error) {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return "", errors.New("repository is empty and cache metadata is missing")
	}
	if strings.HasPrefix(repository, "http://") || strings.HasPrefix(repository, "https://") {
		if strings.HasSuffix(repository, ".git") {
			return repository, nil
		}
		return repository + ".git", nil
	}
	if filepath.IsAbs(repository) {
		if _, err := os.Stat(repository); err == nil {
			return repository, nil
		}
		withGitSuffix := repository + ".git"
		if _, err := os.Stat(withGitSuffix); err == nil {
			return withGitSuffix, nil
		}
		return repository, nil
	}
	if strings.HasPrefix(repository, "ssh://") || strings.HasPrefix(repository, "git://") || strings.HasPrefix(repository, "git@") || strings.HasPrefix(repository, "file://") {
		return repository, nil
	}
	return "", fmt.Errorf("repository %q cannot be used to restore a missing cache entry", repository)
}

func detectRefMode(lock *ChartLock) RepoRefMode {
	for _, dep := range lock.Dependencies {
		if strings.HasPrefix(dep.Repository, localChartRepoPrefix) {
			return LocalRef
		}
	}
	return RemoteRef
}

func readLockFile(chartsDir string) (*ChartLock, error) {
	data, err := readFileInDir(chartsDir, lockFileName)
	if err != nil {
		return nil, err
	}
	var lock ChartLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}
	return &lock, nil
}

func replaceChartDir(src, dest string) error {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, cacheDirPerm); err != nil {
		return fmt.Errorf("ensure chart parent dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(parent, filepath.Base(dest)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp chart dir: %w", err)
	}
	defer cleanupTempDir(tmpDir)

	if err := copyDir(src, tmpDir); err != nil {
		return err
	}

	backupPath := filepath.Join(parent, "."+filepath.Base(dest)+fmt.Sprintf(".old-%d", time.Now().UnixNano()))
	hasExisting, err := stageExistingDest(dest, backupPath)
	if err != nil {
		return err
	}

	if err := activateChartDir(tmpDir, dest, backupPath, hasExisting); err != nil {
		return err
	}

	if hasExisting {
		if err := os.RemoveAll(backupPath); err != nil {
			return fmt.Errorf("remove old chart dir: %w", err)
		}
	}

	return nil
}

// cleanupTempDir removes a leftover temp directory. A successful
// activate-rename has already moved tmpDir to dest, so ErrNotExist is the
// expected outcome and is silenced.
func cleanupTempDir(tmpDir string) {
	if err := os.RemoveAll(tmpDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("failed to remove temp chart dir", "path", tmpDir, "error", err)
	}
}

// stageExistingDest renames an existing dest to backupPath so the new chart
// can be moved into place atomically. Returns hasExisting=false when dest
// is absent (a fresh install).
func stageExistingDest(dest, backupPath string) (bool, error) {
	_, err := os.Stat(dest)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat chart dir: %w", err)
	}
	if err := os.Rename(dest, backupPath); err != nil {
		return false, fmt.Errorf("stage old chart dir: %w", err)
	}
	return true, nil
}

// activateChartDir promotes tmpDir to dest. On failure, when a backup of
// the prior chart exists, it is rolled back into dest; both legs failing
// leaves the directory missing and is reported in the wrapped error.
func activateChartDir(tmpDir, dest, backupPath string, hasExisting bool) error {
	err := os.Rename(tmpDir, dest)
	if err == nil {
		return nil
	}
	if !hasExisting {
		return fmt.Errorf("activate chart dir: %w", err)
	}
	if rbErr := os.Rename(backupPath, dest); rbErr != nil {
		slog.Error("failed to roll back chart dir; both new and old paths may be missing", "dest", dest, "backup", backupPath, "error", rbErr)
		return fmt.Errorf("activate chart dir: %w (rollback also failed: %v)", err, rbErr)
	}
	return fmt.Errorf("activate chart dir: %w", err)
}

func copyDir(src, dst string) error {
	srcRoot, err := os.OpenRoot(src)
	if err != nil {
		return fmt.Errorf("open source root: %w", err)
	}
	defer func() { _ = srcRoot.Close() }()

	if err := os.MkdirAll(dst, cacheDirPerm); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	dstRoot, err := os.OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("open dest root: %w", err)
	}
	defer func() { _ = dstRoot.Close() }()

	return fs.WalkDir(srcRoot.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		return copyDirEntry(srcRoot, dstRoot, path, d, walkErr)
	})
}

func copyDirEntry(srcRoot, dstRoot *os.Root, path string, d fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if path == "." || path == metaFileName {
		return nil
	}

	nativePath := filepath.FromSlash(path)
	if d.IsDir() {
		if err := dstRoot.Mkdir(nativePath, cacheDirPerm); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		return nil
	}
	return copyDirFile(srcRoot, dstRoot, path, nativePath)
}

func copyDirFile(srcRoot, dstRoot *os.Root, path, nativePath string) error {
	// Open via srcRoot — the kernel refuses to traverse symlinks
	// that escape the source tree, so a symlink to /etc/passwd fails here.
	srcFile, err := srcRoot.Open(nativePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	data, mode, err := readRegularFile(srcFile, path)
	if err != nil {
		return err
	}
	return writeFileTo(dstRoot, nativePath, data, mode)
}

func readRegularFile(srcFile *os.File, path string) ([]byte, os.FileMode, error) {
	info, err := srcFile.Stat()
	if err != nil {
		_ = srcFile.Close()
		return nil, 0, err
	}
	if info.IsDir() {
		_ = srcFile.Close()
		return nil, 0, fmt.Errorf("unsupported directory entry %s", path)
	}
	if !info.Mode().IsRegular() {
		_ = srcFile.Close()
		return nil, 0, fmt.Errorf("unsupported file mode %s for %s", info.Mode().String(), path)
	}

	data, readErr := io.ReadAll(srcFile)
	if closeErr := srcFile.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return nil, 0, readErr
	}
	return data, info.Mode().Perm(), nil
}

func writeFileTo(dstRoot *os.Root, nativePath string, data []byte, mode os.FileMode) error {
	dstFile, err := dstRoot.OpenFile(nativePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := dstFile.Write(data); err != nil {
		_ = dstFile.Close()
		return err
	}
	return dstFile.Close()
}

func normalizeChartsDir(chartsDir string) (string, error) {
	if chartsDir == "" {
		return defaultChartsDir, nil
	}
	if filepath.IsAbs(chartsDir) {
		return "", fmt.Errorf("charts_dir must be a relative path, got %q", chartsDir)
	}
	cleaned := filepath.Clean(chartsDir)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("charts_dir must not contain '..', got %q", chartsDir)
	}
	return cleaned, nil
}

func outWriter(opts BootstrapOptions) io.Writer {
	if opts.Out != nil {
		return opts.Out
	}
	return os.Stdout
}

func logf(opts BootstrapOptions, format string, args ...any) {
	if opts.Quiet {
		return
	}
	_, _ = fmt.Fprintf(outWriter(opts), format, args...)
}
