package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/macbookpro/quark/internal/agent"
	"github.com/macbookpro/quark/internal/config"
	"golang.org/x/term"
)

type Terminal struct {
	renderer *glamour.TermRenderer
	oldState *term.State
	width    int
}

func NewTerminal() *Terminal {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		renderer = nil
	}

	width := 100
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width = w - 2
	}

	t := &Terminal{renderer: renderer, width: width}
	t.enterRaw()
	return t
}

func (t *Terminal) Close() {
	t.exitRaw()
}

func (t *Terminal) enterRaw() {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			t.oldState = state
		}
	}
}

func (t *Terminal) exitRaw() {
	if t.oldState != nil {
		term.Restore(int(os.Stdin.Fd()), t.oldState)
		t.oldState = nil
	}
}

func (t *Terminal) withCooked(fn func()) {
	t.exitRaw()
	defer t.enterRaw()
	fn()
}

func (t *Terminal) Printf(format string, args ...any) {
	t.withCooked(func() {
		fmt.Printf(format, args...)
	})
}

// ── Rainbow / Gradient colors ────────────────────────────────

// ANSI 256-color palette helpers
func (t *Terminal) color256(c int, s string) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", c, s)
}

func (t *Terminal) bgColor256(c int, s string) string {
	return fmt.Sprintf("\033[48;5;%dm%s\033[0m", c, s)
}

// gradient applies a smooth color transition across text (skips spaces)
func (t *Terminal) gradient(text string, start, end int) string {
	runes := []rune(text)
	n := len(runes)
	if n <= 1 {
		return t.color256(start, text)
	}
	// count only visible chars for gradient steps
	visible := 0
	for _, r := range runes {
		if r != ' ' {
			visible++
		}
	}
	if visible <= 1 {
		visible = n
	}

	var out strings.Builder
	vi := 0
	for _, r := range runes {
		if r == ' ' {
			out.WriteRune(r)
			continue
		}
		ratio := float64(vi) / float64(max(visible-1, 1))
		c := int(float64(start)*(1-ratio) + float64(end)*ratio)
		out.WriteString(fmt.Sprintf("\033[38;5;%dm%c\033[0m", c, r))
		vi++
	}
	return out.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Stunning Welcome ─────────────────────────────────────────

func (t *Terminal) ShowWelcome() {
	t.withCooked(func() {
		// ┌─ top border ─────────────────────────────────┐
		top := "  ┌" + strings.Repeat("─", t.width-4) + "┐"
		fmt.Println()
		fmt.Println(t.cyan(top))

		// empty line inside box
		empty := "  │" + strings.Repeat(" ", t.width-4) + "│"
		fmt.Println(t.dim(empty))

		// Quark logo with gradient
		logo := "  │" + centerText("⚡  q u a r k", t.width-4) + "│"
		fmt.Println(t.gradient(logo, 87, 45)) // bright cyan → blue

		// tagline in dim
		tagline := "  │" + centerText("lightweight terminal coding agent", t.width-4) + "│"
		fmt.Println(t.dim(tagline))

		// empty line
		fmt.Println(t.dim(empty))

		// bottom border
		bottom := "  └" + strings.Repeat("─", t.width-4) + "┘"
		fmt.Println(t.cyan(bottom))

		// spacer
		fmt.Println()
	})
}

// ── Free Model Notice ────────────────────────────────────────

func (t *Terminal) ShowFreeModelNotice() {
	t.withCooked(func() {
		// A colored info card
		cardWidth := t.width - 6
		pad := strings.Repeat(" ", 4)

		fmt.Printf("  %s\n", t.color256(87, "┌"+strings.Repeat("─", cardWidth)+"┐"))

		line := fmt.Sprintf("│%s%s  %s", pad, t.color256(220, "✦"), t.color256(87, "free tier active"))
		line += strings.Repeat(" ", cardWidth+t.width-6-(visibleLen(stripANSI(line))+t.width-cardWidth-4-2))
		// better to just build fixed-width
		fmt.Printf("  │%s%s  %s%s│\n",
			pad, t.color256(220, "✦"), t.color256(87, "free tier active"),
			strings.Repeat(" ", cardWidth-len("free tier active")-4-2))
		fmt.Printf("  │%s%s%s│\n",
			pad, t.color256(245, "  openrouter/free · auto-routes"),
			strings.Repeat(" ", cardWidth-len("  openrouter/free · auto-routes")-2))
		fmt.Printf("  │%s%s%s│\n",
			pad, t.color256(245, "  run /config to add your own API key"),
			strings.Repeat(" ", cardWidth-len("  run /config to add your own API key")-2))

		fmt.Printf("  %s\n", t.color256(87, "└"+strings.Repeat("─", cardWidth)+"┘"))
		fmt.Println()
	})
}

// ── Config UI ────────────────────────────────────────────────

func (t *Terminal) ShowConfigUI(cfg *config.Config) {
	t.exitRaw()
	defer t.enterRaw()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println()
		fmt.Printf("  %s\n", t.cyan("┌──────────────────────────────────────────┐"))
		fmt.Printf("  │%s%s│\n", t.bold("          provider configuration"),
			strings.Repeat(" ", t.width-4-38-2))
		fmt.Printf("  %s\n", t.cyan("└──────────────────────────────────────────┘"))

		names := cfg.ProviderNames()
		for i, name := range names {
			status := cfg.ProviderStatus(name)
			active := ""
			if name == cfg.DefaultProvider {
				active = " ⚡"
			}
			statusColor := t.dim(status)
			if status == "key set" {
				statusColor = t.green(status)
			} else if status == "free model" {
				statusColor = t.cyan(status)
			}

			fmt.Printf("     \033[1m%d\033[0m  %-12s %s%s\n", i+1, name, statusColor, active)
		}

		fmt.Println()
		fmt.Printf("  \033[1m?\033[0m  pick a provider  \033[2m(0 to cancel)\033[0m ")

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)

		if line == "0" || line == "" {
			fmt.Println()
			return
		}

		idx, err := strconv.Atoi(line)
		if err != nil || idx < 1 || idx > len(names) {
			continue
		}

		name := names[idx-1]
		t.configureProvider(reader, cfg, name)
	}
}

