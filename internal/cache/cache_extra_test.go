package cache

import (
	"context"
	"os"
	"path/filepath"
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
	ver := readChartVersion(dir)
	if ver != "0.0.0" {
		t.Errorf("version = %q, want 0.0.0", ver)
	}

	// Invalid YAML
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("not yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	ver = readChartVersion(dir)
	if ver != "0.0.0" {
		t.Errorf("version = %q, want 0.0.0", ver)
	}

	// Valid YAML with version
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("version: 1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ver = readChartVersion(dir)
	if ver != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", ver)
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

	lock.Dependencies[0].Repository = "file://charts/a"
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
	if err := os.WriteFile(filepath.Join(src, "meta.json"), []byte("{}"), 0o644); err != nil {
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

	if _, err := os.Stat(filepath.Join(dst, "meta.json")); !os.IsNotExist(err) {
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
	dir := t.TempDir()
	opts := BootstrapOptions{TTL: time.Hour, ChartsDir: dir}
	err := Update(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when lock file is missing")
	}
}

func TestUpdate_EmptyLock(t *testing.T) {
	dir := t.TempDir()
	lock := ChartLock{Generated: time.Now().UTC(), Digest: "", Dependencies: nil}
	data, _ := yaml.Marshal(lock)
	_ = os.WriteFile(filepath.Join(dir, "Chart.lock"), data, 0o644)
	opts := BootstrapOptions{TTL: time.Hour, ChartsDir: dir}
	err := Update(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for empty lock")
	}
}
