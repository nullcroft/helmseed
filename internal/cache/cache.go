package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nullcroft/helmseed/internal/provider"
	"go.yaml.in/yaml/v3"
)

const (
	chartsDir  = ".helm/charts"
	chartFile  = ".helm/Chart.yaml"
	lockFile   = ".helm/Chart.lock"
	maxWorkers = 5
)

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
	TTL    time.Duration
	Mode   RepoRefMode
	Prefix string
}

func cacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "helmseed"), nil
}

func readMeta(dir string) (Meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
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
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644)
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
	base, err := cacheDir()
	if err != nil {
		return err
	}
	return bootstrap(ctx, repos, base, opts)
}

func bootstrap(ctx context.Context, repos []provider.Repo, cacheBase string, opts BootstrapOptions) error {
	// Strip prefix from repo names so cache dirs, chart dirs, and lock entries
	// all use the short name.
	if opts.Prefix != "" {
		stripped := make([]provider.Repo, len(repos))
		for i, r := range repos {
			r.Name = strings.TrimPrefix(r.Name, opts.Prefix)
			stripped[i] = r
		}
		repos = stripped
	}

	if err := os.MkdirAll(cacheBase, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := os.MkdirAll(chartsDir, 0o755); err != nil {
		return fmt.Errorf("create charts dir: %w", err)
	}

	if err := ensureCached(ctx, repos, cacheBase, opts.TTL); err != nil {
		return err
	}

	for _, r := range repos {
		dest := filepath.Join(chartsDir, r.Name)
		if _, err := os.Stat(dest); err == nil {
			fmt.Printf("  skip %s (already exists)\n", r.Name)
			continue
		}

		src := filepath.Join(cacheBase, r.Name)
		fmt.Printf("  copy %s from cache\n", r.Name)
		if err := copyDir(src, dest); err != nil {
			return fmt.Errorf("copy %s from cache: %w", r.Name, err)
		}
	}

	return writeHelmFiles(repos, cacheBase, opts.Mode)
}

func ensureCached(ctx context.Context, repos []provider.Repo, base string, ttl time.Duration) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, maxWorkers)
	)

	for _, r := range repos {
		entryDir := filepath.Join(base, r.Name)

		if isFresh(entryDir, ttl) {
			fmt.Printf("  cache hit %s\n", r.Name)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(r provider.Repo, entryDir string) {
			defer wg.Done()
			defer func() { <-sem }()

			_ = os.RemoveAll(entryDir)

			fmt.Printf("  cache miss %s — cloning ...\n", r.Name)

			cloneOpts := &git.CloneOptions{
				URL:           r.CloneURL,
				Depth:         1,
				SingleBranch:  true,
				ReferenceName: plumbing.NewBranchReferenceName(r.DefaultBranch),
			}

			repo, cloneErr := git.PlainCloneContext(ctx, entryDir, false, cloneOpts)
			if cloneErr != nil {
				_ = os.RemoveAll(entryDir)
				mu.Lock()
				errs = append(errs, fmt.Errorf("clone %s: %w", r.Name, cloneErr))
				mu.Unlock()
				return
			}

			head, err := repo.Head()
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("read HEAD for %s: %w", r.Name, err))
				mu.Unlock()
				return
			}
			commitSHA := head.Hash().String()

			if err := os.RemoveAll(filepath.Join(entryDir, ".git")); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("strip .git from %s: %w", r.Name, err))
				mu.Unlock()
				return
			}

			m := Meta{
				ClonedAt:      time.Now(),
				CloneURL:      r.CloneURL,
				HTTPSURL:      r.HTTPSURL,
				DefaultBranch: r.DefaultBranch,
				Commit:        commitSHA,
			}
			if err := writeMeta(entryDir, m); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("write meta for %s: %w", r.Name, err))
				mu.Unlock()
			}
		}(r, entryDir)
	}

	wg.Wait()
	return errors.Join(errs...)
}

func readChartVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	if err != nil {
		return "0.0.0"
	}
	var cm chartMeta
	if err := yaml.Unmarshal(data, &cm); err != nil || cm.Version == "" {
		return "0.0.0"
	}
	return cm.Version
}

