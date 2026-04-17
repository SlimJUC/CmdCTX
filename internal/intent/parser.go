package intent

import (
	"context"
	"fmt"

	"github.com/slim/cmdctx/internal/ai"
)

// Parser uses an AI provider to convert natural language into a structured Intent.
type Parser struct {
	provider ai.Provider
}

// NewParser creates a Parser backed by the given AI provider.
func NewParser(provider ai.Provider) *Parser {
	return &Parser{provider: provider}
}

// Parse sends the user request to the AI provider and returns a validated Intent.
// contextSnippets are short relevant context excerpts retrieved from local context files.
func (p *Parser) Parse(ctx context.Context, request string, contextSnippets []string) (*Intent, error) {
	req := ai.CompletionRequest{
		SystemPrompt: SystemPrompt(),
		UserPrompt:   BuildUserPrompt(request, contextSnippets),
		MaxTokens:    512,
		Temperature:  0.1, // low temperature for deterministic structured output
	}

	resp, err := p.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI completion failed: %w", err)
	}

	parsed, err := ParseFromString(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("AI returned unparseable response: %w\nRaw response: %s", err, resp.Content)
	}

	return parsed, nil
}

// ParseWithFallback attempts AI parsing, falls back to rule-based parsing on failure.
// This ensures cmdctx remains useful even when no AI provider is configured.
func (p *Parser) ParseWithFallback(ctx context.Context, request string, contextSnippets []string) (*Intent, string, error) {
	parsed, err := p.Parse(ctx, request, contextSnippets)
	if err != nil {
		// Attempt rule-based fallback.
		fallback, fbErr := RuleBasedParse(request)
		if fbErr != nil {
			return nil, "ai", fmt.Errorf("AI parsing failed: %w; rule-based fallback also failed: %v", err, fbErr)
		}
		return fallback, "rule_based", nil
	}
	return parsed, "ai", nil
}

// RuleBasedParse provides a deterministic, no-AI fallback for common patterns.
// This is intentionally limited — it handles the most common cases only.
// It is always safe because it produces only known-good intent structures.
func RuleBasedParse(request string) (*Intent, error) {
	r := normalisedRequest(request)

	intent := &Intent{
		TargetPaths:     []string{"./"},
		ExcludePaths:    []string{"vendor", "node_modules", ".git"},
		ShowLineNumbers: true,
	}

	// --- Pattern: "find [all] <ext> files [containing <pattern>]" ---
	if matches := matchFindFiles(r); matches != nil {
		intent.Intent = IntentFindFiles
		intent.FileGlobs = matches.globs
		intent.ExcludePaths = append(intent.ExcludePaths, matches.excludes...)
		intent.ToolPreference = "fd"
		intent.Explanation = fmt.Sprintf("Find files matching: %v", matches.globs)
		if matches.pattern != "" {
			// Containing a pattern → search_text instead
			intent.Intent = IntentSearchText
			intent.Pattern = matches.pattern
			intent.ToolPreference = "rg"
			intent.Explanation = fmt.Sprintf("Search for %q in %v files", matches.pattern, matches.globs)
		}
		return intent, nil
	}

	// --- Pattern: "search <location> for <pattern>" ---
	if matches := matchSearch(r); matches != nil {
		intent.Intent = IntentSearchText
		intent.Pattern = matches.pattern
		intent.ToolPreference = "rg"
		if matches.isLog {
			intent.Intent = IntentSearchLogs
		}
		if matches.targetPaths != nil {
			intent.TargetPaths = matches.targetPaths
		}
		intent.Explanation = fmt.Sprintf("Search for %q", matches.pattern)
		return intent, nil
	}

	// --- Pattern: "count <pattern> [in <location>]" ---
	if matches := matchCount(r); matches != nil {
		intent.Intent = IntentCountOccurrences
		intent.Pattern = matches.pattern
		intent.CountOnly = true
		intent.ToolPreference = "rg"
		intent.Explanation = fmt.Sprintf("Count occurrences of %q", matches.pattern)
		return intent, nil
	}

	// --- Pattern: "look for <pattern>" / "find <pattern>" ---
	if matches := matchGenericSearch(r); matches != nil {
		intent.Intent = IntentSearchText
		intent.Pattern = matches.pattern
		intent.ToolPreference = "rg"
		intent.Explanation = fmt.Sprintf("Search for %q", matches.pattern)
		return intent, nil
	}

	return nil, fmt.Errorf("unable to determine intent from: %q", request)
}

// ---- Rule-based match helpers ------------------------------------------------

type fileMatch struct {
	globs    []string
	excludes []string
	pattern  string
}

type searchMatch struct {
	pattern     string
	targetPaths []string
	isLog       bool
}

type countMatch struct {
	pattern string
}

type genericMatch struct {
	pattern string
}

func normalisedRequest(r string) string {
	// Lowercase but preserve original for extracting quoted patterns.
	return r
}

