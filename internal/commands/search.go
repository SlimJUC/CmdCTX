package commands

import (
	"fmt"

	"github.com/slim/cmdctx/internal/intent"
)

// buildTextSearch builds a text search command using rg (preferred) or grep (fallback).
// It generates a typed argv — never a raw shell string.
func buildTextSearch(i *intent.Intent, tools map[string]string) ([]string, error) {
	if i.Pattern == "" {
		return nil, fmt.Errorf("text search requires a non-empty pattern")
	}

	tool := preferredTool(i.ToolPreference, []string{"rg", "grep"}, tools)

	switch tool {
	case "rg":
		return buildRipgrepSearch(i), nil
	case "grep":
		return buildGrepSearch(i), nil
	default:
		return buildGrepSearch(i), nil
	}
}

// buildRipgrepSearch builds an rg command from the intent.
func buildRipgrepSearch(i *intent.Intent) []string {
	argv := []string{"rg"}

	// Core flags.
	if i.ShowLineNumbers {
		argv = append(argv, "-n")
	}

	if !i.CaseSensitive {
		argv = append(argv, "-i")
	}

	// Count-only mode.
	if i.CountOnly {
		argv = append(argv, "-c")
	}

	// Context lines.
	if i.ContextLines > 0 {
		argv = append(argv, fmt.Sprintf("-C%d", i.ContextLines))
	}

	// Max results.
	if i.MaxResults > 0 {
		argv = append(argv, fmt.Sprintf("-m%d", i.MaxResults))
	}

	// File glob filters.
	for _, glob := range i.FileGlobs {
		argv = append(argv, "--glob", glob)
	}

	// Exclude paths.
	for _, excl := range i.ExcludePaths {
		argv = append(argv, "--glob", "!"+excl+"/**")
		argv = append(argv, "--glob", "!"+excl)
	}

	// Pattern (always last before paths for clarity).
	argv = append(argv, i.Pattern)

	// Target paths.
	argv = append(argv, i.TargetPaths...)

	return argv
}

// buildGrepSearch builds a grep command from the intent (fallback when rg is unavailable).
func buildGrepSearch(i *intent.Intent) []string {
	argv := []string{"grep", "-r"}

	if i.ShowLineNumbers {
		argv = append(argv, "-n")
	}

	if !i.CaseSensitive {
		argv = append(argv, "-i")
	}

	if i.CountOnly {
		argv = append(argv, "-c")
	}

	if i.ContextLines > 0 {
		argv = append(argv, fmt.Sprintf("-C%d", i.ContextLines))
	}

	// Include patterns — grep uses --include for globs.
	for _, glob := range i.FileGlobs {
		argv = append(argv, "--include="+glob)
	}

	// Exclude directories.
	for _, excl := range i.ExcludePaths {
		argv = append(argv, "--exclude-dir="+excl)
	}

	argv = append(argv, i.Pattern)
	argv = append(argv, i.TargetPaths...)

	return argv
}
