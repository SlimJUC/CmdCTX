package commands

import (
	"fmt"
	"strings"

	"github.com/slim/cmdctx/internal/intent"
)

// buildFileSearch builds a file-discovery command using fd (preferred) or find (fallback).
func buildFileSearch(i *intent.Intent, tools map[string]string) ([]string, error) {
	tool := preferredTool(i.ToolPreference, []string{"fd", "find"}, tools)

	switch tool {
	case "fd":
		return buildFdSearch(i), nil
	case "find":
		return buildFindSearch(i), nil
	default:
		return buildFindSearch(i), nil
	}
}

// buildFdSearch builds an fd command from the intent.
func buildFdSearch(i *intent.Intent) []string {
	argv := []string{"fd"}

	// fd defaults to hidden-file exclusion, which is safe.
	// Type: file only (not dirs).
	argv = append(argv, "--type", "f")

	// File extension/glob filters.
	for _, glob := range i.FileGlobs {
		// fd uses -e for extensions (strip the *. prefix) or -g for full globs.
		if strings.HasPrefix(glob, "*.") {
			argv = append(argv, "-e", strings.TrimPrefix(glob, "*."))
		} else {
			argv = append(argv, "--glob", glob)
		}
	}

	// Exclude directories.
	for _, excl := range i.ExcludePaths {
		argv = append(argv, "--exclude", excl)
	}

	// Pattern — fd treats positional arg as a filename pattern (regex by default).
	if i.Pattern != "" {
		argv = append(argv, i.Pattern)
	} else {
		// No pattern: match everything.
		argv = append(argv, ".")
	}

	// Search roots.
	argv = append(argv, i.TargetPaths...)

	return argv
}

// buildFindSearch builds a POSIX find command from the intent.
// Uses only safe, read-only flags. Never uses -exec with modification commands.
func buildFindSearch(i *intent.Intent) []string {
	argv := []string{"find"}

	// Target paths (find takes paths before flags).
	argv = append(argv, i.TargetPaths...)

	// Exclude prune patterns.
	if len(i.ExcludePaths) > 0 {
		for idx, excl := range i.ExcludePaths {
			argv = append(argv, "-name", excl, "-prune")
			if idx < len(i.ExcludePaths)-1 {
				argv = append(argv, "-o")
			}
		}
		argv = append(argv, "-o")
	}

	// Type: file only.
	argv = append(argv, "-type", "f")

	// File name/extension filters.
	if len(i.FileGlobs) > 0 {
		argv = append(argv, "(")
		for idx, glob := range i.FileGlobs {
			argv = append(argv, "-name", glob)
			if idx < len(i.FileGlobs)-1 {
				argv = append(argv, "-o")
			}
		}
		argv = append(argv, ")")
	}

	// Pattern — find uses -name for filename patterns, not content search.
	// For content, a separate search would be needed; here we just add name matching.
	if i.Pattern != "" && len(i.FileGlobs) == 0 {
		argv = append(argv, "-name", fmt.Sprintf("*%s*", i.Pattern))
	}

	// Output: just print the paths.
	argv = append(argv, "-print")

	return argv
}
