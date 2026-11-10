// Package presets is the table of third-party providers a profile can be
// pointed at with `ccpm add <name> --preset <id>`. To add a provider, add one
// entry to all below; the CLI, the desktop app and the validation test pick it
// up from there.
//
// Every row comes from the provider's own Claude Code page, recorded in
// SourceURL. Leave model env unset where that page states a working default:
// model names go stale faster than anything else in this file.
package presets

import (
	"fmt"
	"strings"
)

// Preset describes one provider. Nothing here is secret: a user's key goes to
// the keystore, and only KeyEnv records how it reaches claude.
type Preset struct {
	ID          string
	Label       string
	Description string
	SourceURL   string
	// Env is copied onto the profile. It always carries ANTHROPIC_BASE_URL.
	Env map[string]string
	// KeyEnv is the variable claude reads the key from.
	KeyEnv string
	// KeyRequired means the user must supply a key. Otherwise KeyValue is the
	// fixed placeholder the provider documents, stored the same way so a launch
	// has one path.
	KeyRequired bool
	KeyValue    string
}

const (
	baseURLEnv   = "ANTHROPIC_BASE_URL"
	authTokenEnv = "ANTHROPIC_AUTH_TOKEN"
)

var all = []Preset{
	{
		ID:          "ollama",
		Label:       "Ollama (local)",
		Description: "Models on your own Ollama server (v0.14.0 or later). Needs no key. Choose a model with: ccpm run <name> --model <model>, or for every launch: ccpm env set ANTHROPIC_MODEL=<model> --profile <name>.",
		SourceURL:   "https://docs.ollama.com/integrations/claude-code",
		Env:         map[string]string{baseURLEnv: "http://localhost:11434"},
		KeyEnv:      authTokenEnv,
		KeyValue:    "ollama",
	},
	{
		ID:          "ollama-cloud",
		Label:       "Ollama Cloud",
		Description: "Ollama's hosted models, no local install. Choose a cloud model with: ccpm run <name> --model <model>, or for every launch: ccpm env set ANTHROPIC_MODEL=<model> --profile <name>.",
		SourceURL:   "https://docs.ollama.com/integrations/claude-code",
		Env:         map[string]string{baseURLEnv: "https://ollama.com"},
		KeyEnv:      authTokenEnv,
		KeyRequired: true,
	},
	{
		ID:          "openrouter",
		Label:       "OpenRouter",
		Description: "OpenRouter's Anthropic-compatible gateway, billed to your OpenRouter credits. OpenRouter guarantees Claude Code only with the Anthropic first-party provider.",
		SourceURL:   "https://openrouter.ai/docs/guides/coding-agents/claude-code-integration",
		Env:         map[string]string{baseURLEnv: "https://openrouter.ai/api"},
		KeyEnv:      authTokenEnv,
		KeyRequired: true,
	},
	{
		ID:          "deepseek",
		Label:       "DeepSeek",
		Description: "DeepSeek's Anthropic API. Claude model names map to DeepSeek models on its side.",
		SourceURL:   "https://api-docs.deepseek.com/quick_start/agent_integrations/claude_code",
		Env:         map[string]string{baseURLEnv: "https://api.deepseek.com/anthropic"},
		KeyEnv:      authTokenEnv,
		KeyRequired: true,
	},
	{
		ID:          "zai",
		Label:       "Z.ai GLM",
		Description: "Z.ai's GLM models through its GLM Coding Plan. Claude Code's model tiers map to GLM on its side.",
		SourceURL:   "https://docs.z.ai/devpack/tool/claude",
		Env: map[string]string{
			baseURLEnv:       "https://api.z.ai/api/anthropic",
			"API_TIMEOUT_MS": "3000000",
		},
		KeyEnv:      authTokenEnv,
		KeyRequired: true,
	},
	{
		// Kimi's page says each unset model tier fails, so its block ships whole.
		ID:          "kimi",
		Label:       "Moonshot Kimi",
		Description: "Moonshot's Kimi models. Change the models with: ccpm env set ANTHROPIC_MODEL=<model> --profile <name>.",
		SourceURL:   "https://platform.kimi.ai/docs/guide/claude-code-kimi",
		Env: map[string]string{
			baseURLEnv:                        "https://api.moonshot.ai/anthropic",
			"ANTHROPIC_MODEL":                 "kimi-k3[1m]",
			"ANTHROPIC_DEFAULT_OPUS_MODEL":    "kimi-k3[1m]",
			"ANTHROPIC_DEFAULT_SONNET_MODEL":  "kimi-k3[1m]",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL":   "kimi-k2.7-code",
			"ANTHROPIC_DEFAULT_FABLE_MODEL":   "kimi-k3[1m]",
			"CLAUDE_CODE_SUBAGENT_MODEL":      "kimi-k3[1m]",
			"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "1000000",
			"CLAUDE_CODE_EFFORT_LEVEL":        "max",
		},
		KeyEnv:      authTokenEnv,
		KeyRequired: true,
	},
	{
		// DeepInfra's page maps every slot and states no default for Claude
		// names, so its recommended block ships whole. Its dated slugs are the
		// first thing here to go stale.
		ID:          "deepinfra",
		Label:       "DeepInfra",
		Description: "DeepInfra's Anthropic-compatible API with its recommended model for each slot. Change the models with: ccpm env set ANTHROPIC_MODEL=<model> --profile <name>.",
		SourceURL:   "https://docs.deepinfra.com/integrations/anthropic",
		Env: map[string]string{
			baseURLEnv:                       "https://api.deepinfra.com/anthropic",
			"ANTHROPIC_MODEL":                "deepseek-ai/DeepSeek-V4.1-Flash-1017[1m]",
			"ANTHROPIC_DEFAULT_FABLE_MODEL":  "moonshotai/Kimi-K3[1m]",
			"ANTHROPIC_DEFAULT_OPUS_MODEL":   "deepseek-ai/DeepSeek-V4-Pro-0813[1m]",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "deepseek-ai/DeepSeek-V4.1-Flash-1017[1m]",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "inclusionAI/Ling-3.0-flash",
			"CLAUDE_CODE_SUBAGENT_MODEL":     "inclusionAI/Ling-3.0-flash",
			"CLAUDE_CODE_MAX_CONTEXT_TOKENS": "131072",
			"CLAUDE_CODE_MAX_OUTPUT_TOKENS":  "16384",
		},
		KeyEnv:      authTokenEnv,
		KeyRequired: true,
	},
}

// All returns every preset in display order. Callers must not modify it.
func All() []Preset { return all }

// Get returns the preset with this id, or an error naming the valid ones.
func Get(id string) (Preset, error) {
	ids := make([]string, 0, len(all))
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
		ids = append(ids, p.ID)
	}
	return Preset{}, fmt.Errorf("unknown preset %q. Available presets: %s", id, strings.Join(ids, ", "))
}

// BaseURL returns the endpoint this preset points claude at.
func (p Preset) BaseURL() string { return p.Env[baseURLEnv] }

// RoutingEnv reports whether an env var decides where claude sends requests,
// which credential it sends, or which model it asks for. A provider profile
// owns these outright: Anthropic documents CLAUDE_CODE_USE_* as outranking
// ANTHROPIC_AUTH_TOKEN, and a stray ANTHROPIC_API_KEY or ANTHROPIC_CUSTOM_HEADERS
// from the shell must not reach a third party.
func RoutingEnv(key string) bool {
	key = strings.ToUpper(key) // Windows env keys are case-insensitive
	if strings.HasPrefix(key, "ANTHROPIC_") {
		return true
	}
	switch key {
	case "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY",
		"CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_SUBAGENT_MODEL":
		return true
	}
	return false
}
