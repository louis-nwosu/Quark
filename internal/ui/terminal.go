package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/louis-nwosu/Quark/internal/agent"
	"github.com/louis-nwosu/Quark/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	// opencode dark theme colors
	colorBG          = "#0a0a0a"
	colorPanel       = "#141414"
	colorElement     = "#1e1e1e"
	colorPrimary     = "#fab283"
	colorSecondary   = "#5c9cf5"
	colorAccent      = "#9d7cd8"
	colorError       = "#e06c75"
	colorWarning     = "#f5a742"
	colorSuccess     = "#7fd88f"
	colorInfo        = "#56b6c2"
	colorText        = "#eeeeee"
	colorTextMuted   = "#808080"
	colorBorder      = "#484848"
	colorAgent       = "#fab283" // warm orange for agent accent
	colorShell       = "#7fd88f" // green for shell mode
)

type Terminal struct {
	app      *tview.Application
	chatView *tview.TextView
	input    *tview.InputField
	inputRow *tview.Flex
	metaBar  *tview.TextView
	footer   *tview.TextView
	flex     *tview.Flex
	pages    *tview.Pages

	ag         *agent.Agent
	cfg        *config.Config
	ctx        context.Context
	cancel     context.CancelFunc
	processing bool
	mu         sync.Mutex

	initialMsg   string
	welcomeShown bool
}

func NewTerminal(ag *agent.Agent, cfg *config.Config) *Terminal {
	t := &Terminal{
		ag:  ag,
		cfg: cfg,
		app: tview.NewApplication(),
	}
	t.buildUI()
	return t
}

func (t *Terminal) buildUI() {
	t.chatView = tview.NewTextView()
	t.chatView.SetDynamicColors(true)
	t.chatView.SetScrollable(true)
	t.chatView.SetWordWrap(true)
	t.chatView.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))

	if t.ag.HasSession() {
		fmt.Fprintf(t.chatView, "[#%s]↻ resumed — %d messages[-]\n\n", colorTextMuted, t.ag.MessageCount())
	} else {
		t.showBanner()
	}

	t.input = tview.NewInputField()
	t.input.SetLabel(fmt.Sprintf("[#%s]╻[-] [#%s]", colorAgent, colorTextMuted))
	t.input.SetFieldWidth(0)
	t.input.SetPlaceholder("ask anything...")
	t.input.SetPlaceholderTextColor(tcell.NewHexColor(0x555555))
	t.input.SetFieldBackgroundColor(tcell.NewHexColor(0x141414))
	t.input.SetFieldTextColor(tcell.NewHexColor(0xeeeeee))
	t.input.SetLabelColor(tcell.NewHexColor(0xeeeeee))
	t.input.SetBackgroundColor(tcell.NewHexColor(0x141414))
	t.input.SetDoneFunc(t.onInputDone)

	t.metaBar = tview.NewTextView()
	t.metaBar.SetDynamicColors(true)
	t.metaBar.SetBackgroundColor(tcell.NewHexColor(0x141414))
	t.metaBar.SetTextAlign(tview.AlignLeft)

	t.updateMetaBar()

	t.inputRow = tview.NewFlex().SetDirection(tview.FlexRow)
	t.inputRow.SetBackgroundColor(tcell.NewHexColor(0x141414))
	inputFieldRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	inputFieldRow.SetBackgroundColor(tcell.NewHexColor(0x141414))
	inputFieldRow.AddItem(t.input, 0, 1, true)
	t.inputRow.AddItem(inputFieldRow, 1, 0, true)
	t.inputRow.AddItem(t.metaBar, 1, 0, false)

	t.footer = tview.NewTextView()
	t.footer.SetDynamicColors(true)
	t.footer.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	t.footer.SetTextAlign(tview.AlignLeft)
	t.updateFooter()

	t.flex = tview.NewFlex().SetDirection(tview.FlexRow)
	t.flex.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	t.flex.AddItem(t.chatView, 0, 1, false)
	t.flex.AddItem(t.inputRow, 3, 0, true)
	t.flex.AddItem(t.footer, 1, 0, false)

	t.pages = tview.NewPages()
	t.pages.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	t.pages.AddPage("main", t.flex, true, true)

	t.app.SetRoot(t.pages, true)
	t.app.SetFocus(t.input)

	t.app.SetInputCapture(func(evt *tcell.EventKey) *tcell.EventKey {
		if evt.Key() == tcell.KeyCtrlC {
			if pg, _ := t.pages.GetFrontPage(); pg != "main" {
				t.pages.SwitchToPage("main")
				t.app.SetFocus(t.input)
				return nil
			}
			t.Stop()
			return nil
		}
		return evt
	})
}

