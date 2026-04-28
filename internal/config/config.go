package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type ProviderKind string

const (
	ProviderGitHub ProviderKind = "github"
	ProviderGitLab ProviderKind = "gitlab"
)

func (p ProviderKind) IsValid() bool {
	return p == ProviderGitHub || p == ProviderGitLab
}

type Config struct {
	Provider       ProviderKind  `mapstructure:"provider"`
	Group          string        `mapstructure:"group"`
	Token          string        `mapstructure:"token"`
	BaseURL        string        `mapstructure:"base_url"`
	Prefix         string        `mapstructure:"prefix"`
	CacheTTL       time.Duration `mapstructure:"cache_ttl"`
	ChartsDir      string        `mapstructure:"charts_dir"`
	CacheDir       string        `mapstructure:"cache_dir"`
	NonInteractive bool          `mapstructure:"non_interactive"`
	// Helm chart metadata
	ChartName        string `mapstructure:"chart_name"`
	ChartDescription string `mapstructure:"chart_description"`
}

func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.AddConfigPath(".")
		v.SetConfigName("helmseed")
		v.SetConfigType("yaml")
	}

	v.SetDefault("cache_ttl", "24h")
	v.SetDefault("charts_dir", ".helm")
	v.SetDefault("cache_dir", "")
	v.SetDefault("non_interactive", false)

	v.SetEnvPrefix("HELMSEED")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Config) validate() error {
	if c.Provider == "" {
		return fmt.Errorf("config: 'provider' is required (github or gitlab)")
	}

	if !c.Provider.IsValid() {
		return fmt.Errorf("config: unsupported provider %q, must be 'github' or 'gitlab'", c.Provider)
	}

	if c.Group == "" {
		return fmt.Errorf("config: 'group' is required")
	}

	return nil
}
