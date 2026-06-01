package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

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

	tui := ui.NewTerminal()
	defer tui.Close()

	cfg, err := config.Load()
	if err != nil {
		tui.Printf("config error: %v\n", err)
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if len(args) > 0 {
		provider := newProvider(tui, cfg)
		if provider == nil {
			return
		}
		ag := newAgent(provider, cfg)
		tui.StreamResponse(ctx, ag, args[0])
		return
	}

	if *prompt != "" {
		interactive(ctx, tui, cfg, *prompt)
		return
	}

	interactive(ctx, tui, cfg, "")
}

func interactive(ctx context.Context, tui *ui.Terminal, cfg *config.Config, initial string) {
	provider := newProvider(tui, cfg)
	if provider == nil {
		return
	}
	ag := newAgent(provider, cfg)

	tui.ShowBanner()

	if ag.HasSession() {
		tui.Printf("  \033[38;5;87m↻\033[0m  \033[38;5;245mresumed — %d messages\033[0m\n", ag.MessageCount())
	}
	tui.ShowProviderStatus(cfg)

	if initial != "" {
		tui.StreamResponse(ctx, ag, initial)
	}

	for {
		msg, err := tui.Prompt()
		if err != nil {
			break
		}
		if msg == "" {
			continue
		}

		if strings.HasPrefix(msg, "/") {
			switch msg {
			case "/clear":
				ag.Clear()
				tui.Printf("  conversation cleared\n")
			case "/config":
				tui.ShowConfigUI(cfg)
				provider = newProvider(tui, cfg)
				if provider != nil {
					ag = newAgent(provider, cfg)
				}
				tui.ShowProviderStatus(cfg)
			case "/exit", "/quit":
				tui.Printf("  goodbye\n")
				return
			case "/help", "/?":
				showSlashHelp(tui)
			default:
				tui.Printf("unknown command: %s\n", msg)
			}
			continue
		}

		tui.StreamResponse(ctx, ag, msg)
	}
}

func newProvider(tui *ui.Terminal, cfg *config.Config) llm.Provider {
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		if _, ok := err.(*llm.ErrNoAPIKey); ok {
			name, key := tui.AskAPIKey()
			if key == "" {
				return nil
			}
			pc := cfg.Providers[name]
			pc.APIKey = key
			cfg.Providers[name] = pc
			cfg.DefaultProvider = name
			cfg.Save()
			provider, err = llm.NewProvider(cfg)
			if err != nil {
				tui.Printf("provider error: %v\n", err)
				return nil
			}
			return provider
		}
		tui.Printf("provider error: %v\n", err)
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

func showSlashHelp(tui *ui.Terminal) {
	b := "\033[1m"
	d := "\033[0m"
	tui.Printf("\n %sSlash Commands%s\n\n", b, d)
	tui.Printf("   %s/clear%s   Clear conversation history\n", b, d)
	tui.Printf("   %s/config%s   Open interactive config UI (add API keys, change provider)\n", b, d)
	tui.Printf("   %s/exit%s    Exit quark\n", b, d)
	tui.Printf("   %s/help%s    Show this message\n", b, d)
	tui.Printf("\n")
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
  /config  Open config UI
  /exit    Exit quark
  /help    Show slash commands
`)
}
