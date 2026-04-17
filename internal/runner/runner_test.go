package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRun_EchoSuccess(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	result, err := Run(ctx, []string{"echo", "hello cmdctx"}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello cmdctx") {
		t.Errorf("expected stdout to contain 'hello cmdctx', got %q", result.Stdout)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestRun_NonZeroExitCode(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	// grep with no matches returns exit code 1.
	result, err := Run(ctx, []string{"grep", "nonexistent_xyz_pattern_123", "/dev/null"}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for no-match grep")
	}
}

func TestRun_Timeout(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()
	opts.Timeout = 100 * time.Millisecond

	// sleep longer than the timeout.
	result, err := Run(ctx, []string{"sleep", "10"}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Error("expected TimedOut to be true")
	}
	if result.ExitCode != -1 {
		t.Errorf("expected exit code -1 for timeout, got %d", result.ExitCode)
	}
}

func TestRun_OutputTruncation(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()
	opts.MaxOutputBytes = 10 // very small limit

	// Generate some output.
	result, err := Run(ctx, []string{"echo", "this is a fairly long output string that should be truncated"}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected output to be truncated")
	}
}

func TestRun_CommandNotFound(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	_, err := Run(ctx, []string{"nonexistent_binary_xyz_123"}, opts)
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
}

func TestRun_EmptyCommand(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	_, err := Run(ctx, []string{}, opts)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestCanRun_Available(t *testing.T) {
	if err := CanRun([]string{"grep"}); err != nil {
		t.Errorf("expected grep to be runnable: %v", err)
	}
}

func TestCanRun_NotAvailable(t *testing.T) {
	if err := CanRun([]string{"nonexistent_tool_xyz"}); err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestCanRun_EmptyCommand(t *testing.T) {
	if err := CanRun([]string{}); err == nil {
		t.Error("expected error for empty command")
	}
}

func TestRun_Stderr(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	// ls on a non-existent path writes to stderr.
	result, err := Run(ctx, []string{"ls", "/nonexistent_path_xyz_123456"}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stderr == "" {
		t.Error("expected stderr output for ls on nonexistent path")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestRun_WorkDir(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()
	opts.WorkDir = "/tmp"

	result, err := Run(ctx, []string{"pwd"}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "tmp") {
		t.Errorf("expected pwd output to contain 'tmp', got %q", result.Stdout)
	}
}
