// Package contextscan provides the local file-system scanner that collects
// structured metadata about a machine or project — without reading sensitive file content.
//
// Safe mode: metadata only (paths, names, extensions, manifest filenames).
// Deep mode: inspects a curated allow-list of non-secret files (README, go.mod, package.json…).
package contextscan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/slim/cmdctx/internal/utils"
)

// ScanMode controls how aggressively the scanner reads file contents.
type ScanMode string

const (
	ScanModeSafe ScanMode = "safe" // metadata + filenames only
	ScanModeDeep ScanMode = "deep" // also reads safe manifest content
)

// Options configures a scan operation.
type Options struct {
	Mode           ScanMode
	ExcludeDirs    []string
	MaxDepth       int
	MaxFiles       int
	FollowSymlinks bool
	AllowedRoots   []string // explicit root dirs to scan; nil = use Root
}

// DefaultOptions returns safe, conservative scan defaults.
func DefaultOptions() Options {
	return Options{
		Mode: ScanModeSafe,
		ExcludeDirs: []string{
			".git", "node_modules", "vendor", "dist", "build", ".cache",
			".next", ".nuxt", "target", "__pycache__", ".tox", ".venv",
			"venv", ".idea", ".vscode", "coverage", ".nyc_output",
		},
		MaxDepth:       8,
		MaxFiles:       50_000,
		FollowSymlinks: false,
	}
}

// ExtensionStats tracks how many files of each extension were found.
type ExtensionStats map[string]int

// ScanResult is the complete output of a single scan operation.
type ScanResult struct {
	Root            string            `json:"root"`
	ScannedAt       time.Time         `json:"scanned_at"`
	Mode            ScanMode          `json:"mode"`
	TotalFiles      int               `json:"total_files"`
	TotalDirs       int               `json:"total_dirs"`
	Extensions      ExtensionStats    `json:"extensions"`
	Frameworks      []Framework       `json:"frameworks"`
	ImportantFiles  []string          `json:"important_files"`
	LogDirs         []string          `json:"log_dirs"`
	ConfigDirs      []string          `json:"config_dirs"`
	IgnoredDirs     []string          `json:"ignored_dirs"`
	ManifestContent map[string]string `json:"manifest_content,omitempty"` // deep mode only
	ToolsAvailable  map[string]string `json:"tools_available"`
	Warnings        []string          `json:"warnings"`
}

// safeManifests is the exclusive allow-list of files whose content may be read in deep mode.
// This list must never include any file that could contain secrets.
var safeManifests = []string{
	"README.md", "readme.md", "README.txt",
	"package.json",
	"composer.json",
	"go.mod",
	"Cargo.toml",
	"pyproject.toml", "setup.py", "setup.cfg",
	"docker-compose.yml", "docker-compose.yaml",
	"compose.yml", "compose.yaml",
	"Makefile",
	".env.example", ".env.sample",
	"nginx.conf", "openresty.conf",
	"Dockerfile",
	"requirements.txt",
}

// safeManiestSet for O(1) lookup.
var safeManifestSet map[string]bool

func init() {
	safeManifestSet = make(map[string]bool, len(safeManifests))
	for _, m := range safeManifests {
		safeManifestSet[strings.ToLower(m)] = true
	}
}

// Scanner performs a local file-system scan.
type Scanner struct {
	opts Options
}

// New creates a new Scanner with the given options.
func New(opts Options) *Scanner {
	if opts.MaxDepth == 0 {
		opts.MaxDepth = 8
	}
	if opts.MaxFiles == 0 {
		opts.MaxFiles = 50_000
	}
	excludeSet := make(map[string]bool, len(opts.ExcludeDirs))
	for _, d := range opts.ExcludeDirs {
		excludeSet[d] = true
	}
	return &Scanner{opts: opts}
}

// ScanProject scans the given directory as a project root.
func (s *Scanner) ScanProject(root string) (*ScanResult, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return s.scan(abs)
}

// ScanMachine scans the user's home directory for machine-level context.
func (s *Scanner) ScanMachine() (*ScanResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return s.scan(home)
}

