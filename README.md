# Quark

**quark** is a lightweight terminal coding agent — think Claude Code or opencode, but with a focus on simplicity and a polished [tview](https://github.com/rivo/tview)-based TUI. It pairs an LLM (OpenAI, Anthropic, or OpenRouter) with file-system tools to autonomously read, write, edit, search, and execute code on your behalf.

## Quick Start

```bash
# Download the binary
go install github.com/louis-nwosu/Quark@latest

# Or build from source
git clone https://github.com/louis-nwosu/Quark.git
cd Quark
go build -o quark .
./quark
```

Quark supports **OpenAI**, **Anthropic Claude**, and **OpenRouter**. On first run, you'll be prompted to add an API key. You can also set `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, or `OPENROUTER_API_KEY` environment variables.

## Usage

```bash
quark                  # Interactive TUI mode (recommended)
quark "add error handling to main.go"   # One-shot mode
quark -m "refactor the auth module"     # Interactive with initial prompt
```

### Flags

| Flag | Description |
|------|-------------|
| `-m <prompt>` | Start interactive mode with an initial message |
| `-model <id>` | Override the provider or model |
| `-help` | Show help |

### Slash Commands (interactive mode)

| Command | Description |
|---------|-------------|
| `/clear` | Clear conversation history |
| `/config` | Open interactive config UI (add API keys, change provider) |
| `/exit` or `/quit` | Exit quark |
| `/help` or `/?` | Show slash commands |

## Features

- **Multi-provider LLM support** — OpenAI, Anthropic Claude, and OpenRouter (with free tier)
- **Streaming responses** — token-by-token text rendering with real-time display
- **Tool-use agent loop** — think-act-observe cycle up to 25 rounds
- **File operations** — read, write, edit with string replacement
- **Code search** — regex (`grep`) and glob pattern matching
- **Shell execution** — run commands with configurable timeout
- **Git integration** — status, diff, and commit tools
- **Session persistence** — conversations survive restarts, auto-resume with history notice
- **Context window compaction** — drops old conversational turns when the history grows too long
- **Polished tview-based TUI** — inspired by opencode's dark theme, with left-border accent messages, metadata bar, and footer status
- **Config discovery** — `.quark.json` in current dir, `~/.config/quark/quark.json`, env var overrides

## Configuration

Config is loaded from the first existing source:

1. `.quark.json` — project-local config (not committed — in `.gitignore`)
2. `~/.config/quark/quark.json` — user-global config
3. Environment variable overrides: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`

### Default providers

Quark ships with three providers pre-configured. OpenRouter is the default when no user API key is found:

| Provider | Default Model | Base URL |
|----------|--------------|----------|
| OpenRouter | `openrouter/free` (auto-routes) | `https://openrouter.ai/api/v1` |
| OpenAI | `gpt-4o` | `https://api.openai.com/v1` |
| Anthropic | `claude-sonnet-4-20250514` | `https://api.anthropic.com` |

### Example config

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-...",
      "model": "gpt-4o",
      "base_url": "https://api.openai.com/v1"
    }
  },
  "default_provider": "openai",
  "max_tool_rounds": 25,
  "compact_threshold": 100,
  "bash_timeout": 30
}
```

## Architecture

```
main.go → cmd/root.go
              │
              ├── internal/config/     Config loading & discovery
              ├── internal/llm/        Provider abstraction (OpenAI, Anthropic)
              │   ├── provider.go      Provider interface, StreamEvent types
              │   ├── openai.go        OpenAI/OpenRouter chat + streaming
              │   └── anthropic.go     Anthropic Claude chat + streaming
              ├── internal/agent/      Agent loop & session management
              │   ├── loop.go          Think-act-observe event loop, compact
              │   └── session.go       Message history, persistence
              ├── internal/tools/      Tool registry & implementations
              │   ├── registry.go      Register, dispatch, schema generation
              │   ├── read.go, write.go, edit.go
              │   ├── bash.go, grep.go, glob.go
              │   └── git.go
              └── internal/ui/         Terminal UI
                  └── terminal.go      tview-based TUI, event handling, config UI
```

### Agent Loop

```
User message
    ↓
LLM responds (streaming text or tool call)
    ↓
If tool call → execute tool → add result to session → loop
If text     → render response → done
```

### Tool System

Tools are registered in a central registry, each implementing a simple function signature:

```go
func(ctx context.Context, args map[string]any) *ToolResult
```

The registry handles argument normalization, JSON schema generation for LLM function-calling, and dispatch by name.

## Development

```bash
go build -o quark .          # Build
go vet ./...                 # Lint
go build -o quark . && go vet ./...   # Build + lint
./quark                      # Run
```

## Progress & Current Limitations

### What's implemented

- **tview-based TUI** replacing the old glamour/liner stack — full terminal event loop, dynamic colors, scrollable chat
- **Input area** with left-border accent, placeholder, and metadata bar showing model/provider
- **Footer bar** with working directory and status indicators (LSP, MCP placeholders)
- **Config UI** overlay — provider list, form-based editing of API keys and models
- **Streaming response display** — tool calls, results, reasoning, errors rendered with opencode-inspired styling
- **Slash commands** — `/clear`, `/config`, `/exit`, `/help`
- **Session persistence** — conversation history across restarts
- **Context compaction** — token-aware eviction and LLM-based summarization
- **Multi-provider** — OpenAI, Anthropic, OpenRouter with streaming SSE

### Known limitations

- **Markdown rendering** — assistant responses are displayed as plain text (no markdown formatting yet)
- **Multi-line input** — single-line InputField only; no multi-line text area or external editor integration
- **No markdown rendering** — code blocks, lists, and headings appear as raw markdown syntax
- **No agent coloring** — currently uses a single agent accent color; per-agent color assignment not implemented
- **File references** — no `@` file autocomplete or file attachment badges
- **No LSP integration** — LSP status indicator is a placeholder in the footer
- **No MCP support** — MCP status indicator is a placeholder in the footer
- **No undo/redo** — no `/undo` or `/redo` commands for reverting changes
- **No session switching** — single session; no multi-session management
- **No model switching** — no model picker dialog or runtime model switching
- **Compact/summarize** — `/compact` slash command not exposed; compaction is auto-only
- **No share links** — no `/share` or `/export` commands
- **Config UI** — basic form-based editing; no fancy provider picker with model selection

## Contributing

Contributions are welcome! Here's how you can help:

- **Pick a limitation above** and submit a PR to fix it
- **Improve the TUI** — better markdown rendering, multi-line input, agent colors
- **Add provider integrations** — Google Gemini, Groq, Azure OpenAI, AWS Bedrock, local models
- **Add LSP integration** — wire up language servers for code intelligence
- **Add MCP support** — implement the Model Context Protocol for extensible tooling
- **Improve session management** — multiple sessions, session switching, undo/redo
- **Write tests** — the codebase needs test coverage across all packages
- **Documentation** — improve the README, add examples, write contributor guides

To contribute:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Why Quark?

- **Single binary** — no Python runtime, no npm install, no Docker
- **Zero config startup** — works immediately with OpenRouter free tier
- **Blazing fast** — Go concurrency, streaming SSE, minimal overhead
- **Privacy-first** — your API keys stay in local config files
- **Simple codebase** — easy to hack on and extend

## License

MIT
