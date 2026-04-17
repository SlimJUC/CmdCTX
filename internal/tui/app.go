package tui

import (
	"os"

	"github.com/slim/cmdctx/internal/contextgen"
)

// loadMachineCtx is a thin wrapper around contextgen.LoadMachineContext
// that the TUI model can call inside a tea.Cmd closure.
func loadMachineCtx() (*contextgen.MachineContext, error) {
	return contextgen.LoadMachineContext()
}

// getCurrentProjectRoot attempts to find a project root by looking for
// a .cmdctx directory, go.mod, package.json, or composer.json in the cwd
// and its parents. Returns the cwd if no project root is found.
func getCurrentProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Walk upward looking for project markers.
	dir := cwd
	for {
		markers := []string{".cmdctx", "go.mod", "package.json", "composer.json", "Cargo.toml", ".git"}
		for _, m := range markers {
			if _, err := os.Stat(dir + "/" + m); err == nil {
				return dir
			}
		}
		parent := parentDir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return cwd
}

func parentDir(dir string) string {
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' {
			if i == 0 {
				return "/"
			}
			return dir[:i]
		}
	}
	return dir
}
