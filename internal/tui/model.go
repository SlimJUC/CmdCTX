// Package tui implements the full-screen Bubble Tea TUI for cmdctx.
// It uses strict model/update/view separation with typed messages for async operations.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slim/cmdctx/internal/ai"
	"github.com/slim/cmdctx/internal/commands"
	"github.com/slim/cmdctx/internal/config"
	"github.com/slim/cmdctx/internal/history"
	"github.com/slim/cmdctx/internal/intent"
	"github.com/slim/cmdctx/internal/policy"
	"github.com/slim/cmdctx/internal/retrieval"
	"github.com/slim/cmdctx/internal/runner"
	"github.com/slim/cmdctx/internal/tui/theme"
	"github.com/slim/cmdctx/internal/utils"
)

// Screen represents a named screen in the TUI navigation stack.
type Screen int

const (
	ScreenHome Screen = iota
	ScreenResult
	ScreenExecution
	ScreenContext
	ScreenHistory
	ScreenSettings
)

// ---- Messages ----------------------------------------------------------------

// intentResultMsg is sent when intent parsing completes.
type intentResultMsg struct {
	parsed    *intent.Intent
	cmdResult *commands.Result
	parsedBy  string
	err       error
}

// execResultMsg is sent when command execution completes.
type execResultMsg struct {
	result *runner.Result
	err    error
}

// historyLoadedMsg is sent when history has been loaded from the DB.
type historyLoadedMsg struct {
	entries []history.Entry
	err     error
}

// windowSizeMsg is a local alias for tea.WindowSizeMsg.
type windowSizeMsg = tea.WindowSizeMsg

// ---- Model -------------------------------------------------------------------

// Model is the root Bubble Tea model. It holds all application state.
type Model struct {
	// Navigation.
	screen Screen

	// Terminal dimensions.
	width  int
	height int

	// Shared dependencies.
	cfg    *config.Config
	pol    *policy.Policy
	store  *history.Store
	styles *theme.Styles

	// Home screen state.
	input          textinput.Model
	inputFocused   bool
	recentItems    []history.Entry
	currentProject string

	// Processing state.
	loading bool
	spinner spinner.Model
	loadMsg string

	// Result screen state.
	currentRequest  string
	currentIntent   *intent.Intent
	currentResult   *commands.Result
	currentParsedBy string

	// Execution screen state.
	execResult *runner.Result
	execHistID int64
	stdout     viewport.Model
	stderr     viewport.Model
	activeTab  int // 0=stdout, 1=stderr, 2=metadata

	// History screen state.
	historyEntries []history.Entry
	historyIdx     int
	historyScroll  viewport.Model

	// Context screen state.
	contextScroll viewport.Model

	// Error state.
	err error

	// Help visible.
	showHelp bool

	// Status line.
	statusMsg   string
	statusTimer time.Time
}

// Run is the TUI entry point — called from the CLI.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	pol, err := policy.Load()
	if err != nil {
		pol = policy.Default()
	}

	store, _ := history.Open()

	m := newModel(cfg, pol, store)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	// Close the DB after TUI exits.
	if fm, ok := finalModel.(Model); ok {
		if fm.store != nil {
			fm.store.Close()
		}
	}

	return nil
}

func newModel(cfg *config.Config, pol *policy.Policy, store *history.Store) Model {
	ti := textinput.New()
	ti.Placeholder = "What do you want to find? (e.g. search nginx logs for 500 errors today)"
	ti.CharLimit = 512
	ti.Width = 80

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := Model{
		screen:        ScreenHome,
		cfg:           cfg,
		pol:           pol,
		store:         store,
		styles:        theme.Default(),
		input:         ti,
		inputFocused:  true,
		spinner:       sp,
		stdout:        viewport.New(80, 20),
		stderr:        viewport.New(80, 20),
		historyScroll: viewport.New(80, 30),
		contextScroll: viewport.New(80, 30),
	}

	return m
}

// ---- Init --------------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
		m.loadRecentHistory(),
		m.detectProject(),
	)
}