func matchFindFiles(r string) *fileMatch {
	low := toLowerCase(r)
	if !containsAny(low, "find", "locate", "list") {
		return nil
	}

	m := &fileMatch{}

	// Extract file extensions.
	exts := extractExtensions(low)
	if len(exts) == 0 {
		return nil
	}
	for _, ext := range exts {
		m.globs = append(m.globs, "*."+ext)
	}

	// Extract "except/excluding <dir>" clauses.
	m.excludes = extractExcludes(low)

	// Extract "containing <pattern>" clause.
	m.pattern = extractContaining(r) // use original for case

	return m
}

func matchSearch(r string) *searchMatch {
	low := toLowerCase(r)
	if !containsAny(low, "search", "grep", "look for") {
		return nil
	}

	m := &searchMatch{}

	// Detect log context.
	if containsAny(low, "log", "logs", "/var/log") {
		m.isLog = true
		m.targetPaths = logPaths(low)
	}

	// Extract "for <pattern>" clause.
	m.pattern = extractAfterKeyword(r, "for")
	if m.pattern == "" {
		return nil
	}

	return m
}

func matchCount(r string) *countMatch {
	low := toLowerCase(r)
	if !containsAny(low, "count", "how many") {
		return nil
	}

	pattern := extractAfterKeyword(r, "occurrences of")
	if pattern == "" {
		pattern = extractAfterKeyword(r, "of")
	}
	if pattern == "" {
		return nil
	}

	return &countMatch{pattern: pattern}
}

func matchGenericSearch(r string) *genericMatch {
	low := toLowerCase(r)
	_ = low
	// Treat entire request minus stop words as pattern.
	pattern := stripStopWords(r)
	if pattern == "" {
		return nil
	}
	return &genericMatch{pattern: pattern}
}

func extractExtensions(s string) []string {
	// Look for common patterns: "php files", "go files", ".php files"
	extMap := map[string]string{
		"php": "php", "go": "go", "js": "js", "ts": "ts",
		"py": "py", "rb": "rb", "java": "java", "rs": "rs",
		"yaml": "yaml", "yml": "yml", "json": "json",
		"css": "css", "html": "html", "sh": "sh",
		"jsx": "jsx", "tsx": "tsx", "vue": "vue",
	}
	var found []string
	for keyword, ext := range extMap {
		// Match "php files" or ".php files" or ".php"
		if containsWord(s, keyword+" files") || containsWord(s, "."+keyword+" files") ||
			containsWord(s, keyword+" file") {
			found = append(found, ext)
		}
	}
	return found
}

func extractExcludes(s string) []string {
	var excludes []string
	keywords := []string{"except", "excluding", "without", "ignore", "not in", "skip"}
	for _, kw := range keywords {
		if after := extractAfterKeyword(s, kw); after != "" {
			// Split on "and" and commas.
			parts := splitAndTrim(after, " and ", ", ", ",")
			excludes = append(excludes, parts...)
		}
	}
	return excludes
}

func extractContaining(s string) string {
	return extractAfterKeyword(s, "containing")
}

func extractAfterKeyword(s, keyword string) string {
	low := toLowerCase(s)
	kl := toLowerCase(keyword)
	idx := indexOf(low, kl)
	if idx == -1 {
		return ""
	}
	after := s[idx+len(keyword):]
	// Trim leading spaces and stop at "except", "excluding", newline.
	after = trimLeading(after)
	stopWords := []string{" except ", " excluding ", " without ", " not in ", "\n"}
	for _, sw := range stopWords {
		if i := indexOf(toLowerCase(after), sw); i != -1 {
			after = after[:i]
		}
	}
	return trimQuotes(after)
}

func logPaths(s string) []string {
	candidates := map[string]string{
		"nginx":   "/var/log/nginx",
		"apache":  "/var/log/apache2",
		"syslog":  "/var/log/syslog",
		"auth":    "/var/log/auth.log",
		"journal": "/var/log",
	}
	var paths []string
	for keyword, path := range candidates {
		if containsWord(s, keyword) {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		paths = []string{"/var/log"}
	}
	return paths
}

func stripStopWords(s string) string {
	stops := []string{
		"find", "search", "look", "for", "all", "the", "a", "an",
		"files", "file", "in", "on", "this", "project",
	}
	words := splitAndTrim(s, " ")
	var kept []string
	for _, w := range words {
		keep := true
		wl := toLowerCase(w)
		for _, stop := range stops {
			if wl == stop {
				keep = false
				break
			}
		}
		if keep {
			kept = append(kept, w)
		}
	}
	return joinStrings(kept, " ")
}

// ---- String helpers ----------------------------------------------------------

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) != -1 {
			return true
		}
	}
	return false
}

func containsWord(s, word string) bool {
	return indexOf(s, word) != -1
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimLeading(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func trimQuotes(s string) string {
	s = trimLeading(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func splitAndTrim(s string, seps ...string) []string {
	if len(seps) == 0 {
		return []string{s}
	}
	sep := seps[0]
	parts := splitOn(s, sep)
	if len(seps) > 1 {
		var result []string
		for _, p := range parts {
			result = append(result, splitAndTrim(p, seps[1:]...)...)
		}
		return result
	}
	var result []string
	for _, p := range parts {
		p = trimLeading(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitOn(s, sep string) []string {
	var parts []string
	for {
		idx := indexOf(s, sep)
		if idx == -1 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
	return parts
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
