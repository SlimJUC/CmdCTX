package utils

import (
	"testing"
)

func TestRedactString(t *testing.T) {
	cases := []struct {
		input    string
		wantSafe bool // true if [REDACTED] should appear, false if should be unchanged
		desc     string
	}{
		{"api_key=sk-abc123def456", true, "API key value redacted"},
		{"password=supersecret", true, "password value redacted"},
		{"normal text without secrets", false, "safe text unchanged"},
		{"-----BEGIN RSA PRIVATE KEY-----", true, "SSH private key header redacted"},
	}

	for _, tc := range cases {
		result := RedactString(tc.input)
		hasRedacted := containsSubstr(result, "[REDACTED]")
		if tc.wantSafe && !hasRedacted {
			t.Errorf("%s: expected [REDACTED] in %q but got %q", tc.desc, tc.input, result)
		}
		if !tc.wantSafe && hasRedacted {
			t.Errorf("%s: did not expect [REDACTED] in %q but got %q", tc.desc, tc.input, result)
		}
	}
}

func TestIsSensitivePath(t *testing.T) {
	cases := []struct {
		path      string
		sensitive bool
	}{
		{".env", true},
		{".env.local", true},
		{"id_rsa", true},
		{"id_ed25519", true},
		{"service-account.json", true},
		{"config.json", false},
		{"README.md", false},
		{"package.json", false},
		{"private.pem", true},
		{"cert.key", true},
	}

	for _, tc := range cases {
		got := IsSensitivePath(tc.path)
		if got != tc.sensitive {
			t.Errorf("IsSensitivePath(%q): expected %v, got %v", tc.path, tc.sensitive, got)
		}
	}
}

func TestTruncateOutput(t *testing.T) {
	data := make([]byte, 100)
	for i := range data {
		data[i] = 'A'
	}

	// No truncation needed.
	out, truncated := TruncateOutput(data, 200)
	if truncated {
		t.Error("expected no truncation for data within limit")
	}
	if len(out) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(out))
	}

	// Truncation needed.
	out, truncated = TruncateOutput(data, 50)
	if !truncated {
		t.Error("expected truncation")
	}
	if len(out) <= 50 {
		t.Error("expected notice to be appended")
	}
}

func TestJoinArgs(t *testing.T) {
	args := []string{"grep", "-r", "payment failed", "."}
	result := JoinArgs(args)
	expected := "grep -r 'payment failed' ."
	if result != expected {
		t.Errorf("JoinArgs: expected %q, got %q", expected, result)
	}

	// No spaces.
	args = []string{"rg", "pattern", "./src"}
	result = JoinArgs(args)
	expected = "rg pattern ./src"
	if result != expected {
		t.Errorf("JoinArgs: expected %q, got %q", expected, result)
	}
}

func TestUniqueStrings(t *testing.T) {
	in := []string{"a", "b", "a", "c", "b"}
	out := UniqueStrings(in)
	if len(out) != 3 {
		t.Errorf("expected 3 unique strings, got %d: %v", len(out), out)
	}
	// Check order preserved.
	if out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Errorf("order not preserved: %v", out)
	}
}

func TestShortenPath(t *testing.T) {
	home := HomeDir()
	path := home + "/go/cmdctx"
	short := ShortenPath(path)
	expected := "~/go/cmdctx"
	if short != expected {
		t.Errorf("ShortenPath: expected %q, got %q", expected, short)
	}

	// Absolute path outside home.
	result := ShortenPath("/var/log/nginx")
	if result != "/var/log/nginx" {
		t.Errorf("expected unchanged /var/log/nginx, got %q", result)
	}
}

func TestToolAvailable(t *testing.T) {
	// grep and find should always be available on Linux.
	if !ToolAvailable("grep") {
		t.Error("expected grep to be available")
	}
	if !ToolAvailable("find") {
		t.Error("expected find to be available")
	}
	if ToolAvailable("nonexistent_tool_xyz_123") {
		t.Error("expected nonexistent tool to be unavailable")
	}
}

func containsSubstr(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
