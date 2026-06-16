package plugins

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"a", "compound-engineering", "My.Plugin_2", "x" + strings.Repeat("y", 127)}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{
		"",
		"..",
		"../escape",
		"a/b",
		`a\b`,
		".hidden",
		"-flag",
		"--upload-pack=evil",
		"name with spaces",
		"x" + strings.Repeat("y", 128), // too long
		"a\x00b",
		"a\nb",
	}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", name)
		}
	}
}

func TestValidateVersion(t *testing.T) {
	valid := []string{"0.0.0", "1.2.3-beta.1", "2026.06.10+build5", "v1"}
	for _, v := range valid {
		if err := ValidateVersion(v); err != nil {
			t.Errorf("ValidateVersion(%q) = %v, want nil", v, err)
		}
	}
	invalid := []string{"", "../1.0", "1.0/x", "-1", ".1", "1 0"}
	for _, v := range invalid {
		if err := ValidateVersion(v); err == nil {
			t.Errorf("ValidateVersion(%q) = nil, want error", v)
		}
	}
}

func TestValidateRef(t *testing.T) {
	valid := []string{"", "main", "release/v1.2", "feature_x", "v2.0.0"}
	for _, ref := range valid {
		if err := ValidateRef(ref); err != nil {
			t.Errorf("ValidateRef(%q) = %v, want nil", ref, err)
		}
	}
	invalid := []string{
		"-w /tmp/pwned",
		"--upload-pack=touch /tmp/x",
		"a..b",
		"with space",
		"tab\there",
		"ctrl\x01char",
		"colon:ref",
		"care^t",
		"til~de",
		`back\slash`,
		strings.Repeat("a", 129),
	}
	for _, ref := range invalid {
		if err := ValidateRef(ref); err == nil {
			t.Errorf("ValidateRef(%q) = nil, want error", ref)
		}
	}
}

func TestValidateSHA(t *testing.T) {
	valid := []string{"", "abc1234", "ABCDEF1", strings.Repeat("a1", 20)}
	for _, sha := range valid {
		if err := ValidateSHA(sha); err != nil {
			t.Errorf("ValidateSHA(%q) = %v, want nil", sha, err)
		}
	}
	invalid := []string{
		"-u /some/path",
		"abc123", // too short (6)
		strings.Repeat("a", 41),
		"ghijklm", // non-hex
		"abc 1234",
		"HEAD",
		"main",
	}
	for _, sha := range invalid {
		if err := ValidateSHA(sha); err == nil {
			t.Errorf("ValidateSHA(%q) = nil, want error", sha)
		}
	}
}

func TestValidateRepo(t *testing.T) {
	valid := []string{"org/repo", "a-b/c.d", "User_1/Repo-2"}
	for _, r := range valid {
		if err := ValidateRepo(r); err != nil {
			t.Errorf("ValidateRepo(%q) = %v, want nil", r, err)
		}
	}
	invalid := []string{"", "norepo", "a/b/c", "-org/repo", "org/-repo", "../x/y", "org/repo extra"}
	for _, r := range invalid {
		if err := ValidateRepo(r); err == nil {
			t.Errorf("ValidateRepo(%q) = nil, want error", r)
		}
	}
}

