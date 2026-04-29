package cache

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nullcroft/helmseed/internal/provider"
	"go.yaml.in/yaml/v3"
)

func TestIsValidRepoName(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"foo", true},
		{"foo-bar", true},
		{"foo_bar", true},
		{"", false},
		{".", false},
		{"..", false},
		{"foo/..", false},
		{"../foo", false},
		{"foo/bar", false},
		{"foo\\bar", false},
		{".hidden", false},
	}
	for _, c := range cases {
		got := isValidRepoName(c.name)
		if got != c.valid {
			t.Errorf("isValidRepoName(%q) = %v, want %v", c.name, got, c.valid)
		}
	}
}

func TestCacheDir_Absolute(t *testing.T) {
	dir, err := cacheDir("/tmp/helmseed-cache")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/tmp/helmseed-cache" {
		t.Errorf("dir = %q, want /tmp/helmseed-cache", dir)
	}
}

func TestCacheDir_Relative(t *testing.T) {
	_, err := cacheDir("relative/cache")
	if err == nil {
		t.Fatal("expected error for relative cache_dir")
	}
}

func TestCacheDir_Default(t *testing.T) {
	dir, err := cacheDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty default cache dir")
	}
}

func TestCacheDir_Traversal(t *testing.T) {
	_, err := cacheDir("/tmp/hel..seed")
	if err == nil {
		t.Fatal("expected error for cache_dir containing '..'")
	}
}

func TestReadChartVersion(t *testing.T) {
	dir := t.TempDir()
	// Missing Chart.yaml
	if _, err := readChartVersion(dir); err == nil {
		t.Fatal("expected error for missing Chart.yaml")
	}

	// Invalid YAML
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("not yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readChartVersion(dir); err == nil {
		t.Fatal("expected error for invalid Chart.yaml")
	}

	// Valid YAML with version
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("version: 1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ver, err := readChartVersion(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", ver)
	}
}

func TestWriteHelmFilesUsesChildChartName(t *testing.T) {
	chartsDir := t.TempDir()
	chartDir := filepath.Join(chartsDir, "charts", "repo-a")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("mkdir chart dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, chartFileName), []byte("name: postgres\nversion: 1.2.3\n"), 0o644); err != nil {
		t.Fatalf("write child chart: %v", err)
	}

	repos := []provider.Repo{{Name: "repo-a", HTTPSURL: "https://example.com/repo-a.git"}}
	if err := writeHelmFiles(repos, chartsDir, RemoteRef, "", ""); err != nil {
		t.Fatalf("writeHelmFiles error: %v", err)
	}

	lock, err := readLockFile(chartsDir)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lock.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(lock.Dependencies))
	}
	if lock.Dependencies[0].Name != "postgres" {
		t.Fatalf("dependency name = %q, want postgres", lock.Dependencies[0].Name)
	}
}

func TestDigestDependencies(t *testing.T) {
	deps := []LockDependency{
		{Name: "b", Repository: "https://b", Version: "2.0.0"},
		{Name: "a", Repository: "https://a", Version: "1.0.0"},
	}
	d1 := digestDependencies(deps)
	d2 := digestDependencies(deps)
	if d1 != d2 {
		t.Error("digest should be deterministic")
	}
	if d1 == "" {
		t.Error("digest should not be empty")
	}
	if d1[:7] != "sha256:" {
		t.Error("digest should start with sha256:")
	}
}

func TestDetectRefMode(t *testing.T) {
	lock := &ChartLock{
		Dependencies: []LockDependency{
			{Name: "a", Repository: "https://example.com/a"},
		},
	}
	if detectRefMode(lock) != RemoteRef {
		t.Error("expected RemoteRef")
	}

	lock.Dependencies[0].Repository = localChartRepoPrefix + "a"
	if detectRefMode(lock) != LocalRef {
		t.Error("expected LocalRef")
	}
}

func TestReadLockFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Chart.lock")
	lock := ChartLock{
		Generated: time.Now().UTC(),
		Digest:    "abc123",
		Dependencies: []LockDependency{
			{Name: "foo", Repository: "https://foo", Version: "1.0.0"},
		},
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := readLockFile(dir)
	if err != nil {
		t.Fatalf("readLockFile error: %v", err)
	}
	if read.Digest != "abc123" {
		t.Errorf("digest = %q, want abc123", read.Digest)
	}
	if len(read.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(read.Dependencies))
	}
}

