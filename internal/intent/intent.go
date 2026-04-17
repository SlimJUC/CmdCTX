// Package intent defines the structured AI intent format and its validation.
// AI is asked to return strict JSON matching this schema. The schema is then
// validated before any command building occurs. This prevents freeform shell
// injection and makes the AI's reasoning transparent and auditable.
package intent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// IntentType classifies the category of operation the user wants to perform.
type IntentType string

const (
	// IntentSearchText performs a text pattern search inside files.
	IntentSearchText IntentType = "search_text"
	// IntentFindFiles locates files by name, extension, or pattern.
	IntentFindFiles IntentType = "find_files"
	// IntentSearchLogs inspects log files for patterns or time-scoped events.
	IntentSearchLogs IntentType = "search_logs"
	// IntentCountOccurrences counts pattern matches (no content dump).
	IntentCountOccurrences IntentType = "count_occurrences"
	// IntentSearchJSON searches inside JSON files using jq-style logic.
	IntentSearchJSON IntentType = "search_json"
	// IntentUnknown is returned when the intent cannot be determined.
	IntentUnknown IntentType = "unknown"
)

// Intent is the strict structured output expected from the AI.
// Every field maps directly to command-builder parameters.
// No free-form shell is accepted here.
type Intent struct {
	// Intent is the primary operation category.
	Intent IntentType `json:"intent"`

	// Pattern is the search string or regex (for text search).
	Pattern string `json:"pattern,omitempty"`

	// TargetPaths is where to search. Defaults to ["./"] if empty.
	TargetPaths []string `json:"target_paths,omitempty"`

	// FileGlobs are file pattern filters, e.g. ["*.php", "*.go"].
	FileGlobs []string `json:"file_globs,omitempty"`

	// ExcludePaths are directories or patterns to skip.
	ExcludePaths []string `json:"exclude_paths,omitempty"`

	// TimeScope is an optional time window for log searches (e.g. "today", "1h", "24h").
	TimeScope string `json:"time_scope,omitempty"`

	// ToolPreference is the preferred CLI tool ("rg", "grep", "fd", "find").
	ToolPreference string `json:"tool_preference,omitempty"`

	// CountOnly when true emits only match counts, not content.
	CountOnly bool `json:"count_only"`

	// ShowLineNumbers enables line number output.
	ShowLineNumbers bool `json:"show_line_numbers"`

	// ContextLines is the number of surrounding lines to include in matches.
	ContextLines int `json:"context_lines"`

	// CaseSensitive overrides the default case-insensitive behaviour.
	CaseSensitive bool `json:"case_sensitive"`

	// MaxResults caps the number of output lines (0 = unlimited).
	MaxResults int `json:"max_results,omitempty"`

	// Explanation is the AI's natural language description of what it will do.
	Explanation string `json:"explanation,omitempty"`

	// Assumptions lists anything the AI assumed to resolve ambiguity.
	Assumptions []string `json:"assumptions,omitempty"`
}

// ParseFromJSON parses an Intent from raw JSON bytes.
// It is strict: unknown fields are silently ignored but required fields are checked.
func ParseFromJSON(data []byte) (*Intent, error) {
	// Extract JSON from the response — AI sometimes wraps it in markdown code fences.
	data = extractJSON(data)

	var i Intent
	if err := json.Unmarshal(data, &i); err != nil {
		return nil, fmt.Errorf("parsing intent JSON: %w", err)
	}

	if err := validate(&i); err != nil {
		return nil, fmt.Errorf("invalid intent: %w", err)
	}

	// Apply safe defaults.
	applyDefaults(&i)

	return &i, nil
}

// ParseFromString is a convenience wrapper for string input.
func ParseFromString(s string) (*Intent, error) {
	return ParseFromJSON([]byte(s))
}

