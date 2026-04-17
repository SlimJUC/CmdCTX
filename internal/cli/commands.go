package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/slim/cmdctx/internal/config"
	"github.com/slim/cmdctx/internal/contextgen"
	"github.com/slim/cmdctx/internal/contextscan"
	"github.com/slim/cmdctx/internal/history"
	"github.com/slim/cmdctx/internal/install"
	"github.com/slim/cmdctx/internal/policy"
	"github.com/slim/cmdctx/internal/retrieval"
	"github.com/slim/cmdctx/internal/utils"
)

// ---- init -------------------------------------------------------------------

func newInitCmd() *cobra.Command {
	var scanMode string
	var projectPath string
	var skipPrompt bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate machine and project context files",
		Long: `Scan the local machine and optionally a project directory to generate
context files used by cmdctx for accurate command generation.

Context files created:
  ~/.cmdctx/machine-context.md
  ~/.cmdctx/machine-context.json
  ~/.cmdctx/command-policy.json
  <project>/.cmdctx/project-context.md
  <project>/.cmdctx/project-context.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(scanMode, projectPath, skipPrompt)
		},
	}

	cmd.Flags().StringVar(&scanMode, "mode", "safe", "Scan mode: safe or deep")
	cmd.Flags().StringVar(&projectPath, "project", "", "Project directory to scan (default: current dir)")
	cmd.Flags().BoolVar(&skipPrompt, "yes", false, "Skip confirmation prompts")

	return cmd
}

func runInit(scanMode, projectPath string, skipPrompt bool) error {
	if err := config.EnsureGlobalDir(); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	// Ensure policy file exists.
	pol := policy.Default()
	if err := policy.Save(pol); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save policy file: %v\n", err)
	}

	// Ensure config file exists.
	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save config file: %v\n", err)
	}

	mode := contextscan.ScanModeSafe
	if strings.ToLower(scanMode) == "deep" {
		mode = contextscan.ScanModeDeep
	}

	opts := contextscan.DefaultOptions()
	opts.Mode = mode

	fmt.Println("\n▶ Scanning machine context...")
	scanner := contextscan.New(opts)
	machineScan, err := scanner.ScanMachine()
	if err != nil {
		return fmt.Errorf("scanning machine: %w", err)
	}

	mc, err := contextgen.GenerateMachineContext(machineScan)
	if err != nil {
		return fmt.Errorf("generating machine context: %w", err)
	}

	fmt.Printf("  ✓ Scanned %d files, %d dirs\n", machineScan.TotalFiles, machineScan.TotalDirs)
	fmt.Printf("  ✓ Detected tools: %d\n", len(mc.ToolsAvailable))
	fmt.Printf("  ✓ Detected stacks: %d\n", len(mc.Frameworks))

	// Index machine context for retrieval.
	store, storeErr := history.Open()
	if storeErr == nil {
		defer store.Close()
		ret := retrieval.New(store)
		_ = ret.IndexMachineContext(mc)
	}

	// Scan project context.
	root := projectPath
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)

	fmt.Printf("\n▶ Scanning project context: %s\n", utils.ShortenPath(root))
	projectScan, err := scanner.ScanProject(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not scan project: %v\n", err)
	} else {
		_, err = contextgen.GenerateProjectContext(root, projectScan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not generate project context: %v\n", err)
		} else {
			fmt.Printf("  ✓ Scanned %d files, %d dirs\n", projectScan.TotalFiles, projectScan.TotalDirs)
			if store != nil {
				ret := retrieval.New(store)
				pc, _ := contextgen.LoadProjectContext(root)
				if pc != nil {
					_ = ret.IndexProjectContext(pc)
				}
			}
		}
	}

	fmt.Println("\n✓ Context generation complete!")
	fmt.Printf("  Config dir: %s\n", config.GlobalDir())
	fmt.Printf("  Run 'cmdctx doctor' to verify your setup.\n\n")

	return nil
}

// ---- ask --------------------------------------------------------------------

func newAskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask <natural language request>",
		Short: "Preview generated command without executing",
		Long:  `Parse a natural language request and show the generated command, explanation, and risk level. Never executes.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Force no-exec for ask.
			flagNoExec = true
			return runNaturalLanguageRequest(cmd, strings.Join(args, " "))
		},
	}
	return cmd
}

