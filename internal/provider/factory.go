package provider

import (
	"fmt"

	"github.com/nullcroft/helmseed/internal/config"
)

func New(cfg *config.Config) (Provider, error) {
	switch cfg.Provider {
	case config.ProviderGitHub:
		return newGitHubProvider(cfg.Token), nil
	case config.ProviderGitLab:
		return newGitLabProvider(cfg.Token, cfg.BaseURL)
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}