// ---- Update ------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = msg.Width - 10
		m.stdout = viewport.New(msg.Width-4, msg.Height-12)
		m.stderr = viewport.New(msg.Width-4, msg.Height-12)
		m.historyScroll = viewport.New(msg.Width-4, msg.Height-10)
		m.contextScroll = viewport.New(msg.Width-4, msg.Height-10)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case intentResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.setStatus("Error: " + msg.err.Error())
			return m, nil
		}
		m.currentIntent = msg.parsed
		m.currentResult = msg.cmdResult
		m.currentParsedBy = msg.parsedBy
		m.err = nil
		m.screen = ScreenResult
		return m, nil

	case execResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.setStatus("Execution error: " + msg.err.Error())
			return m, nil
		}
		m.execResult = msg.result
		m.stdout.SetContent(msg.result.Stdout)
		m.stderr.SetContent(msg.result.Stderr)
		m.screen = ScreenExecution
		return m, nil

	case historyLoadedMsg:
		if msg.err == nil {
			if m.screen == ScreenHome {
				m.recentItems = msg.entries
			} else {
				m.historyEntries = msg.entries
				// Build scrollable content for history screen.
				var content strings.Builder
				for _, e := range msg.entries {
					execStr := "─"
					if e.Executed {
						execStr = "✓"
						if e.ExitCode != nil {
							execStr = fmt.Sprintf("✓(%d)", *e.ExitCode)
						}
					}
					content.WriteString(fmt.Sprintf("#%-5d %s  [%s] %s\n         → %s\n\n",
						e.ID,
						e.CreatedAt.Format("2006-01-02 15:04"),
						execStr,
						e.Prompt,
						e.RenderedCmd,
					))
				}
				m.historyScroll.SetContent(content.String())
			}
		}
		return m, nil

	case contextContentMsg:
		m.contextScroll.SetContent(msg.content)
		return m, nil

	case projectDetectedMsg:
		m.currentProject = msg.root
		return m, nil
	}

	// Pass events to active viewport/input.
	return m.updateActiveComponent(msg, cmds)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.store != nil {
			m.store.Close()
		}
		return m, tea.Quit

	case "q":
		if m.screen != ScreenHome || !m.inputFocused {
			m.screen = ScreenHome
			m.err = nil
			m.input.Focus()
			m.inputFocused = true
			return m, textinput.Blink
		}

	case "esc":
		if m.screen != ScreenHome {
			m.screen = ScreenHome
			m.err = nil
			m.input.Focus()
			m.inputFocused = true
			return m, textinput.Blink
		}

	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	case "tab":
		return m.handleTab()

	case "enter":
		return m.handleEnter()

	case "1":
		if m.screen == ScreenHome {
			m.screen = ScreenHome
		}
	case "2":
		if m.currentResult != nil {
			m.screen = ScreenResult
		}
	case "3":
		if m.execResult != nil {
			m.screen = ScreenExecution
		}
	case "4":
		m.screen = ScreenContext
		return m, m.loadContextContent()
	case "5":
		m.screen = ScreenHistory
		return m, m.loadHistory(50)
	case "6":
		m.screen = ScreenSettings

	// Execution screen tab switching.
	case "left", "h":
		if m.screen == ScreenExecution && m.activeTab > 0 {
			m.activeTab--
		}
	case "right", "l":
		if m.screen == ScreenExecution && m.activeTab < 2 {
			m.activeTab++
		}
	}

	return m, nil
}

func (m Model) handleTab() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenHome:
		m.inputFocused = !m.inputFocused
		if m.inputFocused {
			m.input.Focus()
			return m, textinput.Blink
		}
		m.input.Blur()
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenHome:
		request := strings.TrimSpace(m.input.Value())
		if request == "" {
			return m, nil
		}
		m.currentRequest = request
		m.input.SetValue("")
		m.loading = true
		m.loadMsg = "Parsing intent..."
		return m, tea.Batch(m.spinner.Tick, m.parseIntent(request))

	case ScreenResult:
		// Enter on result screen → confirm execution.
		if m.currentResult != nil {
			m.loading = true
			m.loadMsg = "Executing..."
			return m, tea.Batch(m.spinner.Tick, m.executeCmd())
		}
	}
	return m, nil
}

