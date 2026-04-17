// Package theme defines the visual style for the cmdctx TUI.
// All colors and styles are defined here for easy global customization.
package theme

import "github.com/charmbracelet/lipgloss"

// Palette defines the colour palette.
var (
	ColorPrimary   = lipgloss.AdaptiveColor{Light: "#1D3461", Dark: "#89B4FA"}
	ColorSecondary = lipgloss.AdaptiveColor{Light: "#4A4E69", Dark: "#CBA6F7"}
	ColorAccent    = lipgloss.AdaptiveColor{Light: "#00B4D8", Dark: "#89DCEB"}
	ColorSuccess   = lipgloss.AdaptiveColor{Light: "#2D6A4F", Dark: "#A6E3A1"}
	ColorWarning   = lipgloss.AdaptiveColor{Light: "#B7791F", Dark: "#F9E2AF"}
	ColorError     = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#F38BA8"}
	ColorMuted     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#585B70"}
	ColorBg        = lipgloss.AdaptiveColor{Light: "#F9FAFB", Dark: "#1E1E2E"}
	ColorSurface   = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#313244"}
	ColorBorder    = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#45475A"}
	ColorText      = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#CDD6F4"}
	ColorSubtext   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A6ADC8"}
)

// Styles holds all pre-built Lip Gloss styles.
type Styles struct {
	// App chrome.
	Header   lipgloss.Style
	Footer   lipgloss.Style
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	// Panels.
	Panel       lipgloss.Style
	ActivePanel lipgloss.Style
	// Text.
	Text  lipgloss.Style
	Muted lipgloss.Style
	Bold  lipgloss.Style
	Code  lipgloss.Style
	// Status indicators.
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style
	// Risk badges.
	RiskLow     lipgloss.Style
	RiskMedium  lipgloss.Style
	RiskHigh    lipgloss.Style
	RiskBlocked lipgloss.Style
	// Input.
	Input        lipgloss.Style
	InputFocused lipgloss.Style
	// List items.
	ListItem     lipgloss.Style
	ListSelected lipgloss.Style
	// Help bar.
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style
	HelpSep  lipgloss.Style
}

// Default returns the default (dark-mode-optimised) style set.
func Default() *Styles {
	s := &Styles{}

	s.Header = lipgloss.NewStyle().
		Background(ColorPrimary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Bold(true)

	s.Footer = lipgloss.NewStyle().
		Background(ColorBorder).
		Foreground(ColorSubtext).
		Padding(0, 1)

	s.Title = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		MarginBottom(1)

	s.Subtitle = lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true)

	s.Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2)

	s.ActivePanel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2)

	s.Text = lipgloss.NewStyle().Foreground(ColorText)
	s.Muted = lipgloss.NewStyle().Foreground(ColorMuted)
	s.Bold = lipgloss.NewStyle().Bold(true)
	s.Code = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Background(ColorSurface).
		Padding(0, 1)

	s.Success = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	s.Warning = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	s.Error = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	s.Info = lipgloss.NewStyle().Foreground(ColorPrimary)

	s.RiskLow = lipgloss.NewStyle().
		Background(ColorSuccess).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Bold(true)
	s.RiskMedium = lipgloss.NewStyle().
		Background(ColorWarning).
		Foreground(lipgloss.Color("#000000")).
		Padding(0, 1).
		Bold(true)
	s.RiskHigh = lipgloss.NewStyle().
		Background(ColorError).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Bold(true)
	s.RiskBlocked = lipgloss.NewStyle().
		Background(lipgloss.Color("#FF0000")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Bold(true)

	s.Input = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)
	s.InputFocused = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1)

	s.ListItem = lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(ColorText)
	s.ListSelected = lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(ColorPrimary).
		Bold(true).
		Background(ColorSurface)

	s.HelpKey = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	s.HelpDesc = lipgloss.NewStyle().Foreground(ColorMuted)
	s.HelpSep = lipgloss.NewStyle().Foreground(ColorBorder)

	return s
}
