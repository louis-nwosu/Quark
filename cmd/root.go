package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/louis-nwosu/Quark/internal/agent"
	"github.com/louis-nwosu/Quark/internal/config"
	"github.com/louis-nwosu/Quark/internal/llm"
	"github.com/louis-nwosu/Quark/internal/tools"
	"github.com/louis-nwosu/Quark/internal/ui"
)

func Execute() {
	prompt := flag.String("m", "", "Multi-turn interactive mode with initial prompt")
	model := flag.String("model", "", "Override provider/model")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if *model != "" {
		if _, ok := cfg.Providers[*model]; ok {
			cfg.DefaultProvider = *model
		} else if pc, ok := cfg.Providers[cfg.DefaultProvider]; ok {
			pc.Model = *model
			cfg.Providers[cfg.DefaultProvider] = pc
		}
	}

	args := flag.Args()

	provider := newProvider(cfg)
	if provider == nil {
		return
	}
	ag := newAgent(provider, cfg)

	tui := ui.NewTerminal(ag, cfg)

	if len(args) > 0 {
		tui.SendMessage(args[0])
	}

	if *prompt != "" {
		tui.SendMessage(*prompt)
	}

	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ui error: %v\n", err)
		os.Exit(1)
	}
}

func newProvider(cfg *config.Config) llm.Provider {
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		if _, ok := err.(*llm.ErrNoAPIKey); ok {
			fmt.Fprintf(os.Stderr, "no API key configured — run /config inside quark\n")
			return nil
		}
		fmt.Fprintf(os.Stderr, "provider error: %v\n", err)
		return nil
	}
	return provider
}

func newAgent(provider llm.Provider, cfg *config.Config) *agent.Agent {
	reg := tools.NewRegistry()
	ag := agent.New(provider, reg)
	ag.ContextWindow = cfg.ContextWindow()
	if cfg.CompactThreshold > 0 {
		ag.CompactThreshold = cfg.CompactThreshold
	}
	ag.LoadSession()
	return ag
}

func printHelp() {
	fmt.Print(`quark — lightweight terminal coding agent

Usage:
  quark                  Interactive TUI mode
  quark <prompt>         One-shot mode
  quark -m <prompt>      Interactive mode with initial prompt

Flags:
  -m <prompt>        Interactive mode with initial message
  -model <id>        Override provider/model
  -help              Show this help

Config: ~/.config/quark/quark.json or ./.quark.json

Slash commands:
  /clear  Clear conversation history
  /config Open config UI
  /exit   Exit quark
  /help   Show slash commands
`)
}