func TestReadLockFile_Missing(t *testing.T) {
	_, err := readLockFile(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing lock file")
	}
}

func TestReadLockFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chart.lock"), []byte("not yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readLockFile(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "foo.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(src, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "bar.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	// meta.json should be skipped
	if err := os.WriteFile(filepath.Join(src, metaFileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "foo.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("foo.txt content = %q, want hello", string(data))
	}

	if _, err := os.Stat(filepath.Join(dst, metaFileName)); !os.IsNotExist(err) {
		t.Error("meta.json should not be copied")
	}

	data, err = os.ReadFile(filepath.Join(dst, "sub", "bar.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "world" {
		t.Errorf("sub/bar.txt content = %q, want world", string(data))
	}
}

func TestCopyDir_RejectsSymlinkOutsideSource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	outsideDir := t.TempDir()

	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(src, "outside.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected error for symlink escaping source tree")
	}
}

func TestBootstrap_InvalidRepoName(t *testing.T) {
	tmpDir := t.TempDir()
	repos := []provider.Repo{{Name: "../bad", CloneURL: "", HTTPSURL: "", DefaultBranch: "main"}}
	opts := BootstrapOptions{TTL: time.Hour, ChartsDir: filepath.Join(tmpDir, ".helm")}
	err := bootstrap(context.Background(), repos, tmpDir, opts.ChartsDir, opts)
	if err == nil {
		t.Fatal("expected error for invalid repo name")
	}
}

func TestBootstrap_AbsoluteChartsDir(t *testing.T) {
	repos := []provider.Repo{{Name: "good", CloneURL: "", HTTPSURL: "", DefaultBranch: "main"}}
	opts := BootstrapOptions{TTL: time.Hour, ChartsDir: "/absolute/path"}
	err := Bootstrap(context.Background(), repos, opts)
	if err == nil {
		t.Fatal("expected error for absolute charts_dir")
	}
}

func TestBootstrap_ChartsDirWithDotDot(t *testing.T) {
	repos := []provider.Repo{{Name: "good", CloneURL: "", HTTPSURL: "", DefaultBranch: "main"}}
	opts := BootstrapOptions{TTL: time.Hour, ChartsDir: "../charts"}
	err := Bootstrap(context.Background(), repos, opts)
	if err == nil {
		t.Fatal("expected error for charts_dir containing '..'")
	}
}

func TestUpdate_NoLockFile(t *testing.T) {
	cacheBase := t.TempDir()
	chartsDir := t.TempDir()
	err := updateWithCacheDir(context.Background(), cacheBase, chartsDir, BootstrapOptions{})
	if err == nil {
		t.Fatal("expected error when lock file is missing")
	}
}

func TestUpdate_EmptyLock(t *testing.T) {
	cacheBase := t.TempDir()
	chartsDir := t.TempDir()
	lock := ChartLock{Generated: time.Now().UTC(), Digest: "", Dependencies: nil}
	data, _ := yaml.Marshal(lock)
	_ = os.WriteFile(filepath.Join(chartsDir, "Chart.lock"), data, 0o644)
	err := updateWithCacheDir(context.Background(), cacheBase, chartsDir, BootstrapOptions{})
	if err == nil {
		t.Fatal("expected error for empty lock")
	}
}

