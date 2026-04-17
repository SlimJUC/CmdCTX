package policy

import (
	"testing"
)

func TestDefaultPolicy(t *testing.T) {
	p := Default()
	if len(p.BlockedCommands) == 0 {
		t.Error("expected blocked commands to be non-empty")
	}
	if len(p.BlockedPatterns) == 0 {
		t.Error("expected blocked patterns to be non-empty")
	}
}

func TestValidate_BlockedCommand(t *testing.T) {
	p := Default()

	cases := []struct {
		argv    []string
		allowed bool
		desc    string
	}{
		{[]string{"rm", "-rf", "/tmp/test"}, false, "rm is blocked"},
		{[]string{"sudo", "apt", "install"}, false, "sudo is blocked"},
		{[]string{"chmod", "777", "file"}, false, "chmod is blocked"},
		{[]string{"mv", "a", "b"}, false, "mv is blocked"},
		{[]string{"dd", "if=/dev/zero"}, false, "dd is blocked"},
		{[]string{"grep", "-r", "pattern", "."}, true, "grep is allowed"},
		{[]string{"find", ".", "-name", "*.go"}, true, "find is allowed"},
		{[]string{"rg", "pattern"}, true, "rg is allowed"},
	}

	for _, tc := range cases {
		result := p.Validate(tc.argv)
		if result.Allowed != tc.allowed {
			t.Errorf("%s: expected allowed=%v, got allowed=%v (reason: %s)",
				tc.desc, tc.allowed, result.Allowed, result.Reason)
		}
	}
}

func TestValidate_BlockedPatterns(t *testing.T) {
	p := Default()

	cases := []struct {
		argv    []string
		allowed bool
		desc    string
	}{
		{[]string{"grep", "pattern", ">", "out.txt"}, false, "redirect > is blocked"},
		{[]string{"grep", "pattern", ">>", "out.txt"}, false, "redirect >> is blocked"},
		{[]string{"grep", "$(cmd)", "file"}, false, "command substitution is blocked"},
	}

	for _, tc := range cases {
		result := p.Validate(tc.argv)
		if result.Allowed != tc.allowed {
			t.Errorf("%s: expected allowed=%v, got %v (reason: %s)",
				tc.desc, tc.allowed, result.Allowed, result.Reason)
		}
	}
}

func TestValidate_EmptyCommand(t *testing.T) {
	p := Default()
	result := p.Validate([]string{})
	if result.Allowed {
		t.Error("expected empty command to be blocked")
	}
	if result.Risk != RiskBlocked {
		t.Errorf("expected RiskBlocked, got %s", result.Risk)
	}
}

func TestValidate_RiskLevels(t *testing.T) {
	p := Default()

	// Low risk: specific scoped search with exclusions.
	result := p.Validate([]string{"grep", "-r", "--exclude-dir=vendor", "pattern", "."})
	if result.Risk != RiskLow {
		t.Errorf("expected RiskLow, got %s", result.Risk)
	}

	// High risk: system path access.
	result = p.Validate([]string{"grep", "-r", "pattern", "/etc/"})
	if result.Risk != RiskHigh {
		t.Errorf("expected RiskHigh for /etc/ access, got %s", result.Risk)
	}
}

func TestValidate_PathStripping(t *testing.T) {
	p := Default()

	// Full path to blocked binary should still be blocked.
	result := p.Validate([]string{"/usr/bin/rm", "-rf", "."})
	if result.Allowed {
		t.Error("expected /usr/bin/rm to be blocked (path stripping)")
	}
}

func TestUniqueMerge(t *testing.T) {
	base := []string{"rm", "sudo"}
	extra := []string{"sudo", "dd", "new"}
	result := uniqueMerge(base, extra)

	seen := make(map[string]int)
	for _, s := range result {
		seen[s]++
	}
	for k, count := range seen {
		if count > 1 {
			t.Errorf("duplicate entry %q in merged list", k)
		}
	}
	if len(result) != 4 {
		t.Errorf("expected 4 unique entries, got %d: %v", len(result), result)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// This test uses the temp policy path — skip if not writable.
	t.Skip("integration test requiring ~/.cmdctx — run manually")
}
