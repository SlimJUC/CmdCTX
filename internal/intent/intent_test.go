package intent

import (
	"strings"
	"testing"
)

// ---- ParseFromJSON tests --------------------------------------------------------

func TestParseFromJSON_ValidSearchText(t *testing.T) {
	raw := `{
		"intent": "search_text",
		"pattern": "payment failed",
		"target_paths": ["./"],
		"file_globs": ["*.php"],
		"exclude_paths": ["vendor", "node_modules"],
		"show_line_numbers": true,
		"count_only": false,
		"context_lines": 0,
		"case_sensitive": false,
		"explanation": "Search for payment failed in PHP files",
		"assumptions": []
	}`

	i, err := ParseFromString(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.Intent != IntentSearchText {
		t.Errorf("expected search_text, got %q", i.Intent)
	}
	if i.Pattern != "payment failed" {
		t.Errorf("expected pattern 'payment failed', got %q", i.Pattern)
	}
	if len(i.FileGlobs) != 1 || i.FileGlobs[0] != "*.php" {
		t.Errorf("expected file_globs [*.php], got %v", i.FileGlobs)
	}
}

func TestParseFromJSON_ValidFindFiles(t *testing.T) {
	raw := `{
		"intent": "find_files",
		"target_paths": ["./"],
		"file_globs": ["*.go"],
		"exclude_paths": [".git"],
		"show_line_numbers": false,
		"count_only": false,
		"context_lines": 0,
		"case_sensitive": false
	}`

	i, err := ParseFromString(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.Intent != IntentFindFiles {
		t.Errorf("expected find_files, got %q", i.Intent)
	}
	if i.ToolPreference != "fd" {
		t.Errorf("expected default tool_preference=fd for find_files, got %q", i.ToolPreference)
	}
}

func TestParseFromJSON_MissingPattern(t *testing.T) {
	raw := `{
		"intent": "search_text",
		"target_paths": ["./"]
	}`

	_, err := ParseFromString(raw)
	if err == nil {
		t.Error("expected error for missing pattern in search_text")
	}
}

func TestParseFromJSON_UnknownIntent(t *testing.T) {
	raw := `{
		"intent": "destroy_everything",
		"pattern": "test"
	}`

	_, err := ParseFromString(raw)
	if err == nil {
		t.Error("expected error for unknown intent type")
	}
}

func TestParseFromJSON_PathTraversal(t *testing.T) {
	raw := `{
		"intent": "search_text",
		"pattern": "test",
		"target_paths": ["../../etc/passwd"]
	}`

	_, err := ParseFromString(raw)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestParseFromJSON_ContextLinesCapped(t *testing.T) {
	raw := `{
		"intent": "search_text",
		"pattern": "test",
		"context_lines": 100
	}`

	i, err := ParseFromString(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.ContextLines > 20 {
		t.Errorf("expected context_lines capped at 20, got %d", i.ContextLines)
	}
}

func TestParseFromJSON_DefaultTargetPaths(t *testing.T) {
	raw := `{
		"intent": "search_text",
		"pattern": "test"
	}`

	i, err := ParseFromString(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(i.TargetPaths) == 0 {
		t.Error("expected default target_paths to be set")
	}
	if i.TargetPaths[0] != "./" {
		t.Errorf("expected default target path './', got %q", i.TargetPaths[0])
	}
}

func TestExtractJSON_MarkdownFence(t *testing.T) {
	wrapped := "Here is the result:\n```json\n{\"intent\":\"find_files\"}\n```\nEnd."
	extracted := extractJSON([]byte(wrapped))
	if !strings.Contains(string(extracted), `"intent"`) {
		t.Errorf("failed to extract JSON from markdown fence: %q", string(extracted))
	}
}

func TestExtractJSON_ProseSurrounded(t *testing.T) {
	wrapped := `Sure, here you go: {"intent":"search_text","pattern":"hello"} That's it.`
	extracted := extractJSON([]byte(wrapped))
	result := string(extracted)
	if !strings.HasPrefix(result, "{") || !strings.HasSuffix(result, "}") {
		t.Errorf("expected pure JSON object, got: %q", result)
	}
}

// ---- RuleBasedParse tests ------------------------------------------------------

func TestRuleBasedParse_FindPhpFiles(t *testing.T) {
	i, err := RuleBasedParse("find all php files containing payment failed except vendor and node_modules")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.Intent != IntentSearchText {
		t.Errorf("expected search_text (because 'containing' was found), got %q", i.Intent)
	}
	if i.Pattern == "" {
		t.Error("expected pattern to be extracted")
	}
}

func TestRuleBasedParse_SearchNginxLogs(t *testing.T) {
	i, err := RuleBasedParse("search nginx logs for 500 errors")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.Intent != IntentSearchLogs && i.Intent != IntentSearchText {
		t.Errorf("expected search_logs or search_text, got %q", i.Intent)
	}
}

func TestRuleBasedParse_CountOccurrences(t *testing.T) {
	i, err := RuleBasedParse("count occurrences of timeout in logs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.Intent != IntentCountOccurrences {
		t.Errorf("expected count_occurrences, got %q", i.Intent)
	}
	if !i.CountOnly {
		t.Error("expected CountOnly to be true")
	}
}

func TestRuleBasedParse_DefaultExcludes(t *testing.T) {
	i, err := RuleBasedParse("find all go files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(i.ExcludePaths) == 0 {
		t.Error("expected default exclude paths to be set")
	}
}

func TestSystemPrompt_NotEmpty(t *testing.T) {
	sp := SystemPrompt()
	if len(sp) < 100 {
		t.Error("system prompt seems too short")
	}
	if !strings.Contains(sp, "search_text") {
		t.Error("system prompt should mention intent types")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	prompt := BuildUserPrompt("find php files", []string{"[tools] grep available"})
	if !strings.Contains(prompt, "find php files") {
		t.Error("prompt should contain the request")
	}
	if !strings.Contains(prompt, "grep available") {
		t.Error("prompt should contain context snippets")
	}
}