func (t *Terminal) Run() error {
	if t.initialMsg != "" {
		msg := t.initialMsg
		t.initialMsg = ""
		t.app.QueueUpdateDraw(func() {
			t.input.SetText(msg)
			t.onInputDone(tcell.KeyEnter)
		})
	}
	return t.app.Run()
}

func (t *Terminal) Stop() {
	t.app.Stop()
}

func (t *Terminal) SendMessage(msg string) {
	t.initialMsg = msg
}

// ── UI Building ──

func (t *Terminal) showBanner() {
	w := filepath.Base(t.cwd())
	fmt.Fprintf(t.chatView, `
[#%s]  %s
[#%s]
[#%s]  type a message or /help to get started
[-]
`, colorPrimary, w, colorTextMuted, colorTextMuted)
	t.welcomeShown = true
}

func (t *Terminal) cwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func (t *Terminal) updateMetaBar() {
	pc := t.cfg.Providers[t.cfg.DefaultProvider]
	model := pc.Model
	if model == "" {
		model = t.cfg.DefaultProvider
	}
	statusColor := colorSuccess
	if !t.cfg.HasUserAPIKey() {
		statusColor = colorWarning
	}
	t.metaBar.SetText(fmt.Sprintf(
		"  [#%s]%s[-]  [#%s]·[-]  [#%s]%s[-]",
		colorAgent, "agent",
		colorTextMuted,
		statusColor, model,
	))
}

func (t *Terminal) updateFooter() {
	wd := t.cwd()
	t.footer.SetText(fmt.Sprintf(
		"  [#%s]%s[-]  [#%s]•[-] [#%s]●[-] [#%s]LSP[-]  [#%s]⊙[-] [#%s]MCP[-]  [#%s]/status[-]",
		colorTextMuted, tview.Escape(wd),
		colorTextMuted,
		colorSuccess, colorTextMuted,
		colorSuccess, colorTextMuted,
		colorTextMuted,
	))
}

// ── Input Handling ──

func (t *Terminal) onInputDone(key tcell.Key) {
	if key != tcell.KeyEnter {
		return
	}
	t.mu.Lock()
	if t.processing {
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()

	text := strings.TrimSpace(t.input.GetText())
	if text == "" {
		return
	}
	t.input.SetText("")

	if strings.HasPrefix(text, "/") {
		t.handleSlashCommand(text)
		return
	}

	// user message with left-border accent
	fmt.Fprintf(t.chatView, "[#%s]╻[-] [#%s::b]you[-]\n", colorAgent, colorAgent)
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(t.chatView, "[#%s]╹[-] [#%s]%s[-]\n", colorAgent, colorText, tview.Escape(line))
	}
	fmt.Fprintf(t.chatView, "\n")
	t.chatView.ScrollToEnd()

	t.mu.Lock()
	t.processing = true
	t.mu.Unlock()
	t.input.SetDisabled(true)
	t.updateInputPrompt("processing...")

	go t.runAgent(text)
}

func (t *Terminal) updateInputPrompt(text string) {
	t.input.SetLabel(fmt.Sprintf("[#%s]╻[-] [#%s]%s[-] ", colorAgent, colorTextMuted, text))
}

func (t *Terminal) resetInputPrompt() {
	t.input.SetLabel(fmt.Sprintf("[#%s]╻[-] [#%s]", colorAgent, colorTextMuted))
}

func (t *Terminal) handleSlashCommand(cmd string) {
	switch cmd {
	case "/clear":
		t.ag.Clear()
		t.chatView.Clear()
		fmt.Fprintf(t.chatView, "[#%s]  conversation cleared[-]\n\n", colorTextMuted)
		t.showBanner()

	case "/config":
		t.showConfigPage()

	case "/exit", "/quit":
		t.Stop()

	case "/help", "/?":
		t.showHelp()

	default:
		fmt.Fprintf(t.chatView, "[#%s]╻[-] [#%s]unknown: %s[-]\n", colorError, colorError, tview.Escape(cmd))
		fmt.Fprintf(t.chatView, "[#%s]╹[-] [#%s]type /help for commands[-]\n", colorError, colorTextMuted)
		fmt.Fprint(t.chatView, "\n")
	}
	t.chatView.ScrollToEnd()
}