func (s *Scanner) scan(root string) (*ScanResult, error) {
	result := &ScanResult{
		Root:            root,
		ScannedAt:       time.Now().UTC(),
		Mode:            s.opts.Mode,
		Extensions:      make(ExtensionStats),
		ManifestContent: make(map[string]string),
		ToolsAvailable:  utils.AvailableTools(toolCandidates),
	}

	excludeSet := make(map[string]bool, len(s.opts.ExcludeDirs))
	for _, d := range s.opts.ExcludeDirs {
		excludeSet[strings.ToLower(d)] = true
	}

	depth := 0
	_ = depth

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission denied or similar — skip but note it.
			result.Warnings = append(result.Warnings, "skipped: "+utils.ShortenPath(path))
			return filepath.SkipDir
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		// Compute depth.
		parts := strings.Split(rel, string(filepath.Separator))
		currentDepth := len(parts)
		if currentDepth > s.opts.MaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		lname := strings.ToLower(name)

		if d.IsDir() {
			// Skip excluded and sensitive directories.
			if excludeSet[lname] || utils.IsSensitivePath(name) {
				result.IgnoredDirs = append(result.IgnoredDirs, utils.ShortenPath(path))
				return filepath.SkipDir
			}
			result.TotalDirs++

			// Heuristic: directories named "log", "logs" are log dirs.
			if lname == "log" || lname == "logs" || lname == "var_log" {
				result.LogDirs = append(result.LogDirs, utils.ShortenPath(path))
			}
			// Heuristic: directories named "config", "conf", "etc", "settings" are config dirs.
			if lname == "config" || lname == "conf" || lname == "etc" ||
				lname == "settings" || lname == "configuration" {
				result.ConfigDirs = append(result.ConfigDirs, utils.ShortenPath(path))
			}
			return nil
		}

		// Regular file handling.
		if utils.IsSensitivePath(path) {
			return nil // skip sensitive files silently
		}

		result.TotalFiles++
		if result.TotalFiles > s.opts.MaxFiles {
			result.Warnings = append(result.Warnings, "file limit reached, scan truncated")
			return filepath.SkipAll
		}

		// Extension stats.
		ext := strings.ToLower(filepath.Ext(name))
		if ext != "" {
			result.Extensions[ext]++
		}

		// Track important files.
		if isImportantFile(lname) {
			result.ImportantFiles = append(result.ImportantFiles, utils.ShortenPath(path))
		}

		// Deep mode: read safe manifest content.
		if s.opts.Mode == ScanModeDeep && safeManifestSet[lname] {
			content, readErr := safeReadFile(path)
			if readErr == nil && content != "" {
				result.ManifestContent[utils.ShortenPath(path)] = content
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Run framework detection over collected data.
	result.Frameworks = DetectFrameworks(result)

	// Deduplicate and sort slices for deterministic output.
	result.ImportantFiles = sortUnique(result.ImportantFiles)
	result.LogDirs = sortUnique(result.LogDirs)
	result.ConfigDirs = sortUnique(result.ConfigDirs)
	result.IgnoredDirs = sortUnique(result.IgnoredDirs)

	return result, nil
}

// safeReadFile reads a file and applies redaction.
// It enforces a maximum read size to avoid reading huge files.
func safeReadFile(path string) (string, error) {
	const maxReadBytes = 8 * 1024 // 8 KiB per manifest

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, maxReadBytes)
	n, _ := f.Read(buf)
	if n == 0 {
		return "", nil
	}

	content := string(buf[:n])
	// Redact any accidental secrets.
	content = utils.RedactString(content)
	return content, nil
}

// isImportantFile returns true for files that are structurally significant.
func isImportantFile(name string) bool {
	important := []string{
		"go.mod", "go.sum",
		"package.json", "package-lock.json", "yarn.lock",
		"composer.json", "composer.lock",
		"cargo.toml", "cargo.lock",
		"makefile", "dockerfile",
		"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml",
		"nginx.conf", "openresty.conf",
		".env.example", ".env.sample",
		"readme.md", "readme.txt",
		"requirements.txt", "pipfile", "pipfile.lock",
		"gemfile", "gemfile.lock",
		"pyproject.toml",
	}
	lower := strings.ToLower(name)
	for _, imp := range important {
		if lower == imp {
			return true
		}
	}
	return false
}

var toolCandidates = []string{
	"rg", "fd", "grep", "find", "awk", "sed", "jq",
	"git", "docker", "docker-compose", "kubectl",
	"make", "cmake",
	"nginx", "openresty",
	"systemctl", "journalctl",
	"lsof", "ss", "netstat",
	"top", "htop", "ps",
	"curl", "wget",
	"node", "npm", "yarn", "python3", "python", "php", "go", "cargo", "rustc",
}

func sortUnique(in []string) []string {
	out := utils.UniqueStrings(in)
	sort.Strings(out)
	return out
}