func TestPrepareRepos(t *testing.T) {
	repos := []provider.Repo{
		{Name: "helm-app-frontend"},
		{Name: "helm-app-backend"},
	}

	got, err := prepareRepos(repos, "helm-app-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(got))
	}
	if got[0].Name != "frontend" {
		t.Errorf("repo[0].Name = %q, want frontend", got[0].Name)
	}
	if got[1].Name != "backend" {
		t.Errorf("repo[1].Name = %q, want backend", got[1].Name)
	}

	// Empty prefix: names unchanged
	got2, err := prepareRepos(repos, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2[0].Name != "helm-app-frontend" {
		t.Errorf("empty prefix: repo[0].Name = %q, want helm-app-frontend", got2[0].Name)
	}

	// Invalid repo name
	_, err = prepareRepos([]provider.Repo{{Name: "../bad"}}, "")
	if err == nil {
		t.Error("expected error for invalid repo name")
	}

	_, err = prepareRepos([]provider.Repo{{Name: "chart-a"}, {Name: "a"}}, "chart-")
	if err == nil {
		t.Fatal("expected duplicate name error after prefix normalization")
	}

	_, err = prepareRepos([]provider.Repo{{Name: "chart-"}}, "chart-")
	if err == nil {
		t.Fatal("expected empty name error after prefix normalization")
	}
}

func TestPartitionRepos(t *testing.T) {
	tmp := t.TempDir()
	// fresh repo
	freshDir := filepath.Join(tmp, "fresh")
	_ = os.MkdirAll(freshDir, 0o755)
	_ = writeMeta(freshDir, Meta{ClonedAt: time.Now()})

	// stale repo
	staleDir := filepath.Join(tmp, "stale")
	_ = os.MkdirAll(staleDir, 0o755)
	_ = writeMeta(staleDir, Meta{ClonedAt: time.Now().Add(-48 * time.Hour)})

	repos := []provider.Repo{
		{Name: "fresh"},
		{Name: "stale"},
		{Name: "missing"},
	}
	cached, stale := partitionRepos(repos, tmp, 24*time.Hour)
	if len(cached) != 1 || cached[0].Name != "fresh" {
		t.Errorf("cached = %v, want [fresh]", cached)
	}
	if len(stale) != 2 {
		t.Errorf("stale count = %d, want 2", len(stale))
	}
}

func TestSetupDirs(t *testing.T) {
	tmp := t.TempDir()
	cacheBase := filepath.Join(tmp, "cache")
	chartsDir := filepath.Join(tmp, "charts")
	if err := setupDirs(cacheBase, chartsDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(cacheBase); err != nil {
		t.Errorf("cache dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chartsDir, "charts")); err != nil {
		t.Errorf("charts/charts dir missing: %v", err)
	}
}

func TestCopyRepos(t *testing.T) {
	tmp := t.TempDir()
	cacheBase := filepath.Join(tmp, "cache")
	chartsDir := filepath.Join(tmp, ".helm")
	_ = os.MkdirAll(filepath.Join(chartsDir, "charts"), 0o755)

	// Set up cache entry
	_ = os.MkdirAll(filepath.Join(cacheBase, "repo-a"), 0o755)
	_ = os.WriteFile(filepath.Join(cacheBase, "repo-a", "Chart.yaml"), []byte("name: a\nversion: 1.0.0\n"), 0o644)

	repos := []provider.Repo{{Name: "repo-a"}}
	if err := copyRepos(repos, cacheBase, chartsDir, BootstrapOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chartsDir, "charts", "repo-a", "Chart.yaml")); err != nil {
		t.Errorf("copied chart missing: %v", err)
	}

	// Second copy should skip existing
	if err := copyRepos(repos, cacheBase, chartsDir, BootstrapOptions{}); err != nil {
		t.Fatalf("second copy error: %v", err)
	}
}

func TestWarnSkippedExisting(t *testing.T) {
	var out bytes.Buffer
	opts := BootstrapOptions{Out: &out}
	warnSkippedExisting([]provider.Repo{{Name: "a"}, {Name: "b"}}, opts)
	if !strings.Contains(out.String(), "already exist and were left unchanged") {
		t.Fatalf("unexpected warning output %q", out.String())
	}

	out.Reset()
	opts.Quiet = true
	warnSkippedExisting([]provider.Repo{{Name: "a"}}, opts)
	if out.Len() != 0 {
		t.Fatalf("quiet mode should not print warnings, got %q", out.String())
	}
}

func TestReposFromLock(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "cache")
	_ = os.MkdirAll(filepath.Join(base, "repo-a"), 0o755)
	_ = writeMeta(filepath.Join(base, "repo-a"), Meta{CloneURL: "git@example.com:a.git", HTTPSURL: "https://example.com/a", DefaultBranch: "main"})

	lock := &ChartLock{
		Dependencies: []LockDependency{
			{Name: "repo-a", Repository: "https://example.com/a", Version: "1.0.0"},
		},
	}
	repos, err := reposFromLock(lock, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "repo-a" {
		t.Errorf("name = %q, want repo-a", repos[0].Name)
	}

	// Missing meta should error
	lock2 := &ChartLock{Dependencies: []LockDependency{{Name: "missing", Repository: "", Version: ""}}}
	_, err = reposFromLock(lock2, base)
	if err == nil {
		t.Error("expected error for missing meta")
	}

	lock3 := &ChartLock{Dependencies: []LockDependency{{Name: "../bad", Repository: "", Version: ""}}}
	_, err = reposFromLock(lock3, base)
	if err == nil {
		t.Error("expected error for invalid dependency name")
	}

	lock4 := &ChartLock{Dependencies: []LockDependency{
		{Name: "repo-a", Repository: "", Version: ""},
		{Name: "repo-a", Repository: "", Version: ""},
	}}
	_, err = reposFromLock(lock4, base)
	if err == nil {
		t.Error("expected error for duplicate dependency names")
	}
}

func TestReplaceChartDir(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "cache")
	chartsDir := filepath.Join(tmp, ".helm")
	src := filepath.Join(base, "repo-a")
	_ = os.MkdirAll(src, 0o755)
	_ = os.WriteFile(filepath.Join(src, "Chart.yaml"), []byte("name: a\nversion: 1.0.0\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(chartsDir, "charts"), 0o755)

	dest := filepath.Join(chartsDir, "charts", "repo-a")
	_ = os.MkdirAll(dest, 0o755)
	_ = os.WriteFile(filepath.Join(dest, "stale.txt"), []byte("stale"), 0o644)

	if err := replaceChartDir(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "Chart.yaml")); err != nil {
		t.Errorf("copied chart missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("old chart files should be replaced")
	}
}

func TestUpdateWithCacheDir_ClonesAll(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")

	tmp := t.TempDir()
	base := filepath.Join(tmp, "cache")
	chartsDir := filepath.Join(tmp, ".helm")
	_ = os.MkdirAll(filepath.Join(chartsDir, "charts"), 0o755)

	cacheEntry := filepath.Join(base, "repo-a")
	_ = os.MkdirAll(cacheEntry, 0o755)
	_ = writeMeta(cacheEntry, Meta{
		ClonedAt:      time.Now(),
		CloneURL:      bare1,
		HTTPSURL:      "https://example.com/repo-a",
		DefaultBranch: "master",
		Commit:        "old",
	})

	lock := ChartLock{
		Generated: time.Now().UTC(),
		Digest:    "digest",
		Dependencies: []LockDependency{
			{Name: "repo-a", Repository: "https://example.com/repo-a", Version: "1.0.0"},
		},
	}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartsDir, lockFileName), lockData, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := updateWithCacheDir(context.Background(), base, chartsDir, BootstrapOptions{}); err != nil {
		t.Fatalf("updateWithCacheDir error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(chartsDir, "charts", "repo-a", "Chart.yaml")); err != nil {
		t.Errorf("cloned chart missing: %v", err)
	}

	meta, err := readMeta(filepath.Join(base, "repo-a"))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if meta.Commit == "old" {
		t.Fatal("expected repo to be re-cloned during update")
	}
}

func TestUpdateWithCacheDir_UsesLockRepositoryWhenMetaMissing(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")

	tmp := t.TempDir()
	base := filepath.Join(tmp, "cache")
	chartsDir := filepath.Join(tmp, ".helm")
	if err := os.MkdirAll(filepath.Join(chartsDir, "charts"), 0o755); err != nil {
		t.Fatalf("mkdir charts: %v", err)
	}

	lock := ChartLock{
		Generated: time.Now().UTC(),
		Digest:    "digest",
		Dependencies: []LockDependency{
			{Name: "repo-a", Repository: strings.TrimSuffix(bare1, ".git"), Version: "1.0.0"},
		},
	}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartsDir, lockFileName), lockData, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := updateWithCacheDir(context.Background(), base, chartsDir, BootstrapOptions{}); err != nil {
		t.Fatalf("updateWithCacheDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chartsDir, "charts", "repo-a", chartFileName)); err != nil {
		t.Fatalf("updated chart missing: %v", err)
	}
	if _, err := readMeta(filepath.Join(base, "repo-a")); err != nil {
		t.Fatalf("expected cache meta to be written: %v", err)
	}
}

func TestUpdateWithCacheDir_PreservesChartMetadata(t *testing.T) {
	bareDir := t.TempDir()
	bare1 := initBareRepo(t, bareDir, "repo-a")

	tmp := t.TempDir()
	base := filepath.Join(tmp, "cache")
	chartsDir := filepath.Join(tmp, ".helm")
	_ = os.MkdirAll(filepath.Join(chartsDir, "charts"), 0o755)

	cacheEntry := filepath.Join(base, "repo-a")
	_ = os.MkdirAll(cacheEntry, 0o755)
	_ = writeMeta(cacheEntry, Meta{
		ClonedAt:      time.Now(),
		CloneURL:      bare1,
		HTTPSURL:      "https://example.com/repo-a",
		DefaultBranch: "master",
	})

	lock := ChartLock{
		Generated: time.Now().UTC(),
		Digest:    "digest",
		Dependencies: []LockDependency{
			{Name: "repo-a", Repository: "https://example.com/repo-a", Version: "1.0.0"},
		},
	}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartsDir, lockFileName), lockData, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	existingChart := ChartFile{
		APIVersion:  "v2",
		Name:        "custom-name",
		Description: "custom-description",
		Version:     "2.3.4",
		Type:        "application",
	}
	existingChartData, err := yaml.Marshal(existingChart)
	if err != nil {
		t.Fatalf("marshal existing chart: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartsDir, chartFileName), existingChartData, 0o644); err != nil {
		t.Fatalf("write existing chart: %v", err)
	}

	if err := updateWithCacheDir(context.Background(), base, chartsDir, BootstrapOptions{}); err != nil {
		t.Fatalf("updateWithCacheDir error: %v", err)
	}

	updatedData, err := os.ReadFile(filepath.Join(chartsDir, chartFileName))
	if err != nil {
		t.Fatalf("read chart: %v", err)
	}
	var updated ChartFile
	if err := yaml.Unmarshal(updatedData, &updated); err != nil {
		t.Fatalf("parse chart: %v", err)
	}
	if updated.Name != "custom-name" || updated.Description != "custom-description" || updated.Version != "2.3.4" {
		t.Fatalf("expected existing metadata to be preserved, got %+v", updated)
	}
	if len(updated.Dependencies) != 1 || updated.Dependencies[0].Name != "repo-a" {
		t.Fatalf("expected updated dependencies, got %+v", updated.Dependencies)
	}
}

func TestLoadChartMetadata_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	chart, err := loadChartMetadata(dir, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chart.APIVersion != "v2" || chart.Name != "placeholder" {
		t.Errorf("expected defaults, got %+v", chart)
	}
}

func TestLoadChartMetadata_OverridesPreserveExistingWhenFlagsEmpty(t *testing.T) {
	dir := t.TempDir()
	existing := ChartFile{
		APIVersion:  "v2",
		Name:        "keep-me",
		Description: "keep-desc",
		Version:     "9.9.9",
		Type:        "library",
	}
	data, err := yaml.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, chartFileName), data, 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	chart, err := loadChartMetadata(dir, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chart.Name != "keep-me" || chart.Description != "keep-desc" || chart.Version != "9.9.9" || chart.Type != "library" {
		t.Errorf("existing fields not preserved: %+v", chart)
	}
}

func TestLoadChartMetadata_FlagsOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	existing := ChartFile{APIVersion: "v2", Name: "old", Description: "old-desc", Version: "1.0.0", Type: "application"}
	data, err := yaml.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, chartFileName), data, 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	chart, err := loadChartMetadata(dir, "new-name", "new-desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chart.Name != "new-name" || chart.Description != "new-desc" {
		t.Errorf("flags did not override existing: %+v", chart)
	}
	if chart.Version != "1.0.0" {
		t.Errorf("version should still come from existing chart, got %q", chart.Version)
	}
}

func TestLoadChartMetadata_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, chartFileName), []byte("not yaml: ["), 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
	}
	if _, err := loadChartMetadata(dir, "", ""); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestPartitionRepos_ZeroTTL(t *testing.T) {
	tmp := t.TempDir()
	freshDir := filepath.Join(tmp, "fresh")
	_ = os.MkdirAll(freshDir, 0o755)
	_ = writeMeta(freshDir, Meta{ClonedAt: time.Now()})

	repos := []provider.Repo{{Name: "fresh"}}
	cached, stale := partitionRepos(repos, tmp, 0)
	if len(cached) != 0 {
		t.Errorf("zero TTL should mark every repo stale, got cached=%v", cached)
	}
	if len(stale) != 1 {
		t.Errorf("expected stale entry, got %v", stale)
	}
}

