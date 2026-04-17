// Package runner provides the secure local command execution engine.
// It uses exec.CommandContext for timeout enforcement, captures structured
// stdout/stderr, enforces output limits, and records execution metadata.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/slim/cmdctx/internal/policy"
	"github.com/slim/cmdctx/internal/utils"
)

// Options configures a single execution.
type Options struct {
	// Timeout is the maximum wall-clock time for the command.
	Timeout time.Duration
	// MaxOutputBytes caps the total combined stdout+stderr size before truncation.
	MaxOutputBytes int
	// WorkDir is the working directory. Defaults to os.Getwd().
	WorkDir string
	// Environ overrides the process environment. Defaults to os.Environ().
	Environ []string
}

// DefaultOptions returns safe execution defaults.
func DefaultOptions() Options {
	return Options{
		Timeout:        30 * time.Second,
		MaxOutputBytes: 512 * 1024,
	}
}

// Result holds the complete output of a command execution.
type Result struct {
	Argv       []string
	Stdout     string
	Stderr     string
	ExitCode   int
	Duration   time.Duration
	Truncated  bool
	TimedOut   bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// Run executes the given argv safely and returns a structured Result.
// It never uses a shell — argv[0] is always looked up in PATH directly.
// The caller is responsible for policy validation before calling Run.
func Run(ctx context.Context, argv []string, opts Options) (*Result, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxOutputBytes == 0 {
		opts.MaxOutputBytes = 512 * 1024
	}

	// Resolve the binary path explicitly so we know what we're running.
	binary, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("command not found: %q — install it or check your PATH", argv[0])
	}

	// Apply timeout.
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, binary, argv[1:]...)

	// Work directory.
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	} else {
		cmd.Dir, _ = os.Getwd()
	}

	// Environment: use caller's env or inherit.
	if opts.Environ != nil {
		cmd.Env = opts.Environ
	} else {
		cmd.Env = os.Environ()
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startedAt := time.Now()
	runErr := cmd.Run()
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	result := &Result{
		Argv:       argv,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   duration,
	}

	// Determine exit code.
	// Check context error FIRST — when the context deadline is exceeded,
	// exec.CommandContext kills the process, which also produces an ExitError.
	// Without this ordering, timeout cases would be misclassified as normal failures.
	if runErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
			result.Stderr = fmt.Sprintf("[timed out after %s]", opts.Timeout)
		} else if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Stderr = runErr.Error()
		}
	}

	// Capture and truncate output.
	rawStdout := stdoutBuf.Bytes()
	rawStderr := stderrBuf.Bytes()

	// Combine for total size check.
	totalSize := len(rawStdout) + len(rawStderr)
	if totalSize > opts.MaxOutputBytes {
		result.Truncated = true
		// Allocate proportionally between stdout and stderr.
		stdoutShare := opts.MaxOutputBytes * len(rawStdout) / totalSize
		stderrShare := opts.MaxOutputBytes - stdoutShare
		if stdoutShare < len(rawStdout) {
			rawStdout = rawStdout[:stdoutShare]
		}
		if stderrShare < len(rawStderr) {
			rawStderr = rawStderr[:stderrShare]
		}
	}

	result.Stdout = string(utils.RedactBytes(rawStdout))
	if result.Stderr == "" {
		result.Stderr = string(utils.RedactBytes(rawStderr))
	}

	if result.Truncated {
		result.Stdout += fmt.Sprintf("\n\n[stdout truncated: showing %d of %d bytes]",
			len(rawStdout), len(stdoutBuf.Bytes()))
	}

	return result, nil
}

// CanRun reports whether the binary in argv[0] is available in PATH.
func CanRun(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("binary %q not found in PATH", argv[0])
	}
	return nil
}

// PromptConfirm asks the user for confirmation in an interactive terminal.
// Returns true if the user confirms. If autoYes is true, returns true without prompting
// (only for safe/low-risk commands — callers must enforce risk checks).
func PromptConfirm(display string, risk policy.RiskLevel, autoYes bool) (bool, error) {
	if autoYes {
		if risk == policy.RiskBlocked || risk == policy.RiskHigh {
			return false, fmt.Errorf("--yes flag cannot be used with %s-risk commands", risk)
		}
		return true, nil
	}

	fmt.Printf("\n  Command: %s\n", display)
	fmt.Printf("  Risk:    %s\n\n", risk)
	fmt.Print("Execute this command? [y/N] ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		// EOF or non-interactive terminal — treat as no.
		return false, nil
	}

	return response == "y" || response == "Y" || response == "yes", nil
}
