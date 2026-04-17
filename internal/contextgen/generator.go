// Package contextgen generates machine and project context files from scanner output.
// It produces both a human-readable Markdown file and a machine-parseable JSON file.
// The JSON is the authoritative data source; Markdown is for human review.
package contextgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/slim/cmdctx/internal/config"
	"github.com/slim/cmdctx/internal/contextscan"
	"github.com/slim/cmdctx/internal/utils"
)

// MachineContext is the structured machine-level context summary.
type MachineContext struct {
	GeneratedAt    time.Time               `json:"generated_at"`
	Hostname       string                  `json:"hostname"`
	OS             string                  `json:"os"`
	Arch           string                  `json:"arch"`
	HomeDir        string                  `json:"home_dir"`
	ToolsAvailable map[string]string       `json:"tools_available"`
	LogDirs        []string                `json:"log_dirs"`
	ConfigDirs     []string                `json:"config_dirs"`
	SearchRoots    []string                `json:"search_roots"`
	DefaultExclude []string                `json:"default_excludes"`
	Frameworks     []contextscan.Framework `json:"detected_stacks"`
	ScanMode       string                  `json:"scan_mode"`
	ScanStats      ScanStats               `json:"scan_stats"`
}

// ProjectContext is the structured project-level context summary.
type ProjectContext struct {
	GeneratedAt    time.Time               `json:"generated_at"`
	ProjectRoot    string                  `json:"project_root"`
	Frameworks     []contextscan.Framework `json:"frameworks"`
	LogDirs        []string                `json:"log_dirs"`
	ConfigDirs     []string                `json:"config_dirs"`
	ImportantFiles []string                `json:"important_files"`
	Extensions     map[string]int          `json:"extensions"`
	IgnoredDirs    []string                `json:"ignored_dirs"`
	DefaultExclude []string                `json:"default_excludes"`
	ScanMode       string                  `json:"scan_mode"`
	ScanStats      ScanStats               `json:"scan_stats"`
	Notes          []string                `json:"notes"`
}

// ScanStats summarises what the scanner processed.
type ScanStats struct {
	TotalFiles int      `json:"total_files"`
	TotalDirs  int      `json:"total_dirs"`
	Warnings   []string `json:"warnings,omitempty"`
}

// GenerateMachineContext produces machine context files in ~/.cmdctx/.
func GenerateMachineContext(scan *contextscan.ScanResult) (*MachineContext, error) {
	hostname, _ := os.Hostname()

	ctx := &MachineContext{
		GeneratedAt:    time.Now().UTC(),
		Hostname:       hostname,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		HomeDir:        utils.ShortenPath(scan.Root),
		ToolsAvailable: scan.ToolsAvailable,
		LogDirs:        enrichLogDirs(scan.LogDirs),
		ConfigDirs:     enrichConfigDirs(scan.ConfigDirs),
		SearchRoots:    defaultSearchRoots(scan.Root),
		DefaultExclude: config.DefaultConfig().DefaultExcludes,
		Frameworks:     scan.Frameworks,
		ScanMode:       string(scan.Mode),
		ScanStats: ScanStats{
			TotalFiles: scan.TotalFiles,
			TotalDirs:  scan.TotalDirs,
			Warnings:   scan.Warnings,
		},
	}

	dir := config.GlobalDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating context dir: %w", err)
	}

	// Write JSON.
	jsonPath := filepath.Join(dir, "machine-context.json")
	if err := writeJSON(jsonPath, ctx); err != nil {
		return nil, err
	}

	// Write Markdown.
	mdPath := filepath.Join(dir, "machine-context.md")
	if err := os.WriteFile(mdPath, []byte(renderMachineMarkdown(ctx)), 0o600); err != nil {
		return nil, fmt.Errorf("writing machine context markdown: %w", err)
	}

	return ctx, nil
}