func (t *Terminal) configureProvider(reader *bufio.Reader, cfg *config.Config, name string) {
	pc, exists := cfg.Providers[name]
	if !exists {
		pc = config.ProviderConfig{}
	}

	fmt.Println()
	fmt.Printf("  %s  %s\n", t.color256(87, "▸"), t.bold(name))

	currentKey := ""
	if pc.APIKey != "" {
		currentKey = "(set)"
		if name == "openrouter" && pc.APIKey == "" {
			currentKey = "(free tier)"
		}
	}
	fmt.Printf("    %s %s\n", t.dim("current key:"), t.dim(currentKey))

	fmt.Print("  \033[1m?\033[0m  API key  \033[2m(empty to keep)\033[0m ")
	key, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	key = strings.TrimSpace(key)

	if key != "" {
		pc.APIKey = key
		fmt.Printf("    %s\n", t.green("✓ key updated"))
	}

	fmt.Print("  \033[1m?\033[0m  set default? \033[2m(y/n)\033[0m ")
	yn, _ := reader.ReadString('\n')
	yn = strings.TrimSpace(strings.ToLower(yn))
	if yn == "y" || yn == "yes" {
		cfg.DefaultProvider = name
	}

	fmt.Print("  \033[1m?\033[0m  model  \033[2m(empty to keep)\033[0m ")
	model, _ := reader.ReadString('\n')
	model = strings.TrimSpace(model)
	if model != "" {
		pc.Model = model
	}

	cfg.Providers[name] = pc
	cfg.Save()
	fmt.Printf("  %s\n\n", t.green("✓ saved"))
}

// ── Setup (first run) ────────────────────────────────────────

