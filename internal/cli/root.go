// Package cli implements all Cobra CLI commands for cmdctx.
// The root command implements shorthand natural-language mode:
// if the first argument is not a known subcommand, the full argument list
// is treated as a natural-language request.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/slim/cmdctx/internal/ai"
	"github.com/slim/cmdctx/internal/commands"
	"github.com/slim/cmdctx/internal/config"
	"github.com/slim/cmdctx/internal/history"
	"github.com/slim/cmdctx/internal/intent"
	"github.com/slim/cmdctx/internal/policy"
	"github.com/slim/cmdctx/internal/retrieval"
	"github.com/slim/cmdctx/internal/runner"
	"github.com/slim/cmdctx/internal/utils"
)

// knownSubcommands is the set of registered subcommand names.
// Anything not in this set triggers the shorthand NL mode.
var knownSubcommands = map[string]bool{
	"init":       true,
	"ask":        true,
	"run":        true,
	"tui":        true,
	"refresh":    true,
	"doctor":     true,
	"history":    true,
	"config":     true,
	"providers":  true,
	"help":       true,
	"completion": true,
}

// flags for shorthand NL mode.
var (
	flagRun    bool
	flagYes    bool
	flagNoExec bool
	flagJSON   bool
)

// NewRootCmd builds and returns the root Cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cmdctx [natural language request]",
		Short: "AI-powered local terminal assistant for safe command generation",
		Long: `cmdctx — Local AI command copilot

Generate safe shell search/investigation commands from natural language.

Examples:
  cmdctx find all php files containing "payment failed" except vendor and node_modules
  cmdctx search nginx logs for 500 errors today
  cmdctx count timeout occurrences in logs
  cmdctx "look for redis timeout references in this project"

Explicit subcommands:
  cmdctx init           Generate machine and project context
  cmdctx ask "<query>"  Preview generated command (no execution)
  cmdctx run "<query>"  Generate and offer to execute command
  cmdctx tui            Launch interactive full-screen TUI
  cmdctx refresh        Regenerate context files
  cmdctx doctor         Check installation and configuration
  cmdctx history        Browse command history
  cmdctx config         View or edit configuration
  cmdctx providers      Manage AI providers`,
		// DisableFlagParsing: false — we want flags like --yes to work.
		// TraverseChildren allows flags defined here to be parsed before subcommand detection.
		TraverseChildren: true,
		// RunE is called only when no subcommand matches — i.e., shorthand NL mode.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			// Join all args as the natural language request.
			// Shell already handles quoting — we just join what we received.
			request := strings.Join(args, " ")
			return runNaturalLanguageRequest(cmd, request)
		},
	}

	// Shorthand NL mode flags (also available on ask/run).
	root.PersistentFlags().BoolVar(&flagRun, "run", false, "Generate command and offer execution")
	root.PersistentFlags().BoolVar(&flagYes, "yes", false, "Auto-confirm execution for safe read-only commands only")
	root.PersistentFlags().BoolVar(&flagNoExec, "no-exec", false, "Preview only, never ask to execute")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output machine-readable JSON")

	// Register all subcommands.
	root.AddCommand(newInitCmd())
	root.AddCommand(newAskCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newTUICmd())
	root.AddCommand(newRefreshCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newProvidersCmd())

	return root
}

