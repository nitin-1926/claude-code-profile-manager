package presets

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// secretLike matches provider key prefixes and long unbroken tokens. Model slugs
// and URLs stay under it because their dots and slashes break the run.
var secretLike = regexp.MustCompile(`(?i)\b(sk|pk|ghp|xox[a-z])[-_]|[A-Za-z0-9_-]{32,}`)

func validate(p Preset) error {
	if p.ID == "" || p.Label == "" || p.Description == "" {
		return fmt.Errorf("id, label and description are required")
	}
	if strings.Contains(p.Label+p.Description, "—") {
		return fmt.Errorf("user-visible text contains an em-dash")
	}
	if u, err := url.Parse(p.SourceURL); err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("source URL %q must be https", p.SourceURL)
	}
	base, err := url.Parse(p.BaseURL())
	if err != nil || base.Host == "" {
		return fmt.Errorf("base URL %q is missing or invalid", p.BaseURL())
	}
	local := base.Hostname() == "localhost" || base.Hostname() == "127.0.0.1"
	if base.Scheme != "https" && !local {
		return fmt.Errorf("base URL %q must be https unless it is a local host", p.BaseURL())
	}
	if p.KeyEnv != "ANTHROPIC_AUTH_TOKEN" && p.KeyEnv != "ANTHROPIC_API_KEY" {
		return fmt.Errorf("key env %q is not a credential variable", p.KeyEnv)
	}
	if p.KeyRequired == (p.KeyValue != "") {
		return fmt.Errorf("exactly one of KeyRequired and KeyValue must be set")
	}
	if secretLike.MatchString(p.KeyValue) {
		return fmt.Errorf("fixed key value looks secret")
	}
	for k, v := range p.Env {
		if k == "ANTHROPIC_AUTH_TOKEN" || k == "ANTHROPIC_API_KEY" {
			return fmt.Errorf("env carries credential variable %s", k)
		}
		if secretLike.MatchString(v) {
			return fmt.Errorf("env %s value %q looks secret", k, v)
		}
	}
	return nil
}

func TestEmbeddedPresetsAreValid(t *testing.T) {
	if len(All()) == 0 {
		t.Fatal("no presets")
	}
	seen := map[string]bool{}
	for _, p := range All() {
		if seen[p.ID] {
			t.Errorf("duplicate id %q", p.ID)
		}
		seen[p.ID] = true
		if err := validate(p); err != nil {
			t.Errorf("%s: %v", p.ID, err)
		}
	}
}

// Without this the test above could pass with a validator that rejects nothing.
func TestValidateRejectsBadPresets(t *testing.T) {
	good := func() Preset {
		return Preset{
			ID: "x", Label: "X", Description: "d", SourceURL: "https://x.example/docs",
			Env:    map[string]string{baseURLEnv: "https://x.example"},
			KeyEnv: authTokenEnv, KeyRequired: true,
		}
	}
	if err := validate(good()); err != nil {
		t.Fatalf("control preset rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Preset){
		"http remote":       func(p *Preset) { p.Env = map[string]string{baseURLEnv: "http://x.example"} },
		"no base url":       func(p *Preset) { p.Env = nil },
		"http source":       func(p *Preset) { p.SourceURL = "http://x.example" },
		"unknown key env":   func(p *Preset) { p.KeyEnv = "MY_TOKEN" },
		"key required+val":  func(p *Preset) { p.KeyValue = "ollama" },
		"neither key mode":  func(p *Preset) { p.KeyRequired = false },
		"credential in env": func(p *Preset) { p.Env["ANTHROPIC_AUTH_TOKEN"] = "abc" },
		"secret prefix":     func(p *Preset) { p.Env["FOO"] = "sk-or-v1-abc" },
		"long token":        func(p *Preset) { p.Env["FOO"] = strings.Repeat("a1", 20) },
		"em-dash":           func(p *Preset) { p.Description = "a — b" },
	} {
		p := good()
		mutate(&p)
		if validate(p) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestGetUnknownListsValidIDs(t *testing.T) {
	if p, err := Get("openrouter"); err != nil || p.BaseURL() != "https://openrouter.ai/api" {
		t.Fatalf("Get(openrouter) = %+v, %v", p, err)
	}
	_, err := Get("nope")
	if err == nil {
		t.Fatal("unknown id accepted")
	}
	for _, p := range All() {
		if !strings.Contains(err.Error(), p.ID) {
			t.Errorf("error %q does not list %q", err, p.ID)
		}
	}
}

func TestRoutingEnv(t *testing.T) {
	for key, want := range map[string]bool{
		"ANTHROPIC_API_KEY":             true,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":  true,
		"anthropic_base_url":            true,
		"CLAUDE_CODE_USE_BEDROCK":       true,
		"CLAUDE_CODE_OAUTH_TOKEN":       true,
		"CLAUDE_CONFIG_DIR":             false,
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS": false,
		"PATH":                          false,
	} {
		if got := RoutingEnv(key); got != want {
			t.Errorf("RoutingEnv(%s) = %v, want %v", key, got, want)
		}
	}
}
