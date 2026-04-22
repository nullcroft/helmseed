package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "helmseed.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ValidGitHub(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
provider: github
group: myorg
token: ghp_fake
`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Provider != ProviderGitHub {
		t.Errorf("provider = %q, want github", c.Provider)
	}
	if c.Group != "myorg" {
		t.Errorf("group = %q, want myorg", c.Group)
	}
	if c.Token != "ghp_fake" {
		t.Errorf("token = %q, want ghp_fake", c.Token)
	}
}

func TestLoad_ValidGitLab(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
provider: gitlab
group: mygroup/subgroup
token: glpat-fake
base_url: https://gitlab.example.com
`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Provider != ProviderGitLab {
		t.Errorf("provider = %q, want gitlab", c.Provider)
	}
	if c.BaseURL != "https://gitlab.example.com" {
		t.Errorf("base_url = %q, want https://gitlab.example.com", c.BaseURL)
	}
}

func TestLoad_MissingProvider(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
group: myorg
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestLoad_UnsupportedProvider(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
provider: bitbucket
group: myorg
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestLoad_MissingGroup(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
provider: github
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing group")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
provider: github
group: myorg
token: original
`)

	t.Setenv("HELMSEED_TOKEN", "env-token-123")

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Token != "env-token-123" {
		t.Errorf("token = %q, want env-token-123", c.Token)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	viper.Reset()
	_, err := Load("/tmp/nonexistent-helmseed-config-12345.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
}

func TestLoad_DefaultCacheTTL(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	path := writeConfig(t, dir, `
provider: github
group: myorg
`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.CacheTTL != 24*60*60*1e9 { // 24h in nanoseconds
		t.Errorf("cache_ttl = %v, want 24h", c.CacheTTL)
	}
}
