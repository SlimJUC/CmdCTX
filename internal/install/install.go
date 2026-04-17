// Package install provides helpers for the binary install/uninstall lifecycle.
// It handles PATH detection, binary placement, and post-install messaging.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	BinaryName    = "cmdctx"
	DefaultBinDir = ".local/bin"
	GlobalBinDir  = "/usr/local/bin"
)

// DefaultInstallDir returns the default per-user install directory.
func DefaultInstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, DefaultBinDir)
}

// IsInPath reports whether the given directory is present in PATH.
func IsInPath(dir string) bool {
	pathVar := os.Getenv("PATH")
	parts := strings.Split(pathVar, ":")
	for _, p := range parts {
		// Expand ~ manually since os.Getenv doesn't expand it.
		if strings.HasPrefix(p, "~") {
			home, _ := os.UserHomeDir()
			p = filepath.Join(home, p[1:])
		}
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// Install copies the binary at srcPath to destDir/cmdctx.
// If destDir does not exist, it is created.
func Install(srcPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating install dir %q: %w", destDir, err)
	}

	destPath := filepath.Join(destDir, BinaryName)

	// Read source binary.
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading binary: %w", err)
	}

	// Write destination with execute permissions.
	if err := os.WriteFile(destPath, data, 0o755); err != nil {
		return fmt.Errorf("writing binary to %q: %w", destPath, err)
	}

	return nil
}

// Uninstall removes the binary from destDir. It does not remove app data
// unless removeData is true.
func Uninstall(destDir string, removeData bool) error {
	binPath := filepath.Join(destDir, BinaryName)
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing binary: %w", err)
	}

	if removeData {
		home, _ := os.UserHomeDir()
		dataDir := filepath.Join(home, ".cmdctx")
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("removing app data: %w", err)
		}
	}

	return nil
}

// PostInstallMessage returns the message to print after a successful install.
func PostInstallMessage(installDir string) string {
	var b strings.Builder

	b.WriteString("\n✓ cmdctx installed to: " + installDir + "\n\n")

	if !IsInPath(installDir) {
		b.WriteString("⚠  Warning: " + installDir + " is not in your PATH.\n")
		b.WriteString("   Add the following to your shell profile (~/.bashrc, ~/.zshrc, etc.):\n\n")
		b.WriteString("     export PATH=\"$HOME/.local/bin:$PATH\"\n\n")
		b.WriteString("   Then restart your shell or run:\n\n")
		b.WriteString("     source ~/.zshrc  # or ~/.bashrc\n\n")
	}

	b.WriteString("To get started:\n")
	b.WriteString("  cmdctx init          # generate machine context\n")
	b.WriteString("  cmdctx doctor        # check setup\n")
	b.WriteString("  cmdctx providers     # configure AI provider\n")
	b.WriteString("  cmdctx tui           # launch interactive TUI\n\n")

	return b.String()
}

// BinaryPath returns the path to the currently running binary.
func BinaryPath() (string, error) {
	return os.Executable()
}

// IsInstalled reports whether cmdctx is installed in the given directory.
func IsInstalled(dir string) bool {
	path := filepath.Join(dir, BinaryName)
	_, err := os.Stat(path)
	return err == nil
}
