// Package retrieval implements a lightweight lexical/tag-based retrieval layer
// over local context files stored in SQLite. No vector DB is needed for MVP.
// The goal is to send only relevant context snippets to the AI, not entire files.
package retrieval

import (
	"fmt"
	"strings"

	"github.com/slim/cmdctx/internal/contextgen"
	"github.com/slim/cmdctx/internal/history"
)

// Retriever retrieves relevant context snippets for a user request.
type Retriever struct {
	store *history.Store
}

// New creates a Retriever backed by the given history store.
func New(store *history.Store) *Retriever {
	return &Retriever{store: store}
}

// RelevantSnippets returns a short list of context snippet strings relevant
// to the user request. These are sent to the AI as additional context.
// Maximum 5 snippets, each at most 200 characters, to keep prompts small.
func (r *Retriever) RelevantSnippets(request string) []string {
	tags := extractTags(request)

	chunks, err := r.store.QueryContextChunks(tags, 8)
	if err != nil || len(chunks) == 0 {
		return nil
	}

	// Score and rank by relevance to the request.
	type scored struct {
		content string
		score   int
	}
	var candidates []scored
	requestLower := strings.ToLower(request)

	for _, chunk := range chunks {
		score := 0
		contentLower := strings.ToLower(chunk.Content)

		// Keyword overlap scoring.
		words := strings.Fields(requestLower)
		for _, word := range words {
			if len(word) > 3 && strings.Contains(contentLower, word) {
				score++
			}
		}
		// Tag match bonus.
		for _, tag := range tags {
			for _, chunkTag := range chunk.Tags {
				if strings.EqualFold(tag, chunkTag) {
					score += 2
				}
			}
		}

		if score > 0 {
			content := truncateSnippet(chunk.Content, 200)
			candidates = append(candidates, scored{
				content: fmt.Sprintf("[%s] %s", chunk.Section, content),
				score:   score,
			})
		}
	}

	// Sort by score descending (simple insertion sort is fine for small slices).
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Return top 5.
	maxSnippets := 5
	if len(candidates) < maxSnippets {
		maxSnippets = len(candidates)
	}
	snippets := make([]string, maxSnippets)
	for i := 0; i < maxSnippets; i++ {
		snippets[i] = candidates[i].content
	}
	return snippets
}

// IndexMachineContext indexes machine context into the SQLite store for retrieval.
func (r *Retriever) IndexMachineContext(ctx *contextgen.MachineContext) error {
	// Index tools.
	if len(ctx.ToolsAvailable) > 0 {
		toolNames := make([]string, 0, len(ctx.ToolsAvailable))
		for name := range ctx.ToolsAvailable {
			toolNames = append(toolNames, name)
		}
		if err := r.store.StoreContextChunk(
			"machine-context", "tools",
			"Available tools: "+strings.Join(toolNames, ", "),
			[]string{"tools", "search", "machine"},
		); err != nil {
			return err
		}
	}

	// Index log directories.
	if len(ctx.LogDirs) > 0 {
		if err := r.store.StoreContextChunk(
			"machine-context", "log_dirs",
			"Log directories: "+strings.Join(ctx.LogDirs, ", "),
			[]string{"logs", "log", "nginx", "apache", "syslog"},
		); err != nil {
			return err
		}
	}

	// Index config directories.
	if len(ctx.ConfigDirs) > 0 {
		if err := r.store.StoreContextChunk(
			"machine-context", "config_dirs",
			"Config directories: "+strings.Join(ctx.ConfigDirs, ", "),
			[]string{"config", "conf", "settings", "nginx"},
		); err != nil {
			return err
		}
	}

	// Index search roots.
	if len(ctx.SearchRoots) > 0 {
		if err := r.store.StoreContextChunk(
			"machine-context", "search_roots",
			"Search roots: "+strings.Join(ctx.SearchRoots, ", "),
			[]string{"search", "project", "root", "code"},
		); err != nil {
			return err
		}
	}

	// Index detected stacks.
	if len(ctx.Frameworks) > 0 {
		stacks := make([]string, 0, len(ctx.Frameworks))
		for _, fw := range ctx.Frameworks {
			stacks = append(stacks, fw.Name)
		}
		stackTags := append([]string{"framework", "stack"}, stacks...)
		if err := r.store.StoreContextChunk(
			"machine-context", "stacks",
			"Detected stacks: "+strings.Join(stacks, ", "),
			stackTags,
		); err != nil {
			return err
		}
	}

	return nil
}