func digestDependencies(deps []LockDependency) string {
	sorted := make([]LockDependency, len(deps))
	copy(sorted, deps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	h := sha256.New()
	for _, d := range sorted {
		_, _ = fmt.Fprintf(h, "%s|%s|%s\n", d.Name, d.Repository, d.Version)
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func writeHelmFiles(repos []provider.Repo, cacheBase string, mode RepoRefMode) error {
	var (
		lockDeps  []LockDependency
		chartDeps []ChartDependency
	)
	for _, r := range repos {
		version := readChartVersion(filepath.Join(cacheBase, r.Name))

		repo := strings.TrimSuffix(r.HTTPSURL, ".git")
		if mode == LocalRef {
			repo = "file://charts/" + r.Name
		}

		lockDeps = append(lockDeps, LockDependency{
			Name:       r.Name,
			Repository: repo,
			Version:    version,
		})
		chartDeps = append(chartDeps, ChartDependency{
			Name:       r.Name,
			Version:    version,
			Repository: repo,
		})
	}

	chart := ChartFile{
		APIVersion:   "v2",
		Name:         "placeholder",
		Description:  "placeholder",
		Version:      "0.1.0",
		Type:         "application",
		Dependencies: chartDeps,
	}
	chartData, err := yaml.Marshal(chart)
	if err != nil {
		return fmt.Errorf("marshal chart file: %w", err)
	}
	if err := os.WriteFile(chartFile, chartData, 0o644); err != nil {
		return fmt.Errorf("write chart file: %w", err)
	}
	fmt.Printf("  wrote %s\n", chartFile)

	lock := ChartLock{
		Generated:    time.Now().UTC(),
		Digest:       digestDependencies(lockDeps),
		Dependencies: lockDeps,
	}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}
	if err := os.WriteFile(lockFile, lockData, 0o644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	fmt.Printf("  wrote %s\n", lockFile)

	return nil
}

// Update re-fetches stale repos listed in .helm/Chart.lock,
// replaces their .helm/charts/ entries, and rewrites the lock file.
func Update(ctx context.Context, ttl time.Duration) error {
	base, err := cacheDir()
	if err != nil {
		return err
	}
	return updateWithCacheDir(ctx, base, ttl)
}

func updateWithCacheDir(ctx context.Context, base string, ttl time.Duration) error {
	lock, err := readLockFile()
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}
	if len(lock.Dependencies) == 0 {
		return fmt.Errorf("no entries in %s — run bootstrap first", lockFile)
	}

	mode := detectRefMode(lock)

	var repos []provider.Repo
	for _, dep := range lock.Dependencies {
		entryDir := filepath.Join(base, dep.Name)
		m, err := readMeta(entryDir)
		if err != nil {
			return fmt.Errorf("read cache meta for %s: %w", dep.Name, err)
		}
		repos = append(repos, provider.Repo{
			Name:          dep.Name,
			CloneURL:      m.CloneURL,
			HTTPSURL:      m.HTTPSURL,
			DefaultBranch: m.DefaultBranch,
		})
	}

	if err := ensureCached(ctx, repos, base, ttl); err != nil {
		return err
	}

	for _, r := range repos {
		dest := filepath.Join(chartsDir, r.Name)
		_ = os.RemoveAll(dest)

		src := filepath.Join(base, r.Name)
		fmt.Printf("  copy %s from cache\n", r.Name)
		if err := copyDir(src, dest); err != nil {
			return fmt.Errorf("copy %s from cache: %w", r.Name, err)
		}
	}

	return writeHelmFiles(repos, base, mode)
}

func detectRefMode(lock *ChartLock) RepoRefMode {
	for _, dep := range lock.Dependencies {
		if strings.HasPrefix(dep.Repository, "file://") {
			return LocalRef
		}
	}
	return RemoteRef
}

func readLockFile() (*ChartLock, error) {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return nil, err
	}
	var lock ChartLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}
	return &lock, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if rel == "meta.json" {
			return nil
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode())
	})
}