func (t *Terminal) AskAPIKey() (provider, apiKey string) {
	t.exitRaw()
	defer t.enterRaw()

	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("  %s\n", t.color256(87, "┌──────────────────────────────────────────┐"))
	fmt.Printf("  │%s%s│\n", t.bold("             initial setup"),
		strings.Repeat(" ", t.width-4-35-2))
	fmt.Printf("  %s\n", t.color256(87, "└──────────────────────────────────────────┘"))
	fmt.Println("  Pick a provider:")
	fmt.Println("    \033[1m1\033[0m  OpenAI")
	fmt.Println("    \033[1m2\033[0m  Anthropic")
	fmt.Println()

	for {
		fmt.Print("  \033[1m?\033[0m  select ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		switch line {
		case "1", "openai", "OpenAI":
			provider = "openai"
		case "2", "anthropic", "Anthropic":
			provider = "anthropic"
		default:
			continue
		}
		break
	}

	fmt.Println()
	fmt.Printf("  \033[1m?\033[0m  paste your \033[1m%s\033[0m API key: ", provider)
	key, err := reader.ReadString('\n')
	if err != nil {
		return provider, ""
	}
	fmt.Println()

	return provider, strings.TrimSpace(key)
}

// ── Prompt ───────────────────────────────────────────────────