func (t *Terminal) showHelp() {
	fmt.Fprint(t.chatView, "[#ffffff::b]Commands[-]\n\n")
	for _, h := range []struct{ cmd, desc string }{
		{"/clear", "Clear conversation"},
		{"/config", "Configure provider"},
		{"/exit", "Exit quark"},
		{"/help", "Show this"},
	} {
		fmt.Fprintf(t.chatView, "  [#%s]%-8s[-]  [#%s]%s[-]\n", colorPrimary, h.cmd, colorText, h.desc)
	}
	fmt.Fprint(t.chatView, "\n")
}

// ── Agent Events ──

func (t *Terminal) runAgent(msg string) {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	defer func() {
		t.app.QueueUpdateDraw(func() {
			t.mu.Lock()
			t.processing = false
			t.mu.Unlock()
			t.input.SetDisabled(false)
			t.resetInputPrompt()
			t.app.SetFocus(t.input)
		})
	}()

	events := make(chan agent.Event, 20)
	go t.ag.Run(ctx, msg, events)

	for evt := range events {
		e := evt
		t.app.QueueUpdateDraw(func() {
			t.handleEvent(e)
		})
	}

	t.app.QueueUpdateDraw(func() {
		t.chatView.ScrollToEnd()
	})
}

var (
	hadThinking bool
	hadStream   bool
)

func (t *Terminal) handleEvent(evt agent.Event) {
	switch evt.Type {
	case agent.EventReasoning:
		if !hadThinking {
			fmt.Fprintf(t.chatView, "[#%s]◌ thinking[-] [#%s::i]%s[-]\n", colorTextMuted, colorTextMuted, tview.Escape(evt.Text))
			hadThinking = true
		} else {
			text := strings.ReplaceAll(evt.Text, "\n", " ")
			fmt.Fprintf(t.chatView, "[#%s]%s[-]", colorTextMuted, tview.Escape(text))
		}

	case agent.EventTextChunk:
		if !hadStream {
			fmt.Fprintf(t.chatView, "[#%s]╻[-] [#%s::b]assistant[-]\n", colorSecondary, colorSecondary)
			hadStream = true
		}
		fmt.Fprint(t.chatView, tview.Escape(evt.Text))

	case agent.EventText:
		if !hadStream {
			fmt.Fprintf(t.chatView, "[#%s]╻[-] [#%s::b]assistant[-]\n", colorSecondary, colorSecondary)
		}
		fmt.Fprintln(t.chatView, tview.Escape(evt.Text))
		fmt.Fprint(t.chatView, "\n")

	case agent.EventToolStart:
		hadThinking = false
		if hadStream {
			hadStream = false
			fmt.Fprint(t.chatView, "\n")
		}
		primary := t.primaryArg(evt.Tool)
		if primary != "" {
			primary = " " + primary
			if len(primary) > 50 {
				primary = primary[:50] + "…"
			}
		}
		fmt.Fprintf(t.chatView, "[#%s]⚡ %s%s[-]\n", colorInfo, evt.Tool.Name, tview.Escape(primary))

	case agent.EventToolResult:
		r := evt.ToolResult
		if r.Success {
			fmt.Fprintf(t.chatView, "[#%s]✓ %s  [#%s]%s[-]\n", colorSuccess, r.Name, colorTextMuted, tview.Escape(r.Summary))
		} else {
			fmt.Fprintf(t.chatView, "[#%s]✗ %s  [#%s]%s[-]\n", colorError, r.Name, colorTextMuted, tview.Escape(r.Summary))
		}

	case agent.EventToolError:
		r := evt.ToolResult
		fmt.Fprintf(t.chatView, "[#%s]✗ %s  [#%s]%s[-]\n", colorWarning, r.Name, colorTextMuted, tview.Escape(r.Summary))

	case agent.EventError:
		msg := evt.Error.Error()
		if strings.Contains(msg, "401") || strings.Contains(msg, "Authentication") || strings.Contains(msg, "auth") {
			fmt.Fprintf(t.chatView, "[#%s]✗ not configured  [#%s]run /config to set up an API key[-]\n", colorWarning, colorTextMuted)
		} else {
			fmt.Fprintf(t.chatView, "[#%s]✗ error  [#%s]%s[-]\n", colorError, colorTextMuted, tview.Escape(msg))
		}
		fmt.Fprint(t.chatView, "\n")

	case agent.EventDone:
		hadThinking = false
		hadStream = false
	}

	t.chatView.ScrollToEnd()
}

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
	case "grep", "glob":
		if pat, ok := tool.Arguments["pattern"]; ok {
			return fmt.Sprintf("%s", pat)
		}
	}
	return ""
}

// ── Config Page ──

