package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// requireProfileFlag registers a required --profile flag bound to target.
func requireProfileFlag(cmd *cobra.Command, target *string, usage string) {
	cmd.Flags().StringVar(target, "profile", "", usage)
	_ = cmd.MarkFlagRequired("profile")
}

// loadProfile loads the config and looks up the named profile, returning the
// standard "profile %q not found" error when it is absent.
func loadProfile(name string) (*config.Config, config.ProfileConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.ProfileConfig{}, fmt.Errorf("loading config: %w", err)
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, config.ProfileConfig{}, fmt.Errorf("profile %q not found", name)
	}
	return cfg, p, nil
}
