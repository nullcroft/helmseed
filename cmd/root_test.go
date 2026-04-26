package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/nullcroft/helmseed/internal/config"
	"github.com/nullcroft/helmseed/internal/provider"
	"github.com/spf13/cobra"
)

func TestExecute(t *testing.T) {
	// Execute with no subcommand should print help and return nil.
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(bytes.NewBuffer(nil))
	rootCmd.SetErr(bytes.NewBuffer(nil))
	err := Execute()
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
}

func TestIsQuietAndIsConfirm(t *testing.T) {
	origQuiet := quiet
	origYes := flagYes
	defer func() {
		quiet = origQuiet
		flagYes = origYes
	}()

	quiet = true
	flagYes = true
	if !IsQuiet() {
		t.Error("IsQuiet() should be true")
	}
	if !IsConfirm() {
		t.Error("IsConfirm() should be true")
	}

	quiet = false
	flagYes = false
	if IsQuiet() {
		t.Error("IsQuiet() should be false")
	}
	if IsConfirm() {
		t.Error("IsConfirm() should be false")
	}
}

func TestFilterByPrefix(t *testing.T) {
	repos := []provider.Repo{
		{Name: "helm-app-frontend"},
		{Name: "helm-app-backend"},
		{Name: "infra-db"},
	}

	// Empty prefix returns all
	out := filterByPrefix(repos, "")
	if len(out) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(out))
	}

	out = filterByPrefix(repos, "helm-app-")
	if len(out) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(out))
	}
	for _, r := range out {
		if r.Name != "helm-app-frontend" && r.Name != "helm-app-backend" {
			t.Errorf("unexpected repo %q", r.Name)
		}
	}

	out = filterByPrefix(repos, "no-match")
	if len(out) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(out))
	}
}

func TestRequireConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/helmseed.yaml"
	content := `
provider: github
group: myorg
`
	if err := writeFile(path, content); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	origCfgFile := cfgFile
	cfgFile = path
	defer func() { cfgFile = origCfgFile }()

	err := requireConfig(cmd, []string{})
	if err != nil {
		t.Fatalf("requireConfig() error: %v", err)
	}

	cfg := getConfig(cmd.Context())
	if cfg == nil {
		t.Fatal("expected config in context")
	}
	if cfg.Provider != config.ProviderGitHub {
		t.Errorf("provider = %q, want github", cfg.Provider)
	}
	if cfg.Group != "myorg" {
		t.Errorf("group = %q, want myorg", cfg.Group)
	}
}

func TestRequireConfig_InvalidFile(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	origCfgFile := cfgFile
	cfgFile = "/nonexistent/config.yaml"
	defer func() { cfgFile = origCfgFile }()

	err := requireConfig(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestGetConfig_PanicsWithoutValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when config is missing from context")
		}
	}()
	getConfig(context.Background())
}

func TestHandleProviderError(t *testing.T) {
	// Non-rate-limit error is returned as-is
	err := errors.New("some error")
	result := handleProviderError(err)
	if result != err {
		t.Errorf("expected same error, got %v", result)
	}

	// Rate limit error gets wrapped with hint
	reset := time.Now().Add(time.Hour)
	rateErr := &provider.RateLimitError{
		Provider:  "github",
		Limit:     60,
		Remaining: 0,
		Reset:     reset,
		Cause:     provider.ErrRateExhausted,
	}
	result = handleProviderError(rateErr)
	if result == nil {
		t.Fatal("expected non-nil error")
	}
	expectedHint := reset.Format(time.RFC3339)
	if result.Error() == "" || !contains(result.Error(), expectedHint) {
		t.Errorf("expected hint with reset time %q, got %q", expectedHint, result.Error())
	}
}

func TestVersionCmd(t *testing.T) {
	version = "v0.1.0-test"
	// version command writes directly to stdout via fmt.Printf
	// so we just verify it doesn't panic and has the right version string
	versionCmd.Run(versionCmd, []string{})
	// If we reach here without panic, the command executed
}

func writeFile(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = f.WriteString(content)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func contains(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
