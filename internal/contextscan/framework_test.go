package contextscan

import (
	"testing"
)

func TestDetectFrameworks_Go(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/go.mod"},
		Extensions:      ExtensionStats{".go": 42},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if !hasFramework(frameworks, "Go") {
		t.Error("expected Go to be detected")
	}
	// Should be high confidence since both file signal and extension signal match.
	for _, fw := range frameworks {
		if fw.Name == "Go" && fw.Confidence != "high" {
			t.Errorf("expected high confidence for Go, got %q", fw.Confidence)
		}
	}
}

func TestDetectFrameworks_NodeJS(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/package.json"},
		Extensions:      ExtensionStats{".js": 20, ".mjs": 2},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if !hasFramework(frameworks, "Node.js") {
		t.Error("expected Node.js to be detected")
	}
}

func TestDetectFrameworks_Docker(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/docker-compose.yml"},
		Extensions:      ExtensionStats{},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if !hasFramework(frameworks, "Docker") {
		t.Error("expected Docker to be detected")
	}
}

func TestDetectFrameworks_PHP(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/composer.json"},
		Extensions:      ExtensionStats{".php": 100},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if !hasFramework(frameworks, "PHP") {
		t.Error("expected PHP to be detected")
	}
}

func TestDetectFrameworks_Python(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/requirements.txt"},
		Extensions:      ExtensionStats{".py": 50},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if !hasFramework(frameworks, "Python") {
		t.Error("expected Python to be detected")
	}
}

func TestDetectFrameworks_Rust(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/cargo.toml"},
		Extensions:      ExtensionStats{".rs": 30},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if !hasFramework(frameworks, "Rust") {
		t.Error("expected Rust to be detected")
	}
}

func TestDetectFrameworks_NoDuplicates(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{"~/project/go.mod", "~/project/go.sum"},
		Extensions:      ExtensionStats{".go": 10},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	seen := make(map[string]int)
	for _, fw := range frameworks {
		seen[fw.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("framework %q detected %d times (expected once)", name, count)
		}
	}
}

func TestDetectFrameworks_EmptyProject(t *testing.T) {
	result := &ScanResult{
		ImportantFiles:  []string{},
		Extensions:      ExtensionStats{},
		ManifestContent: make(map[string]string),
	}

	frameworks := DetectFrameworks(result)
	if len(frameworks) != 0 {
		t.Errorf("expected no frameworks for empty project, got %d", len(frameworks))
	}
}

func hasFramework(frameworks []Framework, name string) bool {
	for _, fw := range frameworks {
		if fw.Name == name {
			return true
		}
	}
	return false
}
