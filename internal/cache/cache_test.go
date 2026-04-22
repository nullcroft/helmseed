package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/nullcroft/helmseed/internal/provider"
	"go.yaml.in/yaml/v3"
)

// initBareRepo creates a bare git repo with one commit so it can be cloned.
func initBareRepo(t *testing.T, dir, branch string) string {
	t.Helper()
	path := filepath.Join(dir, branch+".git")

	repo, err := git.PlainInit(path, true)
	if err != nil {
		t.Fatalf("init bare repo: %v", err)
	}

	tmp := t.TempDir()
	wt, err := git.PlainClone(tmp, false, &git.CloneOptions{URL: path})
	if err != nil {
		wt, err = git.PlainInit(tmp, false)
		if err != nil {
			t.Fatalf("init working repo: %v", err)
		}
	}

	w, err := wt.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	f, err := os.Create(filepath.Join(tmp, "Chart.yaml"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if _, err := f.WriteString("name: test\n"); err != nil {
		_ = f.Close()
		t.Fatalf("write file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	if _, err := w.Add("Chart.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}
	_, err = w.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	remote, err := wt.CreateRemote(&config.RemoteConfig{
		Name: "bare",
		URLs: []string{path},
	})
	if err != nil {
		t.Fatalf("create remote: %v", err)
	}
	if err := remote.Push(&git.PushOptions{RemoteName: "bare"}); err != nil {
		t.Fatalf("push: %v", err)
	}

	if _, err = repo.Reference("refs/heads/master", true); err != nil {
		t.Fatalf("bare repo missing master ref: %v", err)
	}

	return path
}

func TestBootstrap_ClonesAndCopies(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")
	bare2 := initBareRepo(t, bareDir, "repo-b")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	repos := []provider.Repo{
		{Name: "repo-a", CloneURL: bare1, HTTPSURL: "https://example.com/repo-a", DefaultBranch: "master"},
		{Name: "repo-b", CloneURL: bare2, HTTPSURL: "https://example.com/repo-b", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: RemoteRef, ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("Bootstrap() error: %v", err)
	}

	for _, r := range repos {
		cacheEntry := filepath.Join(tmpCache, r.Name)
		if _, err := os.Stat(filepath.Join(cacheEntry, "Chart.yaml")); err != nil {
			t.Errorf("cache %s: Chart.yaml missing: %v", r.Name, err)
		}
		if _, err := os.Stat(filepath.Join(cacheEntry, "meta.json")); err != nil {
			t.Errorf("cache %s: meta.json missing: %v", r.Name, err)
		}
		if _, err := os.Stat(filepath.Join(cacheEntry, ".git")); !os.IsNotExist(err) {
			t.Errorf("cache %s: .git should have been stripped", r.Name)
		}
	}

	for _, r := range repos {
		dest := filepath.Join(testChartsDir+"/charts", r.Name)
		if _, err := os.Stat(filepath.Join(dest, "Chart.yaml")); err != nil {
			t.Errorf("charts %s: Chart.yaml missing: %v", r.Name, err)
		}
		if _, err := os.Stat(filepath.Join(dest, "meta.json")); !os.IsNotExist(err) {
			t.Errorf("charts %s: meta.json should not be copied", r.Name)
		}
	}

	lockFile := filepath.Join(testChartsDir, lockFileName)
	lockData, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	var lock ChartLock
	if err := yaml.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("lock file is not valid YAML: %v", err)
	}
	if lock.Generated.IsZero() {
		t.Error("lock file: generated timestamp is zero")
	}
	if lock.Digest == "" {
		t.Error("lock file: digest is empty")
	}
	if len(lock.Dependencies) != 2 {
		t.Fatalf("lock file: expected 2 dependencies, got %d", len(lock.Dependencies))
	}
	for _, dep := range lock.Dependencies {
		if dep.Name == "" {
			t.Error("lock dependency: name is empty")
		}
		if dep.Repository == "" {
			t.Error("lock dependency: repository is empty")
		}
		if dep.Version == "" {
			t.Error("lock dependency: version is empty")
		}
	}

	for _, r := range repos {
		m, err := readMeta(filepath.Join(tmpCache, r.Name))
		if err != nil {
			t.Errorf("read meta %s: %v", r.Name, err)
			continue
		}
		if len(m.Commit) != 40 {
			t.Errorf("meta %s: commit SHA should be 40 chars, got %q", r.Name, m.Commit)
		}
	}
}

func TestBootstrap_CacheHit(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	repos := []provider.Repo{
		{Name: "repo-a", CloneURL: bare1, HTTPSURL: "https://example.com/repo-a", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: RemoteRef, ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("first Bootstrap() error: %v", err)
	}

	_ = os.RemoveAll(filepath.Join(testChartsDir, "charts", "repo-a"))

	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("second Bootstrap() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(testChartsDir, "charts", "repo-a", "Chart.yaml")); err != nil {
		t.Errorf("Chart.yaml missing after cache-hit bootstrap: %v", err)
	}
}

func TestBootstrap_TTLExpired(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	repos := []provider.Repo{
		{Name: "repo-a", CloneURL: bare1, HTTPSURL: "https://example.com/repo-a", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: RemoteRef, ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("first Bootstrap() error: %v", err)
	}

	m := Meta{
		ClonedAt:      time.Now().Add(-48 * time.Hour),
		CloneURL:      bare1,
		DefaultBranch: "master",
	}
	if err := writeMeta(filepath.Join(tmpCache, "repo-a"), m); err != nil {
		t.Fatalf("backdate meta: %v", err)
	}

	_ = os.RemoveAll(filepath.Join(testChartsDir, "charts", "repo-a"))

	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("second Bootstrap() error: %v", err)
	}

	meta, err := readMeta(filepath.Join(tmpCache, "repo-a"))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if time.Since(meta.ClonedAt) > time.Minute {
		t.Errorf("meta.ClonedAt should be recent, got %v", meta.ClonedAt)
	}
}

func TestBootstrap_SkipsExistingCharts(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "existing")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	dest := filepath.Join(testChartsDir+"/charts", "existing")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "marker.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	repos := []provider.Repo{
		{Name: "existing", CloneURL: bare1, HTTPSURL: "https://example.com/existing", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: RemoteRef, ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("Bootstrap() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "marker.txt"))
	if err != nil {
		t.Fatalf("marker.txt missing: %v", err)
	}
	if string(data) != "keep" {
		t.Errorf("marker.txt content changed: got %q", string(data))
	}
}

func TestBootstrap_LocalRef(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")
	bare2 := initBareRepo(t, bareDir, "repo-b")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	repos := []provider.Repo{
		{Name: "repo-a", CloneURL: bare1, HTTPSURL: "https://example.com/repo-a", DefaultBranch: "master"},
		{Name: "repo-b", CloneURL: bare2, HTTPSURL: "https://example.com/repo-b", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: LocalRef, ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("Bootstrap() error: %v", err)
	}

	lockFile := filepath.Join(testChartsDir, lockFileName)
	lockData, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	var lock ChartLock
	if err := yaml.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("lock file is not valid YAML: %v", err)
	}
	for _, dep := range lock.Dependencies {
		expected := "file://charts/" + dep.Name
		if dep.Repository != expected {
			t.Errorf("dependency %s: repository = %q, want %q", dep.Name, dep.Repository, expected)
		}
	}
}

func TestBootstrap_RemoteRef(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	repos := []provider.Repo{
		{Name: "repo-a", CloneURL: bare1, HTTPSURL: "https://example.com/repo-a", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: RemoteRef, ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("Bootstrap() error: %v", err)
	}

	lockFile := filepath.Join(testChartsDir, lockFileName)
	lockData, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	var lock ChartLock
	if err := yaml.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("lock file is not valid YAML: %v", err)
	}
	if len(lock.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(lock.Dependencies))
	}
	expected := "https://example.com/repo-a"
	if lock.Dependencies[0].Repository != expected {
		t.Errorf("repository = %q, want %q", lock.Dependencies[0].Repository, expected)
	}
}

func TestBootstrap_PrefixStripped(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "helm-app-frontend")

	workDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	tmpCache := filepath.Join(workDir, "cache")
	testChartsDir := filepath.Join(workDir, ".helm")

	repos := []provider.Repo{
		{Name: "helm-app-frontend", CloneURL: bare1, HTTPSURL: "https://example.com/helm-app-frontend", DefaultBranch: "master"},
	}

	opts := BootstrapOptions{TTL: 24 * time.Hour, Mode: RemoteRef, Prefix: "helm-app-", ChartsDir: testChartsDir}
	if err := bootstrap(context.Background(), repos, tmpCache, testChartsDir, opts); err != nil {
		t.Fatalf("Bootstrap() error: %v", err)
	}

	lockFile := filepath.Join(testChartsDir, lockFileName)
	lockData, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	var lock ChartLock
	if err := yaml.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("lock file is not valid YAML: %v", err)
	}
	if len(lock.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(lock.Dependencies))
	}
	if lock.Dependencies[0].Name != "frontend" {
		t.Errorf("name = %q, want %q", lock.Dependencies[0].Name, "frontend")
	}

	if _, err := os.Stat(filepath.Join(testChartsDir, "charts", "frontend")); err != nil {
		t.Errorf("charts dir 'frontend' missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(testChartsDir, "charts", "helm-app-frontend")); !os.IsNotExist(err) {
		t.Error("charts dir should use stripped name, not full name")
	}
}
