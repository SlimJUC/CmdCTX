// Package commands implements safe, structured command builders.
// Commands are always assembled as typed argv slices — never as raw shell strings.
// The argv is only joined into a display string for human review.
// Policy validation happens before any command leaves this package.
package commands

import (
	"fmt"

	"github.com/slim/cmdctx/internal/intent"
	"github.com/slim/cmdctx/internal/policy"
	"github.com/slim/cmdctx/internal/utils"
)

// Result holds a fully-built, validated command ready for display or execution.
type Result struct {
	// Argv is the command as a typed argument list (safe for exec.Command).
	Argv []string
	// Display is the human-readable shell-like representation.
	Display string
	// Explanation is a human-readable explanation of what the command does.
	Explanation string
	// Risk is the policy-assessed risk level.
	Risk policy.RiskLevel
	// Warnings are non-fatal advisory messages.
	Warnings []string
	// Assumptions are inherited from the intent.
	Assumptions []string
	// ParsedBy indicates whether the intent came from "ai" or "rule_based".
	ParsedBy string
}

// Build constructs a safe command Result from a parsed Intent.
// It selects the appropriate builder, validates against policy, and returns
// a ready-to-display Result.
func Build(i *intent.Intent, pol *policy.Policy, tools map[string]string) (*Result, error) {
	var argv []string
	var err error

	switch i.Intent {
	case intent.IntentSearchText, intent.IntentCountOccurrences:
		argv, err = buildTextSearch(i, tools)
	case intent.IntentFindFiles:
		argv, err = buildFileSearch(i, tools)
	case intent.IntentSearchLogs:
		argv, err = buildLogSearch(i, tools)
	case intent.IntentSearchJSON:
		argv, err = buildJSONSearch(i, tools)
	case intent.IntentUnknown:
		return nil, fmt.Errorf("intent is 'unknown' — please clarify your request")
	default:
		return nil, fmt.Errorf("unsupported intent type: %q", i.Intent)
	}
	if err != nil {
		return nil, err
	}

	if len(argv) == 0 {
		return nil, fmt.Errorf("builder produced empty command")
	}

	// Always validate against policy before returning.
	validation := pol.Validate(argv)
	if !validation.Allowed {
		return nil, fmt.Errorf("command blocked by policy: %s", validation.Reason)
	}

	return &Result{
		Argv:        argv,
		Display:     utils.JoinArgs(argv),
		Explanation: i.Explanation,
		Risk:        validation.Risk,
		Warnings:    append(validation.Warnings, warningsFromIntent(i)...),
		Assumptions: i.Assumptions,
	}, nil
}

// warningsFromIntent produces advisory warnings based on intent properties.
func warningsFromIntent(i *intent.Intent) []string {
	var w []string
	for _, path := range i.TargetPaths {
		if path == "/" || path == "/etc" || path == "/var" || path == "/home" {
			w = append(w, fmt.Sprintf("searching system path %q — this may be slow and include sensitive files", path))
		}
	}
	if len(i.ExcludePaths) == 0 {
		w = append(w, "no directories are excluded — consider adding vendor, node_modules, .git")
	}
	return w
}

// preferredTool returns the best available tool from the preference list.
// Falls back through the list until something is found in the tools map.
func preferredTool(preference string, fallbacks []string, tools map[string]string) string {
	candidates := append([]string{preference}, fallbacks...)
	for _, c := range candidates {
		if _, ok := tools[c]; ok {
			return c
		}
	}
	// Last resort: return the last fallback even if not in PATH — the runner will fail gracefully.
	if len(fallbacks) > 0 {
		return fallbacks[len(fallbacks)-1]
	}
	return preference
}
