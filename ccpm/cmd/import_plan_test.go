package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
)

// seedImportFixture isolates HOME, creates a minimal ~/.claude with one skill,
// and returns a config with one registered profile.
func seedImportFixture(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	skill := filepath.Join(home, ".claude", "skills", "demo-skill")
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("demo"), 0o600); err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(home, ".ccpm", "profiles", "work")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Version: "1", Profiles: map[string]config.ProfileConfig{}}
	cfg.AddProfile("work", profileDir, "api_key")
	return cfg
}

func TestResolveImportPlanValidation(t *testing.T) {
	cfg := seedImportFixture(t)

	cases := []struct {
		name  string
		state importDefaultState
	}{
		{"profile and all", importDefaultState{profile: "work", all: true}},
		{"live-symlinks with no-share", importDefaultState{profile: "work", liveSymlinks: true, noShare: true}},
		{"live-symlink flag conflict", importDefaultState{profile: "work", liveSymlinks: true, noLiveSymlink: true}},
		{"bad mcp scope", importDefaultState{profile: "work", mcpScope: "bogus"}},
		{"unknown profile", importDefaultState{profile: "nope", only: []string{"skills"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveImportPlan(&tc.state, cfg); err == nil {
				t.Errorf("resolveImportPlan(%+v) = nil error, want validation error", tc.state)
			}
		})
	}
}

func TestResolveImportPlanDropsRunnableTargetsByDefault(t *testing.T) {
	cfg := seedImportFixture(t)

	state := importDefaultState{
		profile:       "work",
		only:          []string{"skills", "hooks", "mcp"},
		selectAll:     true,
		noLiveSymlink: true,
	}
	plan, err := resolveImportPlan(&state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range plan.targets {
		if target == defaultclaude.TargetHooks || target == defaultclaude.TargetMCP {
			t.Errorf("runnable target %s survived without --include-runnable", target)
		}
	}
	if len(plan.targets) != 1 || plan.targets[0] != defaultclaude.TargetSkills {
		t.Errorf("targets = %v, want [skills]", plan.targets)
	}
	if len(plan.profiles) != 1 || plan.profiles[0] != "work" {
		t.Errorf("profiles = %v, want [work]", plan.profiles)
	}
}