func (t *Terminal) Prompt() (string, error) {
	t.exitRaw()
	defer t.enterRaw()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n\033[38;5;83muser\033[0m \033[1m❯\033[0m ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ── Stream Response ──────────────────────────────────────────

func (t *Terminal) StreamResponse(ctx context.Context, ag *agent.Agent, msg string) {
	// echo user message
	t.withCooked(func() {
		fmt.Printf("  %s\n", t.color256(83, "user"))
		fmt.Printf("  %s\n\n", t.color256(250, msg))
	})

	spinner := t.startSpinner("thinking…")
	events := make(chan agent.Event, 10)
	go ag.Run(ctx, msg, events)

	var streamedHeader bool

	for evt := range events {
		if spinner != nil {
			spinner.Stop()
			spinner = nil
		}

		switch evt.Type {
		case agent.EventTextChunk:
			if !streamedHeader {
				streamedHeader = true
				t.withCooked(func() {
					fmt.Printf("  %s\n", t.color256(240, strings.Repeat("┄", t.width-6)))
					fmt.Printf("  %s\n", t.color256(83, "assistant"))
				})
			}
			// print chunk directly — raw mode disabled at start of withCooked
			t.withCooked(func() {
				fmt.Print(evt.Text)
			})

		case agent.EventText:
			if !streamedHeader {
				t.withCooked(func() {
					fmt.Printf("  %s\n", t.color256(240, strings.Repeat("┄", t.width-6)))
				})
				t.printAssistantMessage(evt.Text)
			} else {
				// final newline after streaming text
				t.withCooked(func() {
					fmt.Println()
					fmt.Println()
				})
				// re-render with glamour
				t.printAssistantMessage(evt.Text)
			}

		case agent.EventToolStart:
			if streamedHeader {
				streamedHeader = false
				t.withCooked(func() {
					fmt.Println()
					fmt.Println()
				})
			}
			t.printToolStart(evt.Tool)
			text := fmt.Sprintf("%s %s", evt.Tool.Name, t.primaryArg(evt.Tool))
			if visibleLen(text) > 60 {
				text = text[:60] + "…"
			}
			spinner = t.startSpinner(text)

		case agent.EventToolResult:
			t.printToolResult(evt.ToolResult)
			spinner = t.startSpinner("thinking…")

		case agent.EventToolError:
			t.printToolError(evt.ToolResult)
			spinner = t.startSpinner("retrying…")

		case agent.EventError:
			t.printError(evt.Error)

		case agent.EventDone:
		}
	}
}

// ── Spinner ──────────────────────────────────────────────────

type Spinner struct {
	stop chan struct{}
	done chan struct{}
}

func (s *Spinner) Stop() {
	close(s.stop)
	<-s.done
}

func (t *Terminal) startSpinner(text string) *Spinner {
	s := &Spinner{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go t.runSpinner(s, text)
	return s
}

func (t *Terminal) runSpinner(s *Spinner, text string) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	colors := []int{87, 81, 75, 69, 63, 69, 75, 81} // cyan → blue gradient
	i := 0
	for {
		select {
		case <-s.stop:
			fmt.Print("\r\033[K")
			close(s.done)
			return
		default:
			c := colors[i%len(colors)]
			frame := frames[i%len(frames)]
			fmt.Printf("\r\033[K\033[2m%s\033[0m %s", t.color256(c, frame), t.dim(text))
			i++
			time.Sleep(80 * time.Millisecond)
		}
	}
}

// ── Output renderers ─────────────────────────────────────────

func (t *Terminal) printAssistantMessage(msg string) {
	t.withCooked(func() {
		fmt.Printf("  %s\n", t.color256(83, "assistant"))
	})
	t.renderMarkdown(msg)
}

func (t *Terminal) printToolStart(tool *agent.ToolEvent) {
	t.withCooked(func() {
		primary := t.primaryArg(tool)
		if primary != "" {
			primary = " " + t.color256(245, primary)
		}
		line := fmt.Sprintf("  %s %s%s",
			t.color256(87, "⚡"),
			t.bold(tool.Name),
			primary)
		fmt.Println(line)
	})
}

func (t *Terminal) printToolResult(r *agent.ToolResultEvent) {
	t.withCooked(func() {
		if r.Success {
			fmt.Printf("  %s %s  %s\n",
				t.green("✓"), t.bold(r.Name), t.color256(245, r.Summary))
		} else {
			fmt.Printf("  %s %s  %s\n",
				t.red("✗"), t.bold(r.Name), t.color256(245, r.Summary))
		}
	})
}

func (t *Terminal) printToolError(r *agent.ToolResultEvent) {
	t.withCooked(func() {
		fmt.Printf("  %s %s  %s\n",
			t.red("✗"), t.color256(214, r.Name), t.color256(245, r.Summary))
	})
}

func (t *Terminal) printError(err error) {
	t.withCooked(func() {
		fmt.Printf("\n  %s %s %v\n", t.red("✗"), t.bold("error"), err)
	})
}

// primaryArg extracts the most important argument from a tool call
func (t *Terminal) primaryArg(tool *agent.ToolEvent) string {
	switch tool.Name {
	case "read", "write", "edit":
		if fp, ok := tool.Arguments["file_path"]; ok {
			return fmt.Sprintf("%s", fp)
		}
	case "bash":
		if cmd, ok := tool.Arguments["command"]; ok {
			s := fmt.Sprintf("%s", cmd)
			if len(s) > 80 {
				s = s[:80] + "..."
			}
			return s
		}
	case "grep":
		if pat, ok := tool.Arguments["pattern"]; ok {
			return fmt.Sprintf("%s", pat)
		}
	case "glob":
		if pat, ok := tool.Arguments["pattern"]; ok {
			return fmt.Sprintf("%s", pat)
		}
	}
	return ""
}

func (t *Terminal) renderMarkdown(text string) {
	t.withCooked(func() {
		if t.renderer != nil {
			rendered, err := t.renderer.Render(text)
			if err == nil {
				fmt.Print(rendered)
				return
			}
		}
		fmt.Println(text)
	})
}

// ── ANSI / layout helpers ────────────────────────────────────

func (t *Terminal) dim(s string) string {
	return fmt.Sprintf("\033[2m%s\033[0m", s)
}

func (t *Terminal) green(s string) string {
	return fmt.Sprintf("\033[32m%s\033[0m", s)
}

func (t *Terminal) red(s string) string {
	return fmt.Sprintf("\033[31m%s\033[0m", s)
}

func (t *Terminal) cyan(s string) string {
	return fmt.Sprintf("\033[36m%s\033[0m", s)
}

func (t *Terminal) bold(s string) string {
	return fmt.Sprintf("\033[1m%s\033[0m", s)
}

func centerText(s string, width int) string {
	n := len([]rune(stripANSI(s)))
	if n >= width {
		return " " + s + " "
	}
	pad := (width - n) / 2
	return strings.Repeat(" ", pad) + s + strings.Repeat(" ", width-n-pad)
}

func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			for j := i + 1; j < len(s); j++ {
				if s[j] == 'm' || (s[j] >= 'A' && s[j] <= 'z') {
					i = j + 1
					break
				}
			}
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func visibleLen(s string) int {
	return len(stripANSI(s))
}
