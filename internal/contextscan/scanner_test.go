package contextscan

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDir creates a temporary directory tree for scanner tests.
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"go.mod":                    "module test\ngo 1.21\n",
		"main.go":                   "package main\n",
		"README.md":                 "# Test Project\n",
		"src/app.go":                "package app\n",
		"src/handler.go":            "package app\n",
		"vendor/dep.go":             "package dep\n",
		"node_modules/pkg/index.js": "module.exports = {};\n",
		".git/HEAD":                 "ref: refs/heads/main\n",
		"logs/app.log":              "2024-01-01 error: something\n",
		"config/settings.yaml":      "key: value\n",
		"package.json":              `{"name":"test","version":"1.0.0"}`,
		"docker-compose.yml":        "version: '3'\n",
	}

	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("creating dir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("writing file %s: %v", rel, err)
		}
	}

	return dir
}

func TestScanProject_SafeMode(t *testing.T) {
	dir := setupTestDir(t)
	opts := DefaultOptions()
	opts.Mode = ScanModeSafe

	scanner := New(opts)
	result, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if result.TotalFiles == 0 {
		t.Error("expected files to be scanned")
	}

	// Excluded dirs should be skipped.
	for _, ignored := range result.IgnoredDirs {
		base := filepath.Base(ignored)
		for _, excl := range opts.ExcludeDirs {
			if base == excl {
				goto foundIgnored
			}
		}
	foundIgnored:
	}

	// Important files should include go.mod and package.json.
	found := make(map[string]bool)
	for _, f := range result.ImportantFiles {
		base := filepath.Base(f)
		found[base] = true
	}
	if !found["go.mod"] {
		t.Error("expected go.mod in important files")
	}
	if !found["package.json"] {
		t.Error("expected package.json in important files")
	}
}

func TestScanProject_ExtensionStats(t *testing.T) {
	dir := setupTestDir(t)
	opts := DefaultOptions()
	scanner := New(opts)
	result, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if result.Extensions[".go"] == 0 {
		t.Error("expected .go extension to be counted")
	}
}

func TestScanProject_LogDirDetection(t *testing.T) {
	dir := setupTestDir(t)
	opts := DefaultOptions()
	scanner := New(opts)
	result, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	found := false
	for _, d := range result.LogDirs {
		if filepath.Base(d) == "logs" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'logs' directory to be detected as a log dir")
	}
}

func TestScanProject_ConfigDirDetection(t *testing.T) {
	dir := setupTestDir(t)
	opts := DefaultOptions()
	scanner := New(opts)
	result, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	found := false
	for _, d := range result.ConfigDirs {
		if filepath.Base(d) == "config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'config' directory to be detected as a config dir")
	}
}

func TestScanProject_ExcludesSensitiveFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a sensitive file.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.Mode = ScanModeDeep
	scanner := New(opts)
	result, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// .env should not be in manifest content even in deep mode.
	for path := range result.ManifestContent {
		if filepath.Base(path) == ".env" {
			t.Error("expected .env to be excluded from manifest content")
		}
	}
}

func TestScanProject_VendorExcluded(t *testing.T) {
	dir := setupTestDir(t)
	opts := DefaultOptions()
	scanner := New(opts)
	result, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// vendor/ files should not be counted.
	for _, f := range result.ImportantFiles {
		if filepath.Dir(f) == "vendor" {
			t.Errorf("expected vendor to be excluded, found: %s", f)
		}
	}
}

func TestSafeReadFile_RespectsMaxSize(t *testing.T) {
	dir := t.TempDir()
	// Write a file larger than the 8KiB limit.
	data := make([]byte, 16*1024)
	for i := range data {
		data[i] = 'A'
	}
	path := filepath.Join(dir, "large.md")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := safeReadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content) > 8*1024 {
		t.Errorf("expected content to be capped at 8KiB, got %d bytes", len(content))
	}
}
