# TUI Reference

Launch the full-screen terminal UI:

```bash
cmdctx tui
```

---

## Screens

### 1 — Home (default)

The main entry point. Type your natural language request and press `Enter`.

```
 cmdctx │ Home                                    provider:local │ ~/go/myproject
┌─────────────────────────────────────────────────────────────────────────────┐
│ What do you want to find?                                                   │
│                                                                             │
│ search nginx logs for 500 errors today_                                     │
└─────────────────────────────────────────────────────────────────────────────┘

  Recent:
    12:34  find all php files containing payment failed
    12:31  count timeouts in logs
    12:28  search openresty configs for proxy_pass

  Tip: Press 1-6 to switch screens

Enter Submit │ Tab Focus │ 4 Context │ 5 History │ 6 Settings │ ? Help │ Ctrl+C Quit
```

### 2 — Result

Shows the parsed intent, generated command, explanation, and risk level.

```
  Request:
    search nginx logs for 500 errors today

  Intent:
    search_logs

  Generated Command:
  grep -r -n -i --include=*.log --exclude-dir=.git 500 /var/log/nginx

  Explanation: Search for 500 errors in nginx log files

  Risk: low

  [Enter] Execute  [c] Copy  [Esc] Back
```

### 3 — Execution

Shows stdout, stderr, and metadata from the executed command in scrollable tabs.

```
  [ stdout ]   stderr     metadata

  /var/log/nginx/access.log:1423:192.168.1.1 - - [18/Apr/2026] "GET /api" 500
  /var/log/nginx/access.log:1891:10.0.0.5 - - [18/Apr/2026] "POST /checkout" 500
  ...

← / → Switch tab │ ↑/↓ Scroll │ Esc Back
```

### 4 — Context

Shows the loaded machine context: detected tools, log directories, stacks.

```
  Machine Context
  ────────────────────────────────────────────────────────────

  Hostname: dev-machine
  OS:       linux/amd64
  Home:     ~

  Tools: grep, find, jq, git, awk, sed, ...

  Log dirs:
    /var/log
    /var/log/nginx

  Detected stacks:
    Go (language, high)
    Docker (infra, medium)
```

### 5 — History

Browse previous requests and their results.

```
  #12    2026-04-18 12:34  [─] search nginx logs for 500 errors today
         → grep -r -n -i --include=*.log 500 /var/log/nginx

  #11    2026-04-18 12:31  [✓(0)] count timeout occurrences in logs
         → grep -r -c timeout /var/log

  #10    2026-04-18 12:28  [─] search openresty configs for proxy_pass
         → grep -r -n -i proxy_pass /etc/nginx
```

### 6 — Settings

Shows active configuration. Edit `~/.cmdctx/config.yaml` to change settings.

---

## Keyboard reference

| Key | Action |
|-----|--------|
| `Enter` | Submit request (Home) / Confirm execution (Result) |
| `Esc` | Back to Home |
| `q` | Back to Home (when not in input field) |
| `Tab` | Toggle input focus on Home screen |
| `1` | Go to Home screen |
| `2` | Go to Result screen (if result available) |
| `3` | Go to Execution screen (if executed) |
| `4` | Go to Context screen |
| `5` | Go to History screen |
| `6` | Go to Settings screen |
| `←` / `h` | Previous tab (Execution screen) |
| `→` / `l` | Next tab (Execution screen) |
| `↑` / `↓` | Scroll (History, Context, Execution) |
| `?` | Toggle help |
| `Ctrl+C` | Quit |

---

## Header

The header bar shows:
- App name
- Current screen name
- Active AI provider
- Detected project root

---

## Loading states

While parsing intent or executing a command, a spinner is shown:

```
  ⠴ Parsing intent...
```

---

## Error display

Errors appear in red with a description:

```
  Error: AI completion failed: connection refused (is Ollama running?)
```

Use `Esc` to dismiss and return to the home screen.
