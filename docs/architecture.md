# Architecture

## Design principles

1. **Safety-first** — blocked commands are enforced in code, not described in prompts
2. **Local-first** — works without any AI provider; AI enhances but doesn't gate
3. **Explainable** — every generated command shows intent, explanation, risk, assumptions
4. **Structured** — AI returns JSON intent; Go code builds the final command
5. **Auditable** — full history of requests, parsed intents, commands, and executions in SQLite

## Data flow

```
User natural language request
         │
         ▼
  Context retrieval (SQLite tag-based, no vectors)
         │
         ▼
  AI intent parsing (structured JSON output)
  OR rule-based fallback (no AI required)
         │
         ▼
  Intent validation (schema + safety checks)
         │
         ▼
  Command builder (typed argv construction)
         │
         ▼
  Policy validation (hardcoded safety rules)
         │
         ▼
  Display: intent + command + explanation + risk
         │
         ▼
  User confirmation (explicit, always)
         │
         ▼
  Safe execution (exec.CommandContext, never sh -c)
         │
         ▼
  History storage (redacted SQLite record)
```

## Package structure

```
cmd/cmdctx/           Binary entry point
internal/
  config/             Config types, Viper loading, defaults
  policy/             Safety policy enforcement (hardcoded + user-extensible)
  contextscan/        Local file scanner (safe mode + deep mode)
                      Framework detection (Go, PHP, Node, Docker, etc.)
  contextgen/         Machine + project context file generation (MD + JSON)
  ai/                 Provider interface + OpenAI/Ollama/Anthropic implementations
  intent/             Intent types, JSON schema, AI prompt, rule-based parser
  commands/           Structured command builders (text search, file search, logs, JSON)
  history/            SQLite history store with redaction
  retrieval/          Tag-based context chunk retrieval from SQLite
  runner/             Secure exec.CommandContext wrapper with timeout + truncation
  install/            Binary install/uninstall helpers, PATH detection
  utils/              Shared helpers: redaction, tool detection, path helpers
  cli/                Cobra command definitions (NL shorthand mode + all subcommands)
  tui/                Bubble Tea full-screen TUI
    theme/            Lip Gloss style definitions
```

## AI usage model

AI is used **only** for intent parsing. The flow is:

1. Build a system prompt explaining the strict JSON schema
2. Build a user prompt with the request + relevant context snippets
3. Send to AI provider (temperature=0.1 for determinism)
4. Extract JSON from response (handles markdown code fences and prose wrapping)
5. Validate against schema (required fields, safe values, no path traversal)
6. Apply defaults (target paths, tool preference)

If AI parsing fails (no provider configured, network error, malformed response):
- Fall back to rule-based parser in `internal/intent/parser.go`
- Rule-based parser handles the most common patterns deterministically

AI **never** generates raw shell commands. The Go command builders do that from validated intent fields.

## Command builders

Each builder is a pure function: `Intent → []string (argv)`.

```
buildTextSearch  → rg (preferred) or grep (fallback)
buildFileSearch  → fd (preferred) or find (fallback)
buildLogSearch   → rg/grep with log-specific path resolution
buildJSONSearch  → rg targeting .json files (jq pipeline not used due to pipe policy)
```

Builders always:
- Use typed argv (never string concatenation)
- Apply all exclude paths as proper flags
- Never use shell features (pipes, redirections, substitution)

## Context retrieval

Context files are chunked and tagged in SQLite:

```
context_chunks (source, section, content, tags)
  machine-context / tools       → ["tools", "search", "machine"]
  machine-context / log_dirs    → ["logs", "nginx", "apache"]
  machine-context / stacks      → ["go", "framework", "stack"]
  project-context / frameworks  → ["php", "backend", "project"]
  ...
```

On each request:
1. Extract tags from request words (keyword → tag mapping)
2. Query SQLite for matching chunks
3. Score by keyword overlap + tag match
4. Return top 5 snippets (max 200 chars each)
5. Include in AI prompt

No vector DB, no embeddings — practical lexical matching is sufficient for this use case.

## TUI architecture

Built with Bubble Tea (elm-architecture):
- Single `Model` struct holds all state
- `Update(msg)` returns new model + commands for async work
- `View()` renders pure from model state
- Async operations (AI parsing, execution) return messages via channels
- All screens rendered in a single `View()` call based on `model.screen`

## History schema

```sql
CREATE TABLE history (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  prompt         TEXT,      -- redacted natural language request
  intent_type    TEXT,      -- search_text | find_files | etc.
  intent_json    TEXT,      -- redacted parsed intent JSON
  rendered_cmd   TEXT,      -- redacted final command display string
  parsed_by      TEXT,      -- "ai" or "rule_based"
  risk           TEXT,      -- low | medium | high | blocked
  cwd            TEXT,      -- working directory at time of request
  executed       INTEGER,   -- 0 or 1
  exit_code      INTEGER,   -- null if not executed
  duration_ms    INTEGER,   -- null if not executed
  stdout_snippet TEXT,      -- redacted, truncated
  stderr_snippet TEXT,      -- redacted, truncated
  created_at     TEXT       -- RFC3339 timestamp
);

CREATE TABLE context_chunks (
  source     TEXT,          -- "machine-context" | "project-context:..."
  section    TEXT,          -- "tools" | "log_dirs" | etc.
  content    TEXT,          -- the snippet text
  tags       TEXT,          -- JSON array of tags
  updated_at TEXT,          -- RFC3339 timestamp
  UNIQUE(source, section)
);
```
