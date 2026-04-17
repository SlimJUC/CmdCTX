package cli

import (
	"github.com/slim/cmdctx/internal/tui"
)

// launchTUI starts the Bubble Tea full-screen TUI application.
// It is defined here (in the cli package) to avoid import cycles between
// the cli and tui packages.
func launchTUI() error {
	return tui.Run()
}