// IndexProjectContext indexes project context into the SQLite store for retrieval.
func (r *Retriever) IndexProjectContext(ctx *contextgen.ProjectContext) error {
	source := "project-context:" + ctx.ProjectRoot

	// Index frameworks.
	if len(ctx.Frameworks) > 0 {
		names := make([]string, 0, len(ctx.Frameworks))
		fwTags := []string{"framework", "project"}
		for _, fw := range ctx.Frameworks {
			names = append(names, fw.Name)
			fwTags = append(fwTags, strings.ToLower(fw.Name))
		}
		if err := r.store.StoreContextChunk(
			source, "frameworks",
			"Project frameworks: "+strings.Join(names, ", "),
			fwTags,
		); err != nil {
			return err
		}
	}

	// Index important files (cap at 20 to avoid bloated snippets).
	if len(ctx.ImportantFiles) > 0 {
		files := ctx.ImportantFiles
		if len(files) > 20 {
			files = files[:20]
		}
		if err := r.store.StoreContextChunk(
			source, "important_files",
			"Important files: "+strings.Join(files, ", "),
			[]string{"files", "config", "manifest", "project"},
		); err != nil {
			return err
		}
	}

	// Index log dirs.
	if len(ctx.LogDirs) > 0 {
		if err := r.store.StoreContextChunk(
			source, "log_dirs",
			"Project log directories: "+strings.Join(ctx.LogDirs, ", "),
			[]string{"logs", "log"},
		); err != nil {
			return err
		}
	}

	return nil
}

// IndexFromFiles indexes context from the standard machine context file.
// This is called after context generation or on `cmdctx refresh`.
func (r *Retriever) IndexFromFiles() error {
	mc, err := contextgen.LoadMachineContext()
	if err == nil {
		if indexErr := r.IndexMachineContext(mc); indexErr != nil {
			return fmt.Errorf("indexing machine context: %w", indexErr)
		}
	}
	return nil
}

// IndexProjectFromDir indexes project context for the given project root.
func (r *Retriever) IndexProjectFromDir(root string) error {
	pc, err := contextgen.LoadProjectContext(root)
	if err != nil {
		return err
	}
	return r.IndexProjectContext(pc)
}

// ---- Helpers -----------------------------------------------------------------

// extractTags derives a set of search tags from a natural language request.
// Tags are used for efficient chunk retrieval from SQLite.
func extractTags(request string) []string {
	tagKeywords := map[string][]string{
		"log":      {"logs", "nginx", "apache", "syslog"},
		"logs":     {"logs", "nginx", "apache", "syslog"},
		"nginx":    {"nginx", "logs", "config"},
		"apache":   {"apache", "logs", "config"},
		"config":   {"config", "conf", "settings"},
		"conf":     {"config", "conf"},
		"docker":   {"docker", "infra"},
		"php":      {"php", "backend"},
		"go":       {"go", "backend"},
		"python":   {"python", "backend"},
		"node":     {"node", "backend", "frontend"},
		"frontend": {"frontend", "react", "vue"},
		"backend":  {"backend"},
		"search":   {"search"},
		"find":     {"search", "files"},
		"file":     {"files"},
		"files":    {"files"},
		"redis":    {"redis", "backend"},
		"timeout":  {"logs", "backend"},
		"error":    {"logs"},
		"500":      {"logs", "nginx"},
	}

	var tags []string
	seen := make(map[string]bool)
	words := strings.Fields(strings.ToLower(request))

	for _, word := range words {
		word = strings.Trim(word, `.,!?;:"'()[]`)
		if extra, ok := tagKeywords[word]; ok {
			for _, tag := range extra {
				if !seen[tag] {
					seen[tag] = true
					tags = append(tags, tag)
				}
			}
		}
	}

	return tags
}

func truncateSnippet(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