func (t *Terminal) showConfigPage() {
	configFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	configFlex.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))

	header := tview.NewTextView()
	header.SetDynamicColors(true)
	header.SetTextAlign(tview.AlignCenter)
	header.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	fmt.Fprint(header, "\n[#fab283::b]Provider Configuration[-]\n\n")

	list := tview.NewList()
	list.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	list.SetMainTextColor(tcell.NewHexColor(0xeeeeee))
	list.SetSecondaryTextColor(tcell.NewHexColor(0x808080))
	list.SetShortcutColor(tcell.NewHexColor(0xfab283))
	list.SetHighlightFullLine(true)

	names := t.cfg.ProviderNames()
	for i, name := range names {
		n := name
		status := t.cfg.ProviderStatus(n)
		active := ""
		if n == t.cfg.DefaultProvider {
			active = " [default]"
		}
		list.AddItem(n, status+active, rune('1'+i), func() {
			t.showProviderForm(n)
		})
	}

	cancelBtn := tview.NewButton("Cancel")
	cancelBtn.SetStyle(tcell.StyleDefault.
		Foreground(tcell.NewHexColor(0xeeeeee)).
		Background(tcell.NewHexColor(0x333333)))
	cancelBtn.SetSelectedFunc(func() {
		t.pages.SwitchToPage("main")
		t.app.SetFocus(t.input)
	})

	btnFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	btnFlex.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	btnFlex.AddItem(nil, 0, 1, false)
	btnFlex.AddItem(cancelBtn, 12, 0, true)
	btnFlex.AddItem(nil, 0, 1, false)

	configFlex.AddItem(header, 3, 0, false)
	configFlex.AddItem(list, 0, 1, true)
	configFlex.AddItem(btnFlex, 1, 0, false)

	configFlex.SetInputCapture(func(evt *tcell.EventKey) *tcell.EventKey {
		if evt.Key() == tcell.KeyEsc {
			t.pages.SwitchToPage("main")
			t.app.SetFocus(t.input)
			return nil
		}
		return evt
	})

	t.pages.AddPage("config", configFlex, true, true)
	t.pages.SwitchToPage("config")
	t.app.SetFocus(list)
}

func (t *Terminal) showProviderForm(name string) {
	form := tview.NewForm()
	form.SetBorder(false)
	form.SetLabelColor(tcell.NewHexColor(0xfab283))
	form.SetFieldBackgroundColor(tcell.NewHexColor(0x1e1e1e))
	form.SetFieldTextColor(tcell.NewHexColor(0xeeeeee))
	form.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	form.SetButtonBackgroundColor(tcell.NewHexColor(0xfab283))

	form.AddTextView("Provider", name, 20, 1, false, false)

	pc := t.cfg.Providers[name]
	apiKey := pc.APIKey
	model := pc.Model

	form.AddPasswordField("API Key", apiKey, 50, '*', func(text string) {
		apiKey = text
	})
	form.AddInputField("Model", model, 40, nil, func(text string) {
		model = text
	})

	isDefault := name == t.cfg.DefaultProvider
	form.AddCheckbox("Set as default", isDefault, func(checked bool) {
		isDefault = checked
	})

	form.AddButton("Save", func() {
		pc.APIKey = apiKey
		pc.Model = model
		t.cfg.Providers[name] = pc
		if isDefault {
			t.cfg.DefaultProvider = name
		}
		t.cfg.Save()
		t.updateMetaBar()
		t.chatView.Clear()
		t.showBanner()
		t.updateMetaBar()
		t.pages.SwitchToPage("main")
		t.app.SetFocus(t.input)
	})

	form.AddButton("Cancel", func() {
		t.pages.SwitchToPage("main")
		t.app.SetFocus(t.input)
	})

	form.SetInputCapture(func(evt *tcell.EventKey) *tcell.EventKey {
		if evt.Key() == tcell.KeyEsc {
			t.pages.SwitchToPage("main")
			t.app.SetFocus(t.input)
			return nil
		}
		return evt
	})

	formFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	formFlex.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	formFlex.AddItem(nil, 0, 1, false)
	formFlex.AddItem(form, 10, 0, true)
	formFlex.AddItem(nil, 0, 1, false)

	outerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	outerFlex.SetBackgroundColor(tcell.NewHexColor(0x0a0a0a))
	outerFlex.AddItem(nil, 0, 1, false)
	outerFlex.AddItem(formFlex, 50, 0, true)
	outerFlex.AddItem(nil, 0, 1, false)

	t.pages.AddPage("form", outerFlex, true, true)
	t.pages.SwitchToPage("form")
	t.app.SetFocus(form)
}