// ---- run --------------------------------------------------------------------

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <natural language request>",
		Short: "Generate command and offer execution",
		Long:  `Parse a natural language request, show the generated command, and prompt for execution.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flagRun = true
			return runNaturalLanguageRequest(cmd, strings.Join(args, " "))
		},
	}
	return cmd
}

// ---- tui --------------------------------------------------------------------

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive full-screen TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Import handled in tui package to avoid circular deps.
			return launchTUI()
		},
	}
}

// launchTUI is defined in tui.go in this package.
// This avoids importing the tui package here (it imports cli transitively).

// ---- refresh ----------------------------------------------------------------

func newRefreshCmd() *cobra.Command {
	var scanMode string

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Regenerate context files",
		Long:  "Re-scan and regenerate machine and project context files, then re-index for retrieval.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Refreshing context...")
			return runInit(scanMode, "", true)
		},
	}
	cmd.Flags().StringVar(&scanMode, "mode", "safe", "Scan mode: safe or deep")
	return cmd
}

// ---- doctor -----------------------------------------------------------------

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check installation and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}
}

func runDoctor() error {
	fmt.Printf("\n  cmdctx doctor\n\n")

	cfg, cfgErr := config.Load()
	printCheck("Config file", config.GlobalConfigPath(), cfgErr == nil)

	pol, polErr := policy.Load()
	printCheck("Policy file", policy.PolicyPath(), polErr == nil)
	_ = pol

	printCheck("Machine context", filepath.Join(config.GlobalDir(), "machine-context.json"),
		utils.FileExists(filepath.Join(config.GlobalDir(), "machine-context.json")))

	printCheck("History DB", history.DBPath(),
		utils.FileExists(history.DBPath()))

	fmt.Println()

	// Tool availability.
	fmt.Println("  Tools:")
	tools := []string{"rg", "fd", "grep", "find", "jq", "awk", "sed", "git"}
	for _, tool := range tools {
		if utils.ToolAvailable(tool) {
			fmt.Printf("    ✓ %-12s available\n", tool)
		} else {
			fmt.Printf("    ✗ %-12s not found\n", tool)
		}
	}
	fmt.Println()

	// Install location.
	installDir := install.DefaultInstallDir()
	if cfg != nil && cfg.InstallDir != "" {
		installDir = cfg.InstallDir
	}
	inPath := install.IsInPath(installDir)
	if inPath {
		fmt.Printf("  ✓ Install dir in PATH: %s\n", installDir)
	} else {
		fmt.Printf("  ⚠  Install dir NOT in PATH: %s\n", installDir)
		fmt.Println("     Add to PATH: export PATH=\"$HOME/.local/bin:$PATH\"")
	}

	// AI provider.
	fmt.Println()
	if cfg != nil {
		fmt.Printf("  AI provider: %s\n", cfg.ActiveProvider)
		if p := cfg.ActiveProviderConfig(); p != nil {
			fmt.Printf("  Model:       %s\n", p.Model)
			fmt.Printf("  Type:        %s\n", p.Type)
		} else {
			fmt.Println("  ⚠  No active provider configured — run 'cmdctx providers'")
		}
	}

	fmt.Println()
	return nil
}

func printCheck(label, path string, ok bool) {
	if ok {
		fmt.Printf("  ✓ %-20s %s\n", label, utils.ShortenPath(path))
	} else {
		fmt.Printf("  ✗ %-20s %s (missing)\n", label, utils.ShortenPath(path))
	}
}

// ---- history ----------------------------------------------------------------

func newHistoryCmd() *cobra.Command {
	var limit int
	var searchQuery string

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Browse command history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(limit, searchQuery)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Number of entries to show")
	cmd.Flags().StringVar(&searchQuery, "search", "", "Filter by prompt content")

	return cmd
}

func runHistory(limit int, search string) error {
	store, err := history.Open()
	if err != nil {
		return fmt.Errorf("opening history: %w", err)
	}
	defer store.Close()

	var entries []history.Entry
	if search != "" {
		entries, err = store.Search(search, limit)
	} else {
		entries, err = store.List(limit)
	}
	if err != nil {
		return fmt.Errorf("reading history: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No history entries found.")
		return nil
	}

	fmt.Printf("\n  History (%d entries)\n", len(entries))
	fmt.Println(strings.Repeat("─", 70))

	for _, e := range entries {
		execStr := "─"
		if e.Executed {
			execStr = fmt.Sprintf("✓ exit:%d", func() int {
				if e.ExitCode != nil {
					return *e.ExitCode
				}
				return -1
			}())
		}
		fmt.Printf("  #%-5d %s  [%s] %s\n",
			e.ID,
			e.CreatedAt.Format("2006-01-02 15:04"),
			execStr,
			truncateStr(e.Prompt, 50),
		)
		fmt.Printf("         → %s\n", truncateStr(e.RenderedCmd, 60))
		fmt.Println()
	}

	return nil
}

// ---- config -----------------------------------------------------------------

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View or edit configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Show config file path",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(config.GlobalConfigPath())
		},
	})

	// Default: show config.
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	return cmd
}

// ---- providers --------------------------------------------------------------

func newProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Manage AI providers",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Providers) == 0 {
				fmt.Println("No providers configured.")
				fmt.Println("\nQuick setup:")
				fmt.Println("  # Ollama (local, recommended):")
				fmt.Println("  cmdctx providers add --name local --type ollama --model llama3.2")
				fmt.Println()
				fmt.Println("  # OpenAI:")
				fmt.Println("  cmdctx providers add --name openai --type openai --key sk-... --model gpt-4o-mini")
				return nil
			}
			fmt.Printf("\n  Providers (active: %s)\n\n", cfg.ActiveProvider)
			for _, p := range cfg.Providers {
				active := " "
				if p.Name == cfg.ActiveProvider {
					active = "▶"
				}
				fmt.Printf("  %s %-15s type:%-10s model:%s\n", active, p.Name, p.Type, p.Model)
			}
			fmt.Println()
			return nil
		},
	})

	var addName, addType, addModel, addKey, addURL string
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update a provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			if addName == "" || addType == "" {
				return fmt.Errorf("--name and --type are required")
			}
			cfg, err := config.Load()
			if err != nil {
				cfg = config.DefaultConfig()
			}

			// Check if provider already exists.
			found := false
			for i, p := range cfg.Providers {
				if p.Name == addName {
					cfg.Providers[i] = config.Provider{
						Name:    addName,
						Type:    addType,
						Model:   addModel,
						APIKey:  addKey,
						BaseURL: addURL,
					}
					found = true
					break
				}
			}
			if !found {
				cfg.Providers = append(cfg.Providers, config.Provider{
					Name:    addName,
					Type:    addType,
					Model:   addModel,
					APIKey:  addKey,
					BaseURL: addURL,
				})
			}

			cfg.ActiveProvider = addName
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Printf("✓ Provider '%s' configured and set as active.\n", addName)
			return nil
		},
	}
	addCmd.Flags().StringVar(&addName, "name", "", "Provider name (identifier)")
	addCmd.Flags().StringVar(&addType, "type", "ollama", "Provider type: ollama, openai, anthropic")
	addCmd.Flags().StringVar(&addModel, "model", "", "Model name")
	addCmd.Flags().StringVar(&addKey, "key", "", "API key")
	addCmd.Flags().StringVar(&addURL, "url", "", "Custom base URL")
	cmd.AddCommand(addCmd)

	var useProvider string
	useCmd := &cobra.Command{
		Use:   "use",
		Short: "Set the active provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useProvider == "" && len(args) > 0 {
				useProvider = args[0]
			}
			if useProvider == "" {
				return fmt.Errorf("provider name required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cfg.ActiveProvider = useProvider
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("✓ Active provider set to '%s'\n", useProvider)
			return nil
		},
	}
	useCmd.Flags().StringVar(&useProvider, "provider", "", "Provider name to activate")
	cmd.AddCommand(useCmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	return cmd
}

// ---- helpers ----------------------------------------------------------------

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// formatDuration formats a duration for display.
func formatDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return d.Round(time.Millisecond).String()
}

var _ = formatDuration // used in TUI
