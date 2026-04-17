package commands

import (
	"strings"
	"testing"

	"github.com/slim/cmdctx/internal/intent"
	"github.com/slim/cmdctx/internal/policy"
)

func testPolicy() *policy.Policy { return policy.Default() }

func testTools() map[string]string {
	return map[string]string{
		"grep": "/usr/bin/grep",
		"find": "/usr/bin/find",
	}
}

func testToolsWithRg() map[string]string {
	return map[string]string{
		"rg":   "/usr/bin/rg",
		"grep": "/usr/bin/grep",
		"find": "/usr/bin/find",
	}
}

// ---- Text search builder tests -----------------------------------------------

func TestBuildTextSearch_GrepFallback(t *testing.T) {
	i := &intent.Intent{
		Intent:          intent.IntentSearchText,
		Pattern:         "payment failed",
		TargetPaths:     []string{"./"},
		FileGlobs:       []string{"*.php"},
		ExcludePaths:    []string{"vendor", "node_modules", ".git"},
		ShowLineNumbers: true,
		ToolPreference:  "rg",
	}

	// Use tools without rg — should fall back to grep.
	argv, err := buildTextSearch(i, testTools())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if argv[0] != "grep" {
		t.Errorf("expected grep as fallback, got %q", argv[0])
	}
	if !containsArg(argv, "--include=*.php") {
		t.Error("expected --include=*.php in grep command")
	}
	if !containsArg(argv, "-n") {
		t.Error("expected -n (line numbers) in grep command")
	}
	if !containsArg(argv, "payment failed") {
		t.Error("expected pattern in grep command")
	}
}

func TestBuildTextSearch_RgPreferred(t *testing.T) {
	i := &intent.Intent{
		Intent:          intent.IntentSearchText,
		Pattern:         "authToken",
		TargetPaths:     []string{"./"},
		FileGlobs:       []string{"*.js", "*.ts"},
		ExcludePaths:    []string{"node_modules"},
		ShowLineNumbers: true,
		ToolPreference:  "rg",
	}

	argv, err := buildTextSearch(i, testToolsWithRg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if argv[0] != "rg" {
		t.Errorf("expected rg when available, got %q", argv[0])
	}
	if !containsArg(argv, "--glob") {
		t.Error("expected --glob flag for rg")
	}
	if !containsArg(argv, "authToken") {
		t.Error("expected pattern in rg command")
	}
}

func TestBuildTextSearch_CountOnly(t *testing.T) {
	i := &intent.Intent{
		Intent:         intent.IntentCountOccurrences,
		Pattern:        "timeout",
		TargetPaths:    []string{"./"},
		ExcludePaths:   []string{".git"},
		CountOnly:      true,
		ToolPreference: "rg",
	}

	argv, err := buildTextSearch(i, testTools())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(argv, "-c") {
		t.Error("expected -c (count) flag")
	}
}

func TestBuildTextSearch_EmptyPattern(t *testing.T) {
	i := &intent.Intent{
		Intent:      intent.IntentSearchText,
		Pattern:     "",
		TargetPaths: []string{"./"},
	}
	_, err := buildTextSearch(i, testTools())
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

// ---- File search builder tests -----------------------------------------------

func TestBuildFileSearch_FindFallback(t *testing.T) {
	i := &intent.Intent{
		Intent:         intent.IntentFindFiles,
		FileGlobs:      []string{"*.yml", "*.yaml"},
		ExcludePaths:   []string{".git"},
		TargetPaths:    []string{"./"},
		ToolPreference: "fd",
	}

	// No fd or rg available — should fall back to find.
	argv, err := buildFileSearch(i, testTools())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if argv[0] != "find" {
		t.Errorf("expected find as fallback, got %q", argv[0])
	}
	if !containsArg(argv, "-type") {
		t.Error("expected -type flag in find command")
	}
}

// ---- Build (top-level) tests -------------------------------------------------

func TestBuild_ValidSearchText(t *testing.T) {
	i := &intent.Intent{
		Intent:          intent.IntentSearchText,
		Pattern:         "redis timeout",
		TargetPaths:     []string{"./"},
		ExcludePaths:    []string{"vendor", "node_modules", ".git"},
		ShowLineNumbers: true,
		ToolPreference:  "rg",
	}

	result, err := Build(i, testPolicy(), testTools())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(result.Argv) == 0 {
		t.Error("expected non-empty argv")
	}
	if result.Display == "" {
		t.Error("expected non-empty display string")
	}
	if result.Risk == "" {
		t.Error("expected risk to be set")
	}
}

func TestBuild_UnknownIntent(t *testing.T) {
	i := &intent.Intent{
		Intent: intent.IntentUnknown,
	}

	_, err := Build(i, testPolicy(), testTools())
	if err == nil {
		t.Error("expected error for unknown intent")
	}
}

func TestBuild_PolicyBlocked(t *testing.T) {
	// Manually construct an argv that would pass intent parsing but fail policy.
	// We test this by directly calling Validate which is what Build uses.
	pol := testPolicy()
	result := pol.Validate([]string{"rm", "-rf", "."})
	if result.Allowed {
		t.Error("expected rm to be blocked by policy")
	}
}

// ---- Helper ------------------------------------------------------------------

func containsArg(argv []string, arg string) bool {
	for _, a := range argv {
		if a == arg || strings.Contains(a, arg) {
			return true
		}
	}
	return false
}
