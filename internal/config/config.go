package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	OpenRouterFreeModel = "openrouter/free"
)

type ProviderConfig struct {
	APIKey         string `json:"api_key"`
	Model          string `json:"model"`
	BaseURL        string `json:"base_url,omitempty"`
	ContextWindow  int    `json:"context_window,omitempty"`
	SummarizeModel string `json:"summarize_model,omitempty"`
}

type Config struct {
	Providers        map[string]ProviderConfig `json:"providers"`
	DefaultProvider  string                    `json:"default_provider"`
	MaxToolRounds    int                       `json:"max_tool_rounds"`
	CompactThreshold int                       `json:"compact_threshold"`
	BashTimeout      int                       `json:"bash_timeout"`
}

func Default() *Config {
	return &Config{
		Providers: map[string]ProviderConfig{
			"openrouter": {
				Model:         OpenRouterFreeModel,
				BaseURL:       "https://openrouter.ai/api/v1",
				ContextWindow: 128000,
			},
			"openai": {
				Model:         "gpt-4o",
				BaseURL:       "https://api.openai.com/v1",
				ContextWindow: 128000,
			},
			"anthropic": {
				Model:         "claude-sonnet-4-20250514",
				BaseURL:       "https://api.anthropic.com",
				ContextWindow: 200000,
			},
		},
		DefaultProvider:  "openrouter",
		MaxToolRounds:    25,
		CompactThreshold: 100,
		BashTimeout:      30,
	}
}

func Load() (*Config, error) {
	cfg := Default()

	if data, err := os.ReadFile(".quark.json"); err == nil {
		if err := mergeJSON(cfg, data); err != nil {
			return nil, fmt.Errorf(".quark.json: %w", err)
		}
		return ensureOpenRouter(cfg), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ensureOpenRouter(applyEnvOverrides(cfg)), nil
	}
	path := filepath.Join(home, ".config", "quark", "quark.json")
	if data, err := os.ReadFile(path); err == nil {
		if err := mergeJSON(cfg, data); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}

	cfg = applyEnvOverrides(cfg)
	return ensureOpenRouter(cfg), nil
}

func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	// If .quark.json exists in the current directory, save there
	// (it takes priority on load, so the saved data must go there to persist)
	if _, err := os.Stat(".quark.json"); err == nil {
		return os.WriteFile(".quark.json", data, 0600)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "quark")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(dir, "quark.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (c *Config) HasUserAPIKey() bool {
	for name, pc := range c.Providers {
		if name == "openrouter" && pc.APIKey == "" {
			continue
		}
		if pc.APIKey != "" {
			return true
		}
	}
	return false
}

func (c *Config) IsUsingFreeModel() bool {
	return c.DefaultProvider == "openrouter" &&
		c.Providers["openrouter"].APIKey == ""
}

func (c *Config) ProviderNames() []string {
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) ContextWindow() int {
	if pc, ok := c.Providers[c.DefaultProvider]; ok && pc.ContextWindow > 0 {
		return pc.ContextWindow
	}
	return 128000
}

func (c *Config) ProviderStatus(name string) string {
	pc, ok := c.Providers[name]
	if !ok {
		return "not configured"
	}
	if name == "openrouter" && pc.APIKey == "" {
		return "free model"
	}
	if pc.APIKey != "" {
		return "key set"
	}
	return "no key"
}

func ensureOpenRouter(cfg *Config) *Config {
	if _, ok := cfg.Providers["openrouter"]; !ok {
		if cfg.Providers == nil {
			cfg.Providers = make(map[string]ProviderConfig)
		}
		cfg.Providers["openrouter"] = ProviderConfig{
			Model:   OpenRouterFreeModel,
			BaseURL: "https://openrouter.ai/api/v1",
		}
	}
	if !cfg.HasUserAPIKey() {
		cfg.DefaultProvider = "openrouter"
	}
	return cfg
}

func mergeJSON(cfg *Config, data []byte) error {
	return json.Unmarshal(data, cfg)
}

func applyEnvOverrides(cfg *Config) *Config {
	for name, pc := range cfg.Providers {
		switch name {
		case "openai":
			if v := os.Getenv("OPENAI_API_KEY"); v != "" {
				pc.APIKey = v
				cfg.Providers[name] = pc
			}
		case "anthropic":
			if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
				pc.APIKey = v
				cfg.Providers[name] = pc
			}
		case "openrouter":
			if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
				pc.APIKey = v
				cfg.Providers[name] = pc
			}
		}
	}
	return cfg
}