func TestValidateGitURL(t *testing.T) {
	valid := []string{
		"https://github.com/org/repo.git",
		"https://gitlab.example.com/group/repo",
		"ssh://git@github.com/org/repo.git",
		"git@github.com:org/repo.git",
	}
	for _, u := range valid {
		if err := ValidateGitURL(u); err != nil {
			t.Errorf("ValidateGitURL(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{
		"",
		"file:///etc/passwd",
		"ext::sh -c id",
		"/local/path",
		"./relative",
		"-https://github.com/x/y",
		"http://insecure.example.com/repo", // plain http not allowed
		"git@github.com:org/repo; rm -rf /",
	}
	for _, u := range invalid {
		if err := ValidateGitURL(u); err == nil {
			t.Errorf("ValidateGitURL(%q) = nil, want error", u)
		}
	}
}

func TestValidateLocalPath(t *testing.T) {
	valid := []string{"plugins/foo", "a/b/c", "single"}
	for _, p := range valid {
		if err := ValidateLocalPath(p); err != nil {
			t.Errorf("ValidateLocalPath(%q) = %v, want nil", p, err)
		}
	}
	invalid := []string{"", "../escape", "a/../../b", "/abs/path"}
	for _, p := range invalid {
		if err := ValidateLocalPath(p); err == nil {
			t.Errorf("ValidateLocalPath(%q) = nil, want error", p)
		}
	}
}

// TestResolveSourceRejectsMaliciousManifests exercises the trust boundary the
// way it is hit in production: raw marketplace.json source values.
func TestResolveSourceRejectsMaliciousManifests(t *testing.T) {
	cases := []struct {
		name    string
		source  string // raw JSON for the "source" field
		wantErr bool
	}{
		{"local ok", `"./plugins/foo"`, false},
		{"local traversal", `"../../../home/user/.ssh"`, true},
		{"local absolute", `"/etc"`, true},
		{"github ok", `{"source":"github","repo":"org/repo","ref":"main"}`, false},
		{"github flag-injection ref", `{"source":"github","repo":"org/repo","ref":"--upload-pack=id"}`, true},
		{"github flag-injection sha", `{"source":"github","repo":"org/repo","sha":"-u/tmp/x"}`, true},
		{"github bad repo", `{"source":"github","repo":"-org/repo"}`, true},
		{"url ok", `{"source":"url","url":"https://github.com/org/repo.git"}`, false},
		{"url file scheme", `{"source":"url","url":"file:///etc/passwd"}`, true},
		{"url ext scheme", `{"source":"url","url":"ext::sh -c id"}`, true},
		{"git-subdir ok", `{"source":"git-subdir","url":"https://example.com/r.git","path":"plugins/x"}`, false},
		{"git-subdir traversal path", `{"source":"git-subdir","url":"https://example.com/r.git","path":"../../escape"}`, true},
		{"unknown kind", `{"source":"carrier-pigeon"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := MarketplacePluginSpec{Name: "p", RawSource: json.RawMessage(tc.source)}
			_, err := spec.ResolveSource()
			if tc.wantErr && err == nil {
				t.Errorf("ResolveSource(%s) = nil, want error", tc.source)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ResolveSource(%s) = %v, want nil", tc.source, err)
			}
		})
	}
}

func TestCloneRepoRejectsBadInputsBeforeExec(t *testing.T) {
	// These must fail validation — never reaching git — so a missing git
	// binary or network cannot mask the rejection.
	if err := CloneRepo("file:///etc", t.TempDir()+"/d", "", false); err == nil {
		t.Error("CloneRepo with file:// URL succeeded, want validation error")
	}
	if err := CloneRepo("https://example.com/r.git", t.TempDir()+"/d", "-c core.fsmonitor=evil", false); err == nil {
		t.Error("CloneRepo with flag-injection ref succeeded, want validation error")
	}
	if err := CheckoutSHA(t.TempDir(), "--exec=evil"); err == nil {
		t.Error("CheckoutSHA with flag-injection sha succeeded, want validation error")
	}
}

func TestCachePluginDirRejectsTraversal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := CachePluginDir("../escape", "p", "1.0.0"); err == nil {
		t.Error("CachePluginDir with traversal marketplace succeeded, want error")
	}
	if _, err := CachePluginDir("m", "../escape", "1.0.0"); err == nil {
		t.Error("CachePluginDir with traversal plugin succeeded, want error")
	}
	if _, err := CachePluginDir("m", "p", "../escape"); err == nil {
		t.Error("CachePluginDir with traversal version succeeded, want error")
	}
}
