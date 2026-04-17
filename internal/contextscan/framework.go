package contextscan

import "strings"

// Framework represents a detected technology stack or framework in a project.
type Framework struct {
	Name       string   `json:"name"`
	Category   string   `json:"category"`   // frontend | backend | language | infra | tool
	Confidence string   `json:"confidence"` // high | medium | low
	Evidence   []string `json:"evidence"`
}

// frameworkRule defines detection logic for a single framework.
type frameworkRule struct {
	name     string
	category string
	// At least one indicator file/extension must be present.
	fileSignals []string
	extSignals  []string
	// Bonus signals that raise confidence.
	bonusSignals []string
}

var frameworkRules = []frameworkRule{
	// --- Go ---
	{
		name:        "Go",
		category:    "language",
		fileSignals: []string{"go.mod"},
		extSignals:  []string{".go"},
	},
	// --- Node.js ---
	{
		name:        "Node.js",
		category:    "backend",
		fileSignals: []string{"package.json"},
		extSignals:  []string{".js", ".mjs", ".cjs"},
	},
	// --- React ---
	{
		name:         "React",
		category:     "frontend",
		fileSignals:  []string{"package.json"},
		extSignals:   []string{".jsx", ".tsx"},
		bonusSignals: []string{"react"},
	},
	// --- Next.js ---
	{
		name:        "Next.js",
		category:    "frontend",
		fileSignals: []string{"next.config.js", "next.config.ts", "next.config.mjs"},
		extSignals:  []string{},
	},
	// --- TypeScript ---
	{
		name:        "TypeScript",
		category:    "language",
		fileSignals: []string{"tsconfig.json"},
		extSignals:  []string{".ts", ".tsx"},
	},
	// --- Laravel ---
	{
		name:        "Laravel",
		category:    "backend",
		fileSignals: []string{"artisan", "composer.json"},
		extSignals:  []string{".php"},
	},
	// --- CodeIgniter ---
	{
		name:         "CodeIgniter",
		category:     "backend",
		fileSignals:  []string{"composer.json", "spark"},
		extSignals:   []string{".php"},
		bonusSignals: []string{"codeigniter"},
	},
	// --- PHP (generic) ---
	{
		name:        "PHP",
		category:    "language",
		fileSignals: []string{"composer.json"},
		extSignals:  []string{".php"},
	},
	// --- Python ---
	{
		name:        "Python",
		category:    "language",
		fileSignals: []string{"requirements.txt", "pyproject.toml", "setup.py"},
		extSignals:  []string{".py"},
	},
	// --- Rust ---
	{
		name:        "Rust",
		category:    "language",
		fileSignals: []string{"cargo.toml"},
		extSignals:  []string{".rs"},
	},
	// --- Docker ---
	{
		name:        "Docker",
		category:    "infra",
		fileSignals: []string{"dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"},
		extSignals:  []string{},
	},
	// --- Kubernetes ---
	{
		name:        "Kubernetes",
		category:    "infra",
		fileSignals: []string{"kubernetes", "k8s"},
		extSignals:  []string{},
	},
	// --- Nginx / OpenResty ---
	{
		name:        "Nginx",
		category:    "infra",
		fileSignals: []string{"nginx.conf", "openresty.conf"},
		extSignals:  []string{},
	},
	// --- Ruby/Rails ---
	{
		name:        "Ruby",
		category:    "language",
		fileSignals: []string{"gemfile"},
		extSignals:  []string{".rb"},
	},
	// --- Vue.js ---
	{
		name:        "Vue.js",
		category:    "frontend",
		fileSignals: []string{"vue.config.js", "vite.config.js"},
		extSignals:  []string{".vue"},
	},
	// --- Svelte ---
	{
		name:        "Svelte",
		category:    "frontend",
		fileSignals: []string{"svelte.config.js"},
		extSignals:  []string{".svelte"},
	},
}

// DetectFrameworks analyses a ScanResult and returns detected frameworks.
// It only uses path metadata and extension counts — no file content in safe mode.
func DetectFrameworks(result *ScanResult) []Framework {
	// Build lookup structures for fast matching.
	importantLower := make(map[string]bool, len(result.ImportantFiles))
	for _, f := range result.ImportantFiles {
		base := strings.ToLower(lastPathSegment(f))
		importantLower[base] = true
	}

	var detected []Framework
	seen := make(map[string]bool)

	for _, rule := range frameworkRules {
		if seen[rule.name] {
			continue
		}

		var evidence []string
		matched := false

		// Check file signals.
		for _, sig := range rule.fileSignals {
			if importantLower[strings.ToLower(sig)] {
				evidence = append(evidence, "file: "+sig)
				matched = true
			}
		}

		// Check extension signals.
		for _, ext := range rule.extSignals {
			if count, ok := result.Extensions[ext]; ok && count > 0 {
				evidence = append(evidence, "ext: "+ext)
				matched = true
			}
		}

		if !matched {
			continue
		}

		confidence := "medium"

		// Upgrade confidence with bonus signals.
		bonusCount := 0
		for _, bonus := range rule.bonusSignals {
			// Check if bonus appears in manifest content (deep mode only).
			for _, content := range result.ManifestContent {
				if strings.Contains(strings.ToLower(content), bonus) {
					evidence = append(evidence, "manifest: "+bonus)
					bonusCount++
				}
			}
		}

		// High confidence: both file signals AND ext signals matched.
		fileMatch := false
		extMatch := false
		for _, e := range evidence {
			if strings.HasPrefix(e, "file:") {
				fileMatch = true
			}
			if strings.HasPrefix(e, "ext:") {
				extMatch = true
			}
		}
		if fileMatch && extMatch {
			confidence = "high"
		} else if bonusCount > 0 {
			confidence = "high"
		} else if len(evidence) == 1 {
			confidence = "low"
		}

		detected = append(detected, Framework{
			Name:       rule.name,
			Category:   rule.category,
			Confidence: confidence,
			Evidence:   evidence,
		})
		seen[rule.name] = true
	}

	return detected
}

func lastPathSegment(path string) string {
	// Handle both / and ~ paths.
	path = strings.TrimRight(path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