func TestCloneAndCopyStale_NoStale(t *testing.T) {
	tmp := t.TempDir()
	chartsDir := filepath.Join(tmp, ".helm")
	if err := os.MkdirAll(filepath.Join(chartsDir, "charts"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := cloneAndCopyStale(context.Background(), nil, tmp, chartsDir, BootstrapOptions{Quiet: true})
	if err != nil {
		t.Fatalf("expected no-op success, got %v", err)
	}
}

func TestOutWriter_DefaultsToStdout(t *testing.T) {
	if got := outWriter(BootstrapOptions{}); got != os.Stdout {
		t.Errorf("nil Out should default to os.Stdout, got %T", got)
	}
	var buf bytes.Buffer
	if got := outWriter(BootstrapOptions{Out: &buf}); got != &buf {
		t.Errorf("explicit Out should be returned as-is")
	}
}

func TestLogf_QuietSuppresses(t *testing.T) {
	var buf bytes.Buffer
	logf(BootstrapOptions{Out: &buf, Quiet: true}, "hello %s", "world")
	if buf.Len() != 0 {
		t.Errorf("quiet should suppress output, got %q", buf.String())
	}

	logf(BootstrapOptions{Out: &buf}, "hello %s", "world")
	if buf.String() != "hello world" {
		t.Errorf("expected formatted output, got %q", buf.String())
	}
}

func TestUpdate_PublicWrapperRejectsBadCacheDir(t *testing.T) {
	err := Update(context.Background(), BootstrapOptions{CacheDir: "relative/cache"})
	if err == nil {
		t.Fatal("expected error for relative cache_dir via public Update")
	}
}

func TestUpdate_PublicWrapperRejectsBadChartsDir(t *testing.T) {
	err := Update(context.Background(), BootstrapOptions{ChartsDir: "/abs/charts"})
	if err == nil {
		t.Fatal("expected error for absolute charts_dir via public Update")
	}
}

func FuzzIsValidRepoName(f *testing.F) {
	for _, seed := range []string{"foo", "", ".", "..", "../etc", "foo/bar", "foo\\bar", ".hidden", "foo-bar_1"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		valid := isValidRepoName(name)
		if !valid {
			return
		}
		if name == "" || name == "." || name == ".." {
			t.Fatalf("validator accepted forbidden literal %q", name)
		}
		if strings.Contains(name, "..") {
			t.Fatalf("validator accepted name containing '..': %q", name)
		}
		if strings.ContainsAny(name, "/\\") {
			t.Fatalf("validator accepted path separator in: %q", name)
		}
		if strings.HasPrefix(name, ".") {
			t.Fatalf("validator accepted hidden-style name: %q", name)
		}
	})
}

func FuzzNormalizeChartsDir(f *testing.F) {
	for _, seed := range []string{"", ".helm", "charts", "../escape", "/abs", "a/b/c"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, dir string) {
		got, err := normalizeChartsDir(dir)
		if err != nil {
			return
		}
		if filepath.IsAbs(got) {
			t.Fatalf("normalized dir is absolute: %q (input %q)", got, dir)
		}
		if strings.Contains(got, "..") {
			t.Fatalf("normalized dir contains '..': %q (input %q)", got, dir)
		}
	})
}

func FuzzValidateAbsCachePath(f *testing.F) {
	for _, seed := range []string{"", "/tmp/cache", "/a/../b", "relative", "/a"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, path string) {
		got, err := validateAbsCachePath(path, "fuzz")
		if err != nil {
			return
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("accepted non-absolute path: %q (input %q)", got, path)
		}
		if strings.Contains(got, "..") {
			t.Fatalf("accepted path with '..': %q (input %q)", got, path)
		}
	})
}
