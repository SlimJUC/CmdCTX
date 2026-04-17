// Package policy implements the command safety policy for cmdctx.
// Policy enforcement is done in code — not just described in prompts.
// All command generation goes through Validate before execution is considered.
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/slim/cmdctx/internal/config"
)

// RiskLevel classifies how risky a generated command is.
type RiskLevel string

const (
	RiskLow     RiskLevel = "low"
	RiskMedium  RiskLevel = "medium"
	RiskHigh    RiskLevel = "high"
	RiskBlocked RiskLevel = "blocked"
)

// Policy is the enforced command safety policy.
type Policy struct {
	PreferTools     map[string]string `json:"prefer_tools"`
	BlockedCommands []string          `json:"blocked_commands"`
	BlockedPatterns []string          `json:"blocked_patterns"`
	DefaultExcludes []string          `json:"default_excludes"`
}

// Default returns the built-in safety-first policy.
// This is always applied regardless of any user overrides.
func Default() *Policy {
	return &Policy{
		PreferTools: map[string]string{
			"text_search": "rg",
			"file_search": "fd",
			"json_search": "jq",
		},
		BlockedCommands: []string{
			"rm", "rmdir", "mv", "cp", "chmod", "chown", "chgrp",
			"sudo", "su", "doas",
			"dd", "mkfs", "mkswap", "fdisk", "parted",
			"truncate", "shred", "wipe",
			"systemctl", "service", "init", "telinit",
			"kill", "killall", "pkill", "reboot", "halt", "shutdown", "poweroff",
			"crontab", "at", "batch",
			"iptables", "ip6tables", "ufw", "firewalld",
			"useradd", "userdel", "usermod", "passwd", "groupadd",
			"mount", "umount",
			"insmod", "rmmod", "modprobe",
			"curl", "wget", "fetch", "nc", "ncat", "netcat",
			"pip", "pip3", "npm", "yarn", "apt", "apt-get", "dpkg", "yum", "dnf",
			"brew", "snap", "flatpak",
			"python", "python3", "ruby", "perl", "php", "node",
			"bash", "sh", "zsh", "fish", "dash",
			"eval", "exec",
		},
		BlockedPatterns: []string{
			// Shell write redirections
			">", ">>", "2>", "2>>", "&>",
			// Piping to a shell interpreter
			"| sh", "| bash", "| zsh", "| python", "| ruby", "| perl",
			// Curl/wget piped to shell
			"curl | ", "wget | ", "curl|", "wget|",
			// Command substitution tricks
			"`", "$(", "${",
			// Background persistence
			"nohup", "&", "disown",
			// Here-docs/strings used for execution
			"<<", "<<<",
		},
		DefaultExcludes: []string{
			".git", "node_modules", "vendor", "dist", "build", ".cache",
			".next", ".nuxt", "target", "__pycache__", ".tox", ".venv",
		},
	}
}

// PolicyPath returns the path to the policy file.
func PolicyPath() string {
	return config.GlobalDir() + "/command-policy.json"
}

// Load reads the policy from the global policy file, merging with the hardcoded
// Default so that the built-in blocked commands can never be removed by the user.
func Load() (*Policy, error) {
	base := Default()

	data, err := os.ReadFile(PolicyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var user Policy
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("parsing policy file: %w", err)
	}

	// Merge: user additions only extend the defaults, never shrink them.
	merged := base
	for k, v := range user.PreferTools {
		merged.PreferTools[k] = v
	}
	merged.BlockedCommands = uniqueMerge(base.BlockedCommands, user.BlockedCommands)
	merged.BlockedPatterns = uniqueMerge(base.BlockedPatterns, user.BlockedPatterns)
	merged.DefaultExcludes = uniqueMerge(base.DefaultExcludes, user.DefaultExcludes)

	return merged, nil
}

// Save writes the policy to the global policy file.
func Save(p *Policy) error {
	if err := os.MkdirAll(config.GlobalDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(PolicyPath(), data, 0o600)
}

// ValidationResult holds the outcome of a policy check.
type ValidationResult struct {
	Allowed  bool
	Risk     RiskLevel
	Reason   string
	Warnings []string
}

// Validate checks a command (represented as argv) against the policy.
// It never executes anything — it only analyses the arguments.
func (p *Policy) Validate(argv []string) ValidationResult {
	if len(argv) == 0 {
		return ValidationResult{Allowed: false, Risk: RiskBlocked, Reason: "empty command"}
	}

	binary := strings.ToLower(argv[0])
	// Strip any path prefix so /usr/bin/rm → rm
	if idx := strings.LastIndex(binary, "/"); idx >= 0 {
		binary = binary[idx+1:]
	}

	// Check blocked commands first.
	for _, blocked := range p.BlockedCommands {
		if binary == strings.ToLower(blocked) {
			return ValidationResult{
				Allowed: false,
				Risk:    RiskBlocked,
				Reason:  fmt.Sprintf("command '%s' is not allowed by policy", binary),
			}
		}
	}

	// Check for blocked patterns in the full joined command string.
	full := strings.Join(argv, " ")
	for _, pat := range p.BlockedPatterns {
		if strings.Contains(full, pat) {
			return ValidationResult{
				Allowed: false,
				Risk:    RiskBlocked,
				Reason:  fmt.Sprintf("pattern '%s' is not allowed by policy", pat),
			}
		}
	}

	// Assess risk level based on scope indicators.
	risk, warnings := assessRisk(argv)

	return ValidationResult{
		Allowed:  true,
		Risk:     risk,
		Warnings: warnings,
	}
}

// ValidateShellString validates a raw shell string by tokenising it first.
// This is a best-effort check; structured argv validation is always preferred.
func (p *Policy) ValidateShellString(cmd string) ValidationResult {
	tokens := strings.Fields(cmd)
	return p.Validate(tokens)
}

// assessRisk heuristically determines a risk level for an allowed command.
func assessRisk(argv []string) (RiskLevel, []string) {
	var warnings []string
	full := strings.Join(argv, " ")

	// High-risk indicators.
	highIndicators := []string{
		"/etc/", "/var/", "/sys/", "/proc/", "/boot/",
		"/root/", "/home/", "~", "--no-ignore", "--hidden",
	}
	for _, ind := range highIndicators {
		if strings.Contains(full, ind) {
			warnings = append(warnings, fmt.Sprintf("accesses potentially sensitive path (%s)", ind))
			return RiskHigh, warnings
		}
	}

	// Medium-risk: broad recursive scope without exclusions.
	if strings.Contains(full, "-r") || strings.Contains(full, "--recursive") ||
		strings.Contains(full, "-R") {
		// Without any exclude flags it's medium.
		if !strings.Contains(full, "--exclude") && !strings.Contains(full, "--glob") &&
			!strings.Contains(full, "--ignore") {
			warnings = append(warnings, "broad recursive scan without exclusions")
			return RiskMedium, warnings
		}
	}

	return RiskLow, warnings
}

func uniqueMerge(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	result := make([]string, 0, len(base)+len(extra))
	for _, s := range append(base, extra...) {
		k := strings.ToLower(s)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}