// runNaturalLanguageRequest is the shared handler for natural-language requests.
// It is used by both the shorthand root mode and the explicit ask/run subcommands.
func runNaturalLanguageRequest(cmd *cobra.Command, request string) error {
	if strings.TrimSpace(request) == "" {
		return fmt.Errorf("request cannot be empty")
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	pol, err := policy.Load()
	if err != nil {
		pol = policy.Default()
	}

	// Open history store — non-fatal if unavailable.
	var store *history.Store
	store, _ = history.Open()
	if store != nil {
		defer store.Close()
	}

	// Build retriever and get context snippets.
	var contextSnippets []string
	if store != nil {
		ret := retrieval.New(store)
		contextSnippets = ret.RelevantSnippets(request)
	}

	// Parse intent via AI (with rule-based fallback).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	aiProvider, provErr := ai.NewFromConfig(cfg)
	if provErr != nil {
		aiProvider = &ai.NullProvider{}
	}

	parser := intent.NewParser(aiProvider)
	parsedIntent, parsedBy, parseErr := parser.ParseWithFallback(ctx, request, contextSnippets)
	if parseErr != nil {
		return fmt.Errorf("could not parse intent: %w\n\nTip: run 'cmdctx providers' to configure an AI provider", parseErr)
	}

	// Discover available tools from context or PATH.
	tools := utils.AvailableTools([]string{"rg", "fd", "grep", "find", "jq", "awk", "sed"})

	// Build safe command.
	cmdResult, buildErr := commands.Build(parsedIntent, pol, tools)
	if buildErr != nil {
		return fmt.Errorf("command generation failed: %w", buildErr)
	}
	cmdResult.ParsedBy = parsedBy

	// Record in history (not executed yet).
	intentJSON, _ := json.Marshal(parsedIntent)
	histEntry := &history.Entry{
		Prompt:      request,
		IntentType:  string(parsedIntent.Intent),
		IntentJSON:  string(intentJSON),
		RenderedCmd: cmdResult.Display,
		ParsedBy:    parsedBy,
		Risk:        string(cmdResult.Risk),
	}
	var histID int64
	if store != nil {
		histID, _ = store.Record(histEntry)
	}

	// Output.
	if flagJSON {
		return outputJSON(cmd, request, parsedIntent, cmdResult)
	}

	printResult(request, parsedIntent, cmdResult)

	// Execution logic.
	if flagNoExec {
		return nil
	}

	// Determine whether to ask for execution.
	shouldPrompt := flagRun || !flagNoExec
	// Default behaviour (no --no-exec, no --run): still prompt, matching spec.
	if shouldPrompt {
		confirmed, confirmErr := runner.PromptConfirm(cmdResult.Display, cmdResult.Risk, flagYes)
		if confirmErr != nil {
			return confirmErr
		}

		if confirmed {
			return executeAndRecord(ctx, cmdResult, cfg, store, histID)
		}
		fmt.Println("\nNot executed.")
	}

	return nil
}

// printResult renders the command result to stdout in a human-readable format.
func printResult(request string, i *intent.Intent, result *commands.Result) {
	fmt.Println()
	fmt.Printf("  Request:    %s\n", request)
	fmt.Printf("  Intent:     %s\n", i.Intent)
	fmt.Printf("  Command:    %s\n", result.Display)
	fmt.Println()

	if result.Explanation != "" {
		fmt.Printf("  Explanation: %s\n", result.Explanation)
	}

	if len(result.Assumptions) > 0 {
		fmt.Println("  Assumptions:")
		for _, a := range result.Assumptions {
			fmt.Printf("    • %s\n", a)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("  Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("    ⚠  %s\n", w)
		}
	}

	fmt.Printf("  Risk:       %s\n", result.Risk)
	if result.ParsedBy == "rule_based" {
		fmt.Println("  (parsed by rule-based fallback — no AI provider configured)")
	}
	fmt.Println()
}

// outputJSON prints a machine-readable JSON summary.
func outputJSON(_ *cobra.Command, request string, i *intent.Intent, result *commands.Result) error {
	out := map[string]any{
		"request":     request,
		"intent":      i,
		"command":     result.Display,
		"argv":        result.Argv,
		"explanation": result.Explanation,
		"risk":        result.Risk,
		"warnings":    result.Warnings,
		"assumptions": result.Assumptions,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// executeAndRecord runs the command and records the execution in history.
func executeAndRecord(ctx context.Context, result *commands.Result, cfg *config.Config, store *history.Store, histID int64) error {
	runOpts := runner.Options{
		Timeout:        cfg.ExecutionTimeout,
		MaxOutputBytes: cfg.OutputMaxBytes,
	}
	if runOpts.Timeout == 0 {
		runOpts.Timeout = 30 * time.Second
	}
	if runOpts.MaxOutputBytes == 0 {
		runOpts.MaxOutputBytes = 512 * 1024
	}

	fmt.Printf("\nRunning: %s\n", result.Display)
	fmt.Println(strings.Repeat("─", 60))

	runResult, err := runner.Run(ctx, result.Argv, runOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nExecution error: %v\n", err)
		return err
	}

	// Print output.
	if runResult.Stdout != "" {
		fmt.Print(runResult.Stdout)
	}
	if runResult.Stderr != "" {
		fmt.Fprintf(os.Stderr, "\n[stderr]\n%s", runResult.Stderr)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("  Exit code: %d | Duration: %s", runResult.ExitCode, runResult.Duration.Round(time.Millisecond))
	if runResult.Truncated {
		fmt.Print(" | [output truncated]")
	}
	if runResult.TimedOut {
		fmt.Print(" | [timed out]")
	}
	fmt.Println()

	// Update history record with execution result.
	if store != nil && histID > 0 {
		_ = store.UpdateExecution(
			histID,
			runResult.ExitCode,
			runResult.Duration.Milliseconds(),
			runResult.Stdout,
			runResult.Stderr,
		)
	}

	return nil
}

// Execute is the main entry point called from main.go.
func Execute() error {
	return NewRootCmd().Execute()
}
