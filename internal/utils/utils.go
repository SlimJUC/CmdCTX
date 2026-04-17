// Package utils provides shared, stateless helper functions used across cmdctx packages.
package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ToolAvailable returns true if the given binary can be found in PATH.
func ToolAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// AvailableTools returns a map of tool-name → path for a list of candidate tools.
func AvailableTools(candidates []string) map[string]string {
	result := make(map[string]string, len(candidates))
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			result[name] = p
		}
	}
	return result
}

// HomeDir returns the current user's home directory (panics only on catastrophic failure).
func HomeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/root"
	}
	return h
}

// AbsPath resolves path to an absolute path relative to base.
// If path is already absolute it is returned unchanged.
func AbsPath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// FileExists reports whether path exists and is a regular file.
func FileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// DirExists reports whether path exists and is a directory.
func DirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// TruncateOutput truncates a byte slice to maxBytes and appends a notice when truncated.
func TruncateOutput(data []byte, maxBytes int) ([]byte, bool) {
	if len(data) <= maxBytes {
		return data, false
	}
	notice := fmt.Sprintf("\n\n[output truncated: %d bytes shown of %d total]", maxBytes, len(data))
	return append(data[:maxBytes], []byte(notice)...), true
}

// JoinArgs joins a slice of strings into a displayable shell-like command string.
// It quotes arguments that contain spaces for readability (display only, not exec).
func JoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\n") {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}

// ---- Redaction ----------------------------------------------------------------

// sensitivePatterns is a compiled list of regex patterns to redact from strings.
// These cover common credential and secret patterns.
var sensitivePatterns = []*regexp.Regexp{
	// Generic API keys / tokens (long hex or base64 strings after key= or token=)
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|passwd|pwd|auth|bearer|private[_-]?key)\s*[=:]\s*\S+`),
	// AWS access key
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// Generic hex secret (>= 32 chars)
	regexp.MustCompile(`[0-9a-fA-F]{32,}`),
	// JWT pattern
	regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),
	// SSH private key header
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

// RedactString replaces sensitive-looking strings in s with [REDACTED].
func RedactString(s string) string {
	for _, p := range sensitivePatterns {
		s = p.ReplaceAllStringFunc(s, func(match string) string {
			// Keep the key name part for context (before = or :) if present.
			if idx := strings.IndexAny(match, "=:"); idx >= 0 {
				return match[:idx+1] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return s
}

// RedactBytes applies RedactString to a byte slice.
func RedactBytes(b []byte) []byte {
	return []byte(RedactString(string(b)))
}

// ---- Path helpers -------------------------------------------------------------

// IsSensitivePath returns true for paths that should never be scanned for content
// (keys, credentials, browser data, etc.).
var sensitiveDirs = []string{
	".ssh", ".gnupg", ".gpg", ".aws", ".azure", ".gcloud",
	"credentials", "secrets", "private",
}

var sensitiveFileNames = []string{
	".env", ".env.local", ".env.production", ".env.development",
	"id_rsa", "id_ecdsa", "id_ed25519", "id_dsa",
	".netrc", ".htpasswd",
	"credentials", "secrets.json", "service-account.json",
}

// IsSensitivePath returns true if the path segment name looks like a sensitive
// directory or file that must not be read.
func IsSensitivePath(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	for _, d := range sensitiveDirs {
		if lower == d {
			return true
		}
	}
	for _, f := range sensitiveFileNames {
		if lower == f {
			return true
		}
	}
	// Catch private key files regardless of name.
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") ||
		strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") {
		return true
	}
	return false
}

// ContainsAnyString is a case-insensitive substring check for any needle in s.
func ContainsAnyString(s string, needles []string) bool {
	lower := strings.ToLower(s)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// UniqueStrings returns a deduplicated slice preserving order.
func UniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// ShortenPath replaces the home directory prefix with ~ for display.
func ShortenPath(path string) string {
	home := HomeDir()
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
