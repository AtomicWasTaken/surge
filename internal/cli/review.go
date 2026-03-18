package cli

import (
	"fmt"

	"github.com/AtomicWasTaken/surge/internal/ai"
	"github.com/AtomicWasTaken/surge/internal/config"
	"github.com/AtomicWasTaken/surge/internal/github"
	"github.com/AtomicWasTaken/surge/internal/review"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func runReview(cmd *cobra.Command, args []string) error {
	// Bind flags to viper for config override
	bindings := map[string]string{
		"github-token": "github.token",
		"owner":        "github.owner",
		"repo":         "github.repo",
		"pr":           "github.prNumber",
		"ai-provider":  "ai.provider",
		"ai-model":     "ai.model",
		"ai-base-url":  "ai.baseUrl",
		"ai-api-key":   "ai.apiKey",
		"context-depth": "contextDepth",
		"output":       "output.format",
		"max-inline":   "maxInlineComments",
		"max-tokens":   "maxTokens",
		"temperature":  "temperature",
	}
	for flag, key := range bindings {
		if cmd.Flags().Changed(flag) {
			viper.Set(key, cmd.Flag(flag).Value.String())
		}
	}

	// Load config
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Apply explicit flag overrides
	if cmd.Flags().Changed("verbose") {
		cfg.Output.Colorize = flagVerbose
	}
	_ = cmd // mark as used

	// Apply --no-inline and --no-summary flags
	if flagNoInline {
		cfg.DisableInlineComments = true
	}
	if flagNoSummary {
		cfg.DisableSummaryComment = true
	}

	// Detect owner/repo from git if not set
	owner := cfg.GitHub.Owner
	repo := cfg.GitHub.Repo
	if owner == "" || repo == "" {
		detectedOwner, detectedRepo, err := detectGitInfo()
		if err != nil {
			return fmt.Errorf("could not detect owner/repo: %w (use --owner and --repo flags, or set in config)", err)
		}
		if owner == "" {
			owner = detectedOwner
		}
		if repo == "" {
			repo = detectedRepo
		}
	}

	// Get PR number
	prNumber := cfg.GitHub.PRNumber
	if prNumber <= 0 {
		return fmt.Errorf("PR number is required (use --pr flag or set github.prNumber in config)")
	}

	// Check for required tokens
	if cfg.GitHub.Token == "" {
		return fmt.Errorf("GitHub token is required (set SURGE_GITHUB_TOKEN env var, or use --github-token flag)")
	}
	if cfg.AI.APIKey == "" && cfg.AI.Provider == "claude" {
		return fmt.Errorf("AI API key is required (set SURGE_AI_API_KEY env var, or use --ai-api-key flag)")
	}

	// Create AI client
	var aiClient ai.AIClient
	switch cfg.AI.Provider {
	case "litellm":
		aiClient = ai.NewLiteLLMClient(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model)
	case "claude":
		aiClient = ai.NewClaudeClient(cfg.AI.APIKey, cfg.AI.Model)
	default:
		return fmt.Errorf("unknown AI provider: %s", cfg.AI.Provider)
	}

	// Create GitHub client
	ghClient := github.NewGitHubClient(cfg.GitHub.Token)

	// Create orchestrator
	orch := review.NewOrchestrator(aiClient, ghClient, cfg)

	// Run the review
	result, err := orch.Review(cmd.Context(), owner, repo, prNumber, flagDryRun)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	// Print final status
	if flagDryRun {
		fmt.Println("\n[DRY RUN] No changes posted to the PR.")
	} else {
		fmt.Printf("\nReview posted to %s/%s#%d\n", owner, repo, prNumber)
	}

	// Exit code based on approval
	if !result.Approve && !flagDryRun {
		return nil // Don't exit with error, just report
	}

	return nil
}