// validate enforces required fields and safety constraints.
func validate(i *Intent) error {
	if i.Intent == "" {
		return fmt.Errorf("intent field is required")
	}

	validIntents := map[IntentType]bool{
		IntentSearchText:       true,
		IntentFindFiles:        true,
		IntentSearchLogs:       true,
		IntentCountOccurrences: true,
		IntentSearchJSON:       true,
		IntentUnknown:          true,
	}
	if !validIntents[i.Intent] {
		return fmt.Errorf("unknown intent type: %q", i.Intent)
	}

	// Text-search and count require a pattern.
	if (i.Intent == IntentSearchText || i.Intent == IntentCountOccurrences) && i.Pattern == "" {
		return fmt.Errorf("intent %q requires a non-empty pattern", i.Intent)
	}

	// Validate target paths don't escape root unsafely.
	for _, p := range i.TargetPaths {
		if strings.Contains(p, "..") {
			return fmt.Errorf("target path %q contains path traversal", p)
		}
	}

	// Validate context lines is reasonable.
	if i.ContextLines > 20 {
		i.ContextLines = 20 // cap silently
	}

	return nil
}

// applyDefaults fills in sensible values for optional fields.
func applyDefaults(i *Intent) {
	if len(i.TargetPaths) == 0 {
		i.TargetPaths = []string{"./"}
	}
	if i.ToolPreference == "" {
		switch i.Intent {
		case IntentFindFiles:
			i.ToolPreference = "fd"
		case IntentSearchJSON:
			i.ToolPreference = "jq"
		default:
			i.ToolPreference = "rg"
		}
	}
}

// extractJSON attempts to pull a JSON object out of an AI response that
// may contain prose, markdown code fences, or explanation text.
func extractJSON(data []byte) []byte {
	s := strings.TrimSpace(string(data))

	// Strip markdown code fences: ```json ... ``` or ``` ... ```
	for _, fence := range []string{"```json", "```"} {
		if idx := strings.Index(s, fence); idx != -1 {
			s = s[idx+len(fence):]
			if end := strings.Index(s, "```"); end != -1 {
				s = s[:end]
			}
			s = strings.TrimSpace(s)
			break
		}
	}

	// Find the first { and last } to extract the JSON object.
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		s = s[start : end+1]
	}

	return []byte(s)
}

// SystemPrompt returns the system prompt to instruct the AI to produce Intent JSON.
// It is deterministic and does not depend on user input.
func SystemPrompt() string {
	return `You are a command intent parser for a safe local terminal assistant called cmdctx.

Your job is to parse a natural language search/investigation request and return a strict JSON object.

You MUST return ONLY valid JSON matching this schema. No prose, no explanation outside the JSON.

JSON Schema:
{
  "intent": "search_text" | "find_files" | "search_logs" | "count_occurrences" | "search_json" | "unknown",
  "pattern": "string (required for search_text and count_occurrences)",
  "target_paths": ["array of paths, default: ['./']"],
  "file_globs": ["array of file globs, e.g. '*.php', '*.go'"],
  "exclude_paths": ["array of dirs/patterns to exclude"],
  "time_scope": "string or null (e.g. 'today', '1h', '24h')",
  "tool_preference": "rg | grep | fd | find | jq",
  "count_only": false,
  "show_line_numbers": true,
  "context_lines": 0,
  "case_sensitive": false,
  "max_results": 0,
  "explanation": "brief human-readable explanation of what you will do",
  "assumptions": ["list any assumptions you made to resolve ambiguity"]
}

Rules:
- Always include vendor, node_modules, .git in exclude_paths unless the user explicitly asks for them
- Prefer "rg" for text search, "fd" for file discovery
- Keep target_paths relative (e.g. "./") unless user specifies absolute paths
- If the request is ambiguous, make a safe assumption and document it in assumptions
- Never invent shell commands — only fill in the intent fields
- If you cannot parse the intent, use "unknown"
`
}

// BuildUserPrompt constructs the user-facing prompt including context snippets.
func BuildUserPrompt(request string, contextSnippets []string) string {
	var b strings.Builder
	b.WriteString("User request: ")
	b.WriteString(request)
	b.WriteString("\n\n")

	if len(contextSnippets) > 0 {
		b.WriteString("Relevant context:\n")
		for _, s := range contextSnippets {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("Return only the JSON intent object.")
	return b.String()
}
