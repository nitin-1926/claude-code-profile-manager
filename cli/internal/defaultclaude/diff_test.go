package defaultclaude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeProfileDiffs(t *testing.T) {
	tmp := t.TempDir()

	defaultRoot := filepath.Join(tmp, "default-claude")
	mustMkdir(t, filepath.Join(defaultRoot, "skills", "shared-skill"))
	mustMkdir(t, filepath.Join(defaultRoot, "skills", "unadopted-skill"))
	mustMkdir(t, filepath.Join(defaultRoot, "hooks", "shared-hook"))
	mustMkdir(t, filepath.Join(defaultRoot, "agents"))

	workDir := filepath.Join(tmp, "profiles", "work")
	mustMkdir(t, filepath.Join(workDir, "skills", "shared-skill"))
	mustMkdir(t, filepath.Join(workDir, "skills", "only-in-work"))
	mustMkdir(t, filepath.Join(workDir, "hooks", "shared-hook"))

	personalDir := filepath.Join(tmp, "profiles", "personal")
	mustMkdir(t, filepath.Join(personalDir, "skills", "only-in-personal"))

	diffs := ComputeProfileDiffs(defaultRoot, map[string]string{
		"work":     workDir,
		"personal": personalDir,
	}, []Target{TargetSkills, TargetHooks, TargetAgents, TargetSettings})

	byTarget := map[Target]ProfileDiff{}
	for _, d := range diffs {
		byTarget[d.Target] = d
	}

	// Settings target should be excluded.
	if _, ok := byTarget[TargetSettings]; ok {
		t.Error("settings target should not be produced by ComputeProfileDiffs")
	}

	// Skills: unadopted-skill is in default only; shared-skill is in both.
	sk := byTarget[TargetSkills]
	if len(sk.OnlyInDefault) != 1 || sk.OnlyInDefault[0] != "unadopted-skill" {
		t.Errorf("skills.OnlyInDefault = %v, want [unadopted-skill]", sk.OnlyInDefault)
	}
	if len(sk.SharedAdopted) != 1 || sk.SharedAdopted[0] != "shared-skill" {
		t.Errorf("skills.SharedAdopted = %v, want [shared-skill]", sk.SharedAdopted)
	}
	if got := sk.OnlyInProfiles["work"]; len(got) != 1 || got[0] != "only-in-work" {
		t.Errorf("skills.OnlyInProfiles[work] = %v, want [only-in-work]", got)
	}
	if got := sk.OnlyInProfiles["personal"]; len(got) != 1 || got[0] != "only-in-personal" {
		t.Errorf("skills.OnlyInProfiles[personal] = %v, want [only-in-personal]", got)
	}

	// Hooks: shared in both profiles (well, adopted in work) so OnlyInDefault is empty.
	hk := byTarget[TargetHooks]
	if len(hk.OnlyInDefault) != 0 {
		t.Errorf("hooks.OnlyInDefault = %v, want []", hk.OnlyInDefault)
	}

	// Agents: default has an empty dir, no items.
	ag := byTarget[TargetAgents]
	if len(ag.OnlyInDefault) != 0 {
		t.Errorf("agents.OnlyInDefault = %v, want []", ag.OnlyInDefault)
	}

	if !HasUnadopted(diffs) {
		t.Error("HasUnadopted should be true (unadopted-skill)")
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}