// GenerateProjectContext produces project context files in <project>/.cmdctx/.
func GenerateProjectContext(root string, scan *contextscan.ScanResult) (*ProjectContext, error) {
	notes := buildProjectNotes(scan)

	ctx := &ProjectContext{
		GeneratedAt:    time.Now().UTC(),
		ProjectRoot:    utils.ShortenPath(root),
		Frameworks:     scan.Frameworks,
		LogDirs:        scan.LogDirs,
		ConfigDirs:     scan.ConfigDirs,
		ImportantFiles: scan.ImportantFiles,
		Extensions:     map[string]int(scan.Extensions),
		IgnoredDirs:    scan.IgnoredDirs,
		DefaultExclude: config.DefaultConfig().DefaultExcludes,
		ScanMode:       string(scan.Mode),
		ScanStats: ScanStats{
			TotalFiles: scan.TotalFiles,
			TotalDirs:  scan.TotalDirs,
			Warnings:   scan.Warnings,
		},
		Notes: notes,
	}

	dir := config.ProjectDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating project context dir: %w", err)
	}

	jsonPath := filepath.Join(dir, "project-context.json")
	if err := writeJSON(jsonPath, ctx); err != nil {
		return nil, err
	}

	mdPath := filepath.Join(dir, "project-context.md")
	if err := os.WriteFile(mdPath, []byte(renderProjectMarkdown(ctx)), 0o600); err != nil {
		return nil, fmt.Errorf("writing project context markdown: %w", err)
	}

	return ctx, nil
}

// LoadMachineContext reads the machine context JSON from disk.
func LoadMachineContext() (*MachineContext, error) {
	path := filepath.Join(config.GlobalDir(), "machine-context.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ctx MachineContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, fmt.Errorf("parsing machine context: %w", err)
	}
	return &ctx, nil
}

// LoadProjectContext reads the project context JSON for the given project root.
func LoadProjectContext(root string) (*ProjectContext, error) {
	path := filepath.Join(config.ProjectDir(root), "project-context.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ctx ProjectContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, fmt.Errorf("parsing project context: %w", err)
	}
	return &ctx, nil
}

// ---- Private helpers ---------------------------------------------------------

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling context JSON: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// defaultSearchRoots returns likely useful search starting points.
func defaultSearchRoots(home string) []string {
	candidates := []string{
		filepath.Join(home, "projects"),
		filepath.Join(home, "code"),
		filepath.Join(home, "src"),
		filepath.Join(home, "workspace"),
		filepath.Join(home, "work"),
		"/var/www",
		"/srv",
		"/opt",
	}
	var roots []string
	for _, c := range candidates {
		if utils.DirExists(c) {
			roots = append(roots, utils.ShortenPath(c))
		}
	}
	if len(roots) == 0 {
		roots = []string{utils.ShortenPath(home)}
	}
	return roots
}

// enrichLogDirs adds well-known system log directories if they exist.
func enrichLogDirs(found []string) []string {
	known := []string{"/var/log", "/var/log/nginx", "/var/log/apache2", "/tmp"}
	for _, k := range known {
		if utils.DirExists(k) {
			found = append(found, k)
		}
	}
	return utils.UniqueStrings(found)
}

// enrichConfigDirs adds well-known system config directories if they exist.
func enrichConfigDirs(found []string) []string {
	known := []string{"/etc", "/etc/nginx", "/etc/apache2"}
	for _, k := range known {
		if utils.DirExists(k) {
			found = append(found, k)
		}
	}
	return utils.UniqueStrings(found)
}

// buildProjectNotes generates human-readable notes about the project.
func buildProjectNotes(scan *contextscan.ScanResult) []string {
	var notes []string

	// Framework notes.
	for _, fw := range scan.Frameworks {
		notes = append(notes, fmt.Sprintf("Detected %s (%s, confidence: %s)", fw.Name, fw.Category, fw.Confidence))
	}

	// Extension summary.
	type extCount struct {
		ext   string
		count int
	}
	var counts []extCount
	for ext, n := range scan.Extensions {
		counts = append(counts, extCount{ext, n})
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].count > counts[j].count })
	if len(counts) > 0 {
		topExts := make([]string, 0, 5)
		for i, ec := range counts {
			if i >= 5 {
				break
			}
			topExts = append(topExts, fmt.Sprintf("%s(%d)", ec.ext, ec.count))
		}
		notes = append(notes, "Top file types: "+strings.Join(topExts, ", "))
	}

	return notes
}

