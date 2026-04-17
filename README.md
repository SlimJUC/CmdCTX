# cmdctx

**Local AI-powered terminal assistant for safe command generation.**

`cmdctx` converts natural language into safe, validated shell search/investigation commands. It runs entirely on your machine, never blindly executes AI-generated shell code, and always shows you what it will do before doing it.

```
cmdctx find all php files containing "payment failed" except vendor and node_modules
```
```
  Request:    find all php files containing "payment failed" except vendor and node_modules
  Intent:     search_text
  Command:    grep -r -n -i --include=*.php --exclude-dir=vendor --exclude-dir=node_modules 'payment failed' ./

  Explanation: Search for "payment failed" in PHP files
  Risk:       low

Execute this command? [y/N]
```

---

## What it does

- **Understands natural language** — describe what you want to find
- **Generates safe commands** — structured command builders, never arbitrary shell eval
- **Shows its reasoning** — intent, explanation, assumptions, risk level
- **Asks before executing** — explicit confirmation required
- **Works offline** — local AI (Ollama) or remote (OpenAI, Anthropic)
- **Learns your project** — scans context to improve command accuracy

---

## Installation

### Quick install

```bash
git clone https://github.com/slim/cmdctx
cd cmdctx
make install
```

Installs to `~/.local/bin/cmdctx`. If that directory isn't in your PATH:
```bash
export PATH="$HOME/.local/bin:$PATH"  # add to ~/.bashrc or ~/.zshrc
```

### Global install (requires sudo)

```bash
make install-global
```

### Manual build

```bash
make build
# binary at ./bin/cmdctx
```

---

## First run

```bash
cmdctx init          # scan machine + project context
cmdctx doctor        # verify setup
cmdctx providers     # configure AI provider
```

---

## Configuring AI providers

### Ollama (local, recommended)

```bash
# Install Ollama: https://ollama.ai
ollama pull llama3.2

cmdctx providers add --name local --type ollama --model llama3.2
```

### OpenAI

```bash
cmdctx providers add \
  --name openai \
  --type openai \
  --model gpt-4o-mini \
  --key sk-your-key-here
```

### Anthropic Claude

```bash
cmdctx providers add \
  --name claude \
  --type anthropic \
  --model claude-3-5-haiku-20241022 \
  --key sk-ant-your-key-here
```

Switch between providers:
```bash
cmdctx providers use local
```

---

## Usage examples

### Shorthand mode (recommended)

Just describe what you want:

```bash
cmdctx find all php files containing "payment failed" except vendor and node_modules
cmdctx search nginx logs for 500 errors today
cmdctx look for redis timeout references in this project
cmdctx find docker compose files on this machine
cmdctx count occurrences of authToken in js and ts files
cmdctx search openresty configs for proxy_pass
```

### Explicit subcommands

```bash
# Preview only (no execution prompt)
cmdctx ask "find all php files with payment errors"

# Always prompt for execution
cmdctx run "search nginx logs for 500 errors"
```

### Flags

```bash
--no-exec     Preview command only, never ask to execute
--yes         Auto-confirm for safe read-only commands
--json        Machine-readable JSON output
```

---

## All commands

| Command | Description |
|---------|-------------|
| `cmdctx <natural language>` | Shorthand NL mode (default) |
| `cmdctx init` | Generate machine + project context |
| `cmdctx ask "<query>"` | Preview command, never execute |
| `cmdctx run "<query>"` | Generate + offer execution |
| `cmdctx tui` | Launch interactive full-screen TUI |
| `cmdctx refresh` | Regenerate context files |
| `cmdctx doctor` | Check setup |
| `cmdctx history` | Browse command history |
| `cmdctx config show` | Show configuration |
| `cmdctx providers list` | List AI providers |
| `cmdctx providers add` | Add/update a provider |

---

## Safety model

`cmdctx` is **read-only by default**. The following are always blocked:

- File deletion: `rm`, `rmdir`, `shred`
- File mutation: `mv`, `cp`, `chmod`, `chown`
- Privilege escalation: `sudo`, `su`
- Destructive writes: `dd`, `mkfs`, `truncate`
- Shell execution: `bash`, `sh`, `eval`, `exec`
- Shell redirections: `>`, `>>`, `| sh`, `$(...)`, `` ` ``
- Network mutation: `curl | sh`, `wget | sh`
- Service management: `systemctl`, `service`
- Package management: `apt`, `npm`, `pip`, `yarn`

**Risk levels:**
- `low` — scoped project search with exclusions
- `medium` — broad recursive scan without exclusions  
- `high` — system path access (`/etc`, `/var`, `/home`)
- `blocked` — policy violation, will not generate

The `--yes` flag only works for `low` and `medium` risk commands.

---

## Context files

After `cmdctx init`, these files are created:

```
~/.cmdctx/
├── config.yaml          # Configuration
├── command-policy.json  # Safety policy
├── machine-context.md   # Human-readable machine summary
├── machine-context.json # Machine-readable machine summary
└── history.db           # SQLite history

<project>/.cmdctx/
├── project-context.md   # Project summary
└── project-context.json # Project summary (JSON)
```

---

## TUI

Launch the full-screen interactive interface:

```bash
cmdctx tui
```

Keyboard shortcuts:
- `Enter` — submit request / confirm execution
- `Esc` / `q` — back / quit
- `Tab` — toggle focus
- `1-6` — switch screens
- `←/→` — switch tabs in execution view
- `?` — help

Screens: Home · Result · Execution · Context · History · Settings

---

## Configuration

Config file: `~/.cmdctx/config.yaml`

```yaml
active_provider: local
default_scan_mode: safe
execution_timeout: 30s
output_max_bytes: 524288
history_retention: 90
ai_permission_mode: local_only
providers:
  - name: local
    type: ollama
    base_url: http://localhost:11434
    model: llama3.2
```

Environment variable overrides:
```bash
CMDCTX_ACTIVE_PROVIDER=openai
CMDCTX_LOG_LEVEL=debug
```

---

## Uninstall

```bash
make uninstall          # remove binary only
make uninstall-all      # remove binary + all app data
```

---

## Development

```bash
make build              # build binary
make test               # run all tests
make test-short         # run tests (no integration)
make cover              # run tests with coverage report
make vet                # run go vet
make lint               # run golangci-lint (if installed)
make doctor             # build + run cmdctx doctor
```

---

## License

MIT