func (m Model) updateActiveComponent(msg tea.Msg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch m.screen {
	case ScreenHome:
		if m.inputFocused {
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}

	case ScreenExecution:
		switch m.activeTab {
		case 0:
			m.stdout, cmd = m.stdout.Update(msg)
			cmds = append(cmds, cmd)
		case 1:
			m.stderr, cmd = m.stderr.Update(msg)
			cmds = append(cmds, cmd)
		}

	case ScreenHistory:
		m.historyScroll, cmd = m.historyScroll.Update(msg)
		cmds = append(cmds, cmd)

	case ScreenContext:
		m.contextScroll, cmd = m.contextScroll.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// ---- View --------------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	var sections []string
	sections = append(sections, m.renderHeader())
	sections = append(sections, m.renderBody())
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderHeader() string {
	provider := "none"
	if m.cfg != nil {
		provider = m.cfg.ActiveProvider
	}

	screenNames := map[Screen]string{
		ScreenHome:      "Home",
		ScreenResult:    "Result",
		ScreenExecution: "Execution",
		ScreenContext:   "Context",
		ScreenHistory:   "History",
		ScreenSettings:  "Settings",
	}

	title := " cmdctx "
	screenName := screenNames[m.screen]
	project := utils.ShortenPath(m.currentProject)
	if project == "" {
		project = "~"
	}

	left := title + "│ " + screenName
	right := "provider:" + provider + " │ " + project + " "
	padding := m.width - len(left) - len(right)
	if padding < 0 {
		padding = 0
	}

	header := m.styles.Header.Width(m.width).Render(
		left + strings.Repeat(" ", padding) + right,
	)
	return header
}

func (m Model) renderBody() string {
	// Reserve lines for header (1) + footer (2).
	bodyHeight := m.height - 3
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var content string
	if m.loading {
		content = m.renderLoading()
	} else if m.err != nil && m.screen == ScreenHome {
		content = m.renderError()
	} else {
		switch m.screen {
		case ScreenHome:
			content = m.renderHome()
		case ScreenResult:
			content = m.renderResult()
		case ScreenExecution:
			content = m.renderExecution()
		case ScreenContext:
			content = m.renderContext()
		case ScreenHistory:
			content = m.renderHistory()
		case ScreenSettings:
			content = m.renderSettings()
		}
	}

	// Pad body to fill terminal.
	lines := strings.Split(content, "\n")
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderFooter() string {
	var help string
	switch m.screen {
	case ScreenHome:
		help = m.helpBar([][2]string{
			{"Enter", "Submit"},
			{"Tab", "Focus"},
			{"4", "Context"},
			{"5", "History"},
			{"6", "Settings"},
			{"?", "Help"},
			{"Ctrl+C", "Quit"},
		})
	case ScreenResult:
		help = m.helpBar([][2]string{
			{"Enter", "Execute"},
			{"c", "Copy cmd"},
			{"Esc", "Back"},
			{"q", "Back"},
		})
	case ScreenExecution:
		help = m.helpBar([][2]string{
			{"←/→", "Switch tab"},
			{"↑/↓", "Scroll"},
			{"Esc", "Back"},
		})
	default:
		help = m.helpBar([][2]string{
			{"Esc", "Back"},
			{"↑/↓", "Scroll"},
			{"q", "Back"},
		})
	}

	statusLine := ""
	if m.statusMsg != "" && time.Since(m.statusTimer) < 3*time.Second {
		statusLine = " " + m.statusMsg
	}

	footer := m.styles.Footer.Width(m.width).Render(help + statusLine)
	return footer
}

func (m Model) renderHome() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(m.styles.Title.Render("  What do you want to find?"))
	b.WriteString("\n\n")

	// Input box.
	inputStyle := m.styles.Input
	if m.inputFocused {
		inputStyle = m.styles.InputFocused
	}
	b.WriteString(inputStyle.Width(m.width - 6).Render(m.input.View()))
	b.WriteString("\n\n")

	// Recent prompts.
	if len(m.recentItems) > 0 {
		b.WriteString(m.styles.Muted.Render("  Recent:\n"))
		max := 5
		if len(m.recentItems) < max {
			max = len(m.recentItems)
		}
		for i := 0; i < max; i++ {
			e := m.recentItems[i]
			b.WriteString(m.styles.Muted.Render(fmt.Sprintf("    %s  %s\n",
				e.CreatedAt.Format("15:04"),
				truncate(e.Prompt, m.width-20),
			)))
		}
	} else {
		b.WriteString(m.styles.Muted.Render("  No recent prompts yet.\n"))
		b.WriteString(m.styles.Muted.Render("  Try: search nginx logs for 500 errors today\n"))
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Muted.Render("  Tip: Press 1-6 to switch screens\n"))

	return b.String()
}

func (m Model) renderResult() string {
	if m.currentResult == nil {
		return "\n  No result yet."
	}

	var b strings.Builder
	i := m.currentIntent
	r := m.currentResult

	b.WriteString("\n")
	b.WriteString(m.styles.Bold.Render("  Request:\n"))
	b.WriteString(fmt.Sprintf("    %s\n\n", m.currentRequest))

	b.WriteString(m.styles.Bold.Render("  Intent:\n"))
	b.WriteString(fmt.Sprintf("    %s\n\n", i.Intent))

	b.WriteString(m.styles.Bold.Render("  Generated Command:\n"))
	b.WriteString(m.styles.Code.Render("  "+r.Display) + "\n\n")

	if r.Explanation != "" {
		b.WriteString(m.styles.Bold.Render("  Explanation:\n"))
		b.WriteString(fmt.Sprintf("    %s\n\n", r.Explanation))
	}

	if len(i.Assumptions) > 0 {
		b.WriteString(m.styles.Bold.Render("  Assumptions:\n"))
		for _, a := range i.Assumptions {
			b.WriteString(fmt.Sprintf("    • %s\n", a))
		}
		b.WriteString("\n")
	}

	if len(r.Warnings) > 0 {
		for _, w := range r.Warnings {
			b.WriteString(m.styles.Warning.Render("  ⚠  "+w) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.styles.Bold.Render("  Risk: "))
	b.WriteString(m.renderRisk(r.Risk) + "\n")

	if m.currentParsedBy == "rule_based" {
		b.WriteString(m.styles.Muted.Render("\n  (rule-based fallback — no AI provider)") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Info.Render("  [Enter] Execute  [c] Copy  [Esc] Back\n"))

	return b.String()
}

func (m Model) renderExecution() string {
	if m.execResult == nil {
		return "\n  No execution result."
	}

	var b strings.Builder
	r := m.execResult

	// Tab bar.
	tabs := []string{"stdout", "stderr", "metadata"}
	tabBar := m.renderTabs(tabs, m.activeTab)
	b.WriteString(tabBar + "\n\n")

	switch m.activeTab {
	case 0:
		if r.Stdout == "" {
			b.WriteString(m.styles.Muted.Render("  (no output)\n"))
		} else {
			b.WriteString(m.stdout.View())
		}
	case 1:
		if r.Stderr == "" {
			b.WriteString(m.styles.Muted.Render("  (no stderr)\n"))
		} else {
			b.WriteString(m.stderr.View())
		}
	case 2:
		status := m.styles.Success.Render("✓ success")
		if r.ExitCode != 0 {
			status = m.styles.Error.Render(fmt.Sprintf("✗ exit %d", r.ExitCode))
		}
		if r.TimedOut {
			status = m.styles.Warning.Render("⏱ timed out")
		}
		b.WriteString(fmt.Sprintf("\n  Status:   %s\n", status))
		b.WriteString(fmt.Sprintf("  Duration: %s\n", r.Duration.Round(time.Millisecond)))
		b.WriteString(fmt.Sprintf("  Started:  %s\n", r.StartedAt.Format("15:04:05")))
		if r.Truncated {
			b.WriteString(m.styles.Warning.Render("  ⚠ output was truncated\n"))
		}
		b.WriteString(fmt.Sprintf("\n  Command: %s\n", utils.JoinArgs(r.Argv)))
	}

	return b.String()
}

func (m Model) renderContext() string {
	return m.contextScroll.View()
}

func (m Model) renderHistory() string {
	if len(m.historyEntries) == 0 {
		return "\n  No history entries found.\n  Run some commands first."
	}
	return m.historyScroll.View()
}

func (m Model) renderSettings() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(m.styles.Title.Render("  Settings") + "\n\n")

	if m.cfg == nil {
		b.WriteString("  No configuration loaded.\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("  %-20s %s\n", "Active provider:", m.cfg.ActiveProvider))
	b.WriteString(fmt.Sprintf("  %-20s %s\n", "Scan mode:", m.cfg.DefaultScanMode))
	b.WriteString(fmt.Sprintf("  %-20s %s\n", "Exec timeout:", m.cfg.ExecutionTimeout))
	b.WriteString(fmt.Sprintf("  %-20s %d KiB\n", "Output limit:", m.cfg.OutputMaxBytes/1024))
	b.WriteString(fmt.Sprintf("  %-20s %d days\n", "History retention:", m.cfg.HistoryRetention))
	b.WriteString(fmt.Sprintf("  %-20s %s\n", "AI permission:", m.cfg.AIPermissionMode))

	if p := m.cfg.ActiveProviderConfig(); p != nil {
		b.WriteString("\n  Active Provider:\n")
		b.WriteString(fmt.Sprintf("    %-16s %s\n", "Name:", p.Name))
		b.WriteString(fmt.Sprintf("    %-16s %s\n", "Type:", p.Type))
		b.WriteString(fmt.Sprintf("    %-16s %s\n", "Model:", p.Model))
		if p.BaseURL != "" {
			b.WriteString(fmt.Sprintf("    %-16s %s\n", "Base URL:", p.BaseURL))
		}
	} else {
		b.WriteString("\n  " + m.styles.Warning.Render("No AI provider configured.") + "\n")
		b.WriteString("  Run: cmdctx providers add --name local --type ollama --model llama3.2\n")
	}

	b.WriteString("\n  " + m.styles.Muted.Render("Config file: "+utils.ShortenPath(config.GlobalConfigPath())) + "\n")

	return b.String()
}

func (m Model) renderLoading() string {
	return fmt.Sprintf("\n\n  %s %s\n", m.spinner.View(), m.loadMsg)
}

func (m Model) renderError() string {
	if m.err == nil {
		return ""
	}
	return "\n" + m.styles.Error.Render("  Error: "+m.err.Error()) + "\n"
}

// ---- Async commands ----------------------------------------------------------

func (m Model) parseIntent(request string) tea.Cmd {
	return func() tea.Msg {
		cfg := m.cfg
		if cfg == nil {
			cfg = config.DefaultConfig()
		}

		pol := m.pol
		if pol == nil {
			pol = policy.Default()
		}

		// Get context snippets.
		var snippets []string
		if m.store != nil {
			ret := retrieval.New(m.store)
			snippets = ret.RelevantSnippets(request)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		aiProvider, _ := ai.NewFromConfig(cfg)
		if aiProvider == nil {
			aiProvider = &ai.NullProvider{}
		}

		parser := intent.NewParser(aiProvider)
		parsed, parsedBy, err := parser.ParseWithFallback(ctx, request, snippets)
		if err != nil {
			return intentResultMsg{err: err}
		}

		tools := utils.AvailableTools([]string{"rg", "fd", "grep", "find", "jq"})
		cmdResult, err := commands.Build(parsed, pol, tools)
		if err != nil {
			return intentResultMsg{err: err}
		}
		cmdResult.ParsedBy = parsedBy

		// Record in history.
		if m.store != nil {
			intentJSON, _ := json.Marshal(parsed)
			_, _ = m.store.Record(&history.Entry{
				Prompt:      request,
				IntentType:  string(parsed.Intent),
				IntentJSON:  string(intentJSON),
				RenderedCmd: cmdResult.Display,
				ParsedBy:    parsedBy,
				Risk:        string(cmdResult.Risk),
			})
		}

		return intentResultMsg{parsed: parsed, cmdResult: cmdResult, parsedBy: parsedBy}
	}
}

func (m Model) executeCmd() tea.Cmd {
	return func() tea.Msg {
		if m.currentResult == nil {
			return execResultMsg{err: fmt.Errorf("no command to execute")}
		}

		runOpts := runner.DefaultOptions()
		if m.cfg != nil {
			if m.cfg.ExecutionTimeout > 0 {
				runOpts.Timeout = m.cfg.ExecutionTimeout
			}
			if m.cfg.OutputMaxBytes > 0 {
				runOpts.MaxOutputBytes = m.cfg.OutputMaxBytes
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), runOpts.Timeout)
		defer cancel()

		result, err := runner.Run(ctx, m.currentResult.Argv, runOpts)
		if err != nil {
			return execResultMsg{err: err}
		}
		return execResultMsg{result: result}
	}
}

func (m Model) loadRecentHistory() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return historyLoadedMsg{}
		}
		entries, err := m.store.List(5)
		return historyLoadedMsg{entries: entries, err: err}
	}
}

func (m Model) loadHistory(limit int) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return historyLoadedMsg{}
		}
		entries, err := m.store.List(limit)
		if err == nil {
			// Build view content.
			var content strings.Builder
			for _, e := range entries {
				execStr := "─"
				if e.Executed {
					execStr = "✓"
					if e.ExitCode != nil {
						execStr = fmt.Sprintf("✓(%d)", *e.ExitCode)
					}
				}
				content.WriteString(fmt.Sprintf("#%-5d %s  [%s] %s\n         → %s\n\n",
					e.ID,
					e.CreatedAt.Format("2006-01-02 15:04"),
					execStr,
					e.Prompt,
					e.RenderedCmd,
				))
			}
			_ = content // stored in historyEntries, view is set below
		}
		return historyLoadedMsg{entries: entries, err: err}
	}
}

func (m Model) loadContextContent() tea.Cmd {
	return func() tea.Msg {
		var b strings.Builder
		b.WriteString("Machine Context\n")
		b.WriteString(strings.Repeat("─", 60) + "\n\n")

		// Try to load machine context.
		if mc, err := loadMachineCtx(); err == nil {
			b.WriteString(fmt.Sprintf("Hostname: %s\n", mc.Hostname))
			b.WriteString(fmt.Sprintf("OS:       %s/%s\n", mc.OS, mc.Arch))
			b.WriteString(fmt.Sprintf("Home:     %s\n\n", mc.HomeDir))

			if len(mc.ToolsAvailable) > 0 {
				b.WriteString("Tools: ")
				first := true
				for t := range mc.ToolsAvailable {
					if !first {
						b.WriteString(", ")
					}
					b.WriteString(t)
					first = false
				}
				b.WriteString("\n\n")
			}

			if len(mc.LogDirs) > 0 {
				b.WriteString("Log dirs:\n")
				for _, d := range mc.LogDirs {
					b.WriteString("  " + d + "\n")
				}
				b.WriteString("\n")
			}

			if len(mc.Frameworks) > 0 {
				b.WriteString("Detected stacks:\n")
				for _, fw := range mc.Frameworks {
					b.WriteString(fmt.Sprintf("  %s (%s, %s)\n", fw.Name, fw.Category, fw.Confidence))
				}
			}
		} else {
			b.WriteString("No machine context found.\nRun: cmdctx init\n")
		}

		return contextContentMsg{content: b.String()}
	}
}

type contextContentMsg struct{ content string }

func (m Model) detectProject() tea.Cmd {
	return func() tea.Msg {
		return projectDetectedMsg{root: getCurrentProjectRoot()}
	}
}

type projectDetectedMsg struct{ root string }

// Update needs to handle contextContentMsg and projectDetectedMsg.
// We override the Update to add these cases.
func init() {
	// No init needed — handled inline via type switch above.
}

// ---- Rendering helpers -------------------------------------------------------

func (m Model) renderRisk(risk policy.RiskLevel) string {
	switch risk {
	case policy.RiskLow:
		return m.styles.RiskLow.Render(" low ")
	case policy.RiskMedium:
		return m.styles.RiskMedium.Render(" medium ")
	case policy.RiskHigh:
		return m.styles.RiskHigh.Render(" high ")
	case policy.RiskBlocked:
		return m.styles.RiskBlocked.Render(" blocked ")
	default:
		return string(risk)
	}
}

func (m Model) renderTabs(tabs []string, active int) string {
	var parts []string
	for i, tab := range tabs {
		if i == active {
			parts = append(parts, m.styles.Bold.Render("[ "+tab+" ]"))
		} else {
			parts = append(parts, m.styles.Muted.Render("  "+tab+"  "))
		}
	}
	return "  " + strings.Join(parts, " ")
}

func (m Model) helpBar(pairs [][2]string) string {
	var parts []string
	for _, pair := range pairs {
		key := m.styles.HelpKey.Render(pair[0])
		desc := m.styles.HelpDesc.Render(" " + pair[1])
		parts = append(parts, key+desc)
	}
	return strings.Join(parts, m.styles.HelpSep.Render(" │ "))
}

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
	m.statusTimer = time.Now()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