// ---- Markdown rendering ------------------------------------------------------

func renderMachineMarkdown(ctx *MachineContext) string {
	var b strings.Builder

	b.WriteString("# Machine Context\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", ctx.GeneratedAt.Format(time.RFC3339)))
	b.WriteString("## System\n\n")
	b.WriteString(fmt.Sprintf("- **OS**: %s/%s\n", ctx.OS, ctx.Arch))
	b.WriteString(fmt.Sprintf("- **Hostname**: %s\n", ctx.Hostname))
	b.WriteString(fmt.Sprintf("- **Home**: %s\n\n", ctx.HomeDir))

	b.WriteString("## Available Tools\n\n")
	tools := make([]string, 0, len(ctx.ToolsAvailable))
	for t := range ctx.ToolsAvailable {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	for _, t := range tools {
		b.WriteString(fmt.Sprintf("- `%s`: %s\n", t, ctx.ToolsAvailable[t]))
	}
	b.WriteString("\n")

	if len(ctx.LogDirs) > 0 {
		b.WriteString("## Log Directories\n\n")
		for _, d := range ctx.LogDirs {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	if len(ctx.ConfigDirs) > 0 {
		b.WriteString("## Config Directories\n\n")
		for _, d := range ctx.ConfigDirs {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	if len(ctx.SearchRoots) > 0 {
		b.WriteString("## Search Roots\n\n")
		for _, r := range ctx.SearchRoots {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
		b.WriteString("\n")
	}

	if len(ctx.Frameworks) > 0 {
		b.WriteString("## Detected Stacks\n\n")
		for _, fw := range ctx.Frameworks {
			b.WriteString(fmt.Sprintf("- **%s** (%s) — confidence: %s\n", fw.Name, fw.Category, fw.Confidence))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Default Excludes\n\n")
	for _, e := range ctx.DefaultExclude {
		b.WriteString(fmt.Sprintf("- `%s`\n", e))
	}
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("## Scan Stats\n\n- Files: %d\n- Dirs: %d\n", ctx.ScanStats.TotalFiles, ctx.ScanStats.TotalDirs))

	return b.String()
}

func renderProjectMarkdown(ctx *ProjectContext) string {
	var b strings.Builder

	b.WriteString("# Project Context\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", ctx.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Project Root**: `%s`\n\n", ctx.ProjectRoot))

	if len(ctx.Frameworks) > 0 {
		b.WriteString("## Detected Frameworks\n\n")
		for _, fw := range ctx.Frameworks {
			b.WriteString(fmt.Sprintf("- **%s** (%s) — confidence: %s\n", fw.Name, fw.Category, fw.Confidence))
		}
		b.WriteString("\n")
	}

	if len(ctx.ImportantFiles) > 0 {
		b.WriteString("## Important Files\n\n")
		for _, f := range ctx.ImportantFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	if len(ctx.LogDirs) > 0 {
		b.WriteString("## Log Directories\n\n")
		for _, d := range ctx.LogDirs {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	if len(ctx.ConfigDirs) > 0 {
		b.WriteString("## Config Directories\n\n")
		for _, d := range ctx.ConfigDirs {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	if len(ctx.Notes) > 0 {
		b.WriteString("## Notes\n\n")
		for _, n := range ctx.Notes {
			b.WriteString(fmt.Sprintf("- %s\n", n))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Ignored Directories\n\n")
	for _, d := range ctx.IgnoredDirs {
		b.WriteString(fmt.Sprintf("- %s\n", d))
	}

	return b.String()
}
