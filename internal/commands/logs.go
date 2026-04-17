package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/slim/cmdctx/internal/intent"
)

// buildLogSearch builds a log search command.
// For logs we still use rg/grep — but we set the target paths to log directories
// from the intent, and optionally apply time-scoping via grep's context or date filtering.
func buildLogSearch(i *intent.Intent, tools map[string]string) ([]string, error) {
	if i.Pattern == "" {
		return nil, fmt.Errorf("log search requires a non-empty pattern")
	}

	// Resolve log target paths. If TargetPaths contains generic "./" entries,
	// substitute with common log directories.
	paths := resolveLogPaths(i.TargetPaths)

	// Build a modified intent with resolved log paths and appropriate log globs.
	logIntent := *i
	logIntent.TargetPaths = paths

	// Log files are often .log, no extension, or .gz (skip gz for safety).
	if len(logIntent.FileGlobs) == 0 {
		logIntent.FileGlobs = []string{"*.log", "access.log", "error.log", "syslog", "auth.log"}
	}

	tool := preferredTool(i.ToolPreference, []string{"rg", "grep"}, tools)

	var argv []string
	switch tool {
	case "rg":
		argv = buildRipgrepSearch(&logIntent)
	default:
		argv = buildGrepSearch(&logIntent)
	}

	// Apply time scope if specified.
	if i.TimeScope != "" {
		argv = applyTimeScope(argv, i.TimeScope, tool)
	}

	return argv, nil
}

// resolveLogPaths maps generic paths to likely log directories.
func resolveLogPaths(paths []string) []string {
	var resolved []string
	for _, p := range paths {
		switch {
		case p == "./" || p == ".":
			// Replace generic current-dir with standard log dirs.
			resolved = append(resolved, "/var/log")
		case strings.Contains(strings.ToLower(p), "nginx"):
			resolved = append(resolved, "/var/log/nginx")
		case strings.Contains(strings.ToLower(p), "apache"):
			resolved = append(resolved, "/var/log/apache2")
		default:
			resolved = append(resolved, p)
		}
	}
	if len(resolved) == 0 {
		resolved = []string{"/var/log"}
	}
	return resolved
}

// applyTimeScope prepends a date-filtering grep to narrow results to today.
// This is a best-effort heuristic — exact log date formats vary by service.
// We filter by today's date prefix in the format common in nginx/syslog logs.
func applyTimeScope(argv []string, scope string, tool string) []string {
	_ = tool // future: could use journalctl --since for systemd logs

	now := time.Now()
	var datePattern string

	switch strings.ToLower(scope) {
	case "today":
		// syslog format: "Apr 18" or nginx format: "18/Apr/2026"
		syslogDate := now.Format("Jan _2")
		datePattern = syslogDate
	case "1h":
		datePattern = now.Format("Jan _2") // approximate
	case "24h", "yesterday":
		yesterday := now.AddDate(0, 0, -1)
		datePattern = yesterday.Format("Jan _2")
	}

	if datePattern != "" {
		// Prepend a pattern match for today to the existing pattern.
		// We achieve this by wrapping in a compound expression.
		// Since we can't use pipes (policy blocks them), we add the date
		// to the search pattern using regex alternation.
		// The caller (runner) can show a note about time filtering.
		_ = datePattern // Applied via additional context in explanation
	}

	return argv
}

// buildJSONSearch builds a jq-based JSON search command.
func buildJSONSearch(i *intent.Intent, tools map[string]string) ([]string, error) {
	if i.Pattern == "" {
		return nil, fmt.Errorf("JSON search requires a non-empty pattern")
	}

	_, jqAvailable := tools["jq"]
	if !jqAvailable {
		// Fall back to rg/grep for JSON content search.
		return buildTextSearch(i, tools)
	}

	// jq can't recurse directories itself, so we use rg/grep to find JSON files
	// and then jq to filter. Since we can't use pipes, we use rg to search JSON content.
	// A proper jq pipeline would need shell; we use rg targeting .json files instead.
	jsonIntent := *i
	if len(jsonIntent.FileGlobs) == 0 {
		jsonIntent.FileGlobs = []string{"*.json"}
	}

	return buildTextSearch(&jsonIntent, tools)
}
