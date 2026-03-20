package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AtomicWasTaken/surge/internal/config"
	"github.com/spf13/cobra"
)

// versionInfo holds the version metadata injected at build time.
var versionInfo = struct {
	Version string
	Commit  string
	Date    string
}{"dev", "", ""}

var (
	rootCmd = &cobra.Command{
		Use:   "surge",
		Short: "surge - AI-powered PR code review",
		Long: `surge is an AI-powered code review tool for pull requests.

It analyzes your PR diffs and provides structured feedback on security,
performance, logic, code quality, and "vibe codability" (detecting
AI-generated code that's technically correct but soulless).

Example:
  surge review --pr 123
  surge review --config surge.yaml --dry-run
  surge config init`,
	}
	reviewCmd = &cobra.Command{
		Use:   "review",
		Short: "Run a code review on a pull request",
		Long:  "Fetches the PR diff, runs AI analysis, and posts results as a review comment.",
		RunE:  runReview,
	}
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Manage surge configuration",
	}
	configInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize a surge.yaml config file",
		Long:  "Creates a surge.yaml file in the current directory with default settings.",
		RunE:  runConfigInit,
	}
	configValidateCmd = &cobra.Command{
		Use:   "validate",
		Short: "Validate the surge.yaml config file",
		RunE:  runConfigValidate,
	}
	diffCmd = &cobra.Command{
		Use:   "diff",
		Short: "Fetch and display the diff for a PR",
		Long:  "Fetches the diff for a PR without running a review. Useful for debugging.",
		RunE:  runDiff,
	}
)

var (
	flagConfig    string
	flagDryRun    bool
	flagVerbose   bool
	flagNoInline  bool
	flagNoSummary bool
	getwd         = os.Getwd
)

func Execute() error {
	return rootCmd.Execute()
}

func SetVersion(version, commit, date string) {
	versionInfo.Version = version
	versionInfo.Commit = commit
	versionInfo.Date = date
	rootCmd.Version = version
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.SetVersionTemplate(fmt.Sprintf("surge %s (commit: %s, date: %s)\n",
		versionInfo.Version, versionInfo.Commit, versionInfo.Date))

	// Global flags
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Print output without posting to PR")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose debug output")

	// Review flags
	reviewCmd.Flags().String("github-token", "", "GitHub personal access token (or set SURGE_GITHUB_TOKEN)")
	reviewCmd.Flags().String("owner", "", "Repository owner (auto-detected from git)")
	reviewCmd.Flags().String("repo", "", "Repository name (auto-detected from git)")
	reviewCmd.Flags().Int("pr", 0, "PR number")
	reviewCmd.Flags().String("ai-provider", "", "AI provider: litellm or claude")
	reviewCmd.Flags().String("ai-model", "", "AI model name")
	reviewCmd.Flags().String("ai-base-url", "", "litellm proxy base URL")
	reviewCmd.Flags().String("ai-api-key", "", "AI API key")
	reviewCmd.Flags().String("context-depth", "", "Context depth: diff-only, relevant, or full")
	reviewCmd.Flags().StringP("output", "o", "", "Output format: terminal, markdown, json")
	reviewCmd.Flags().BoolVar(&flagNoInline, "no-inline", false, "Skip inline diff comments")
	reviewCmd.Flags().BoolVar(&flagNoSummary, "no-summary", false, "Skip summary comment")
	reviewCmd.Flags().Int("max-inline", 0, "Maximum number of inline comments")
	reviewCmd.Flags().Int("max-tokens", 0, "Maximum tokens for AI response")
	reviewCmd.Flags().Float64("temperature", 0, "AI temperature (0.0-1.0)")

	// Diff flags
	diffCmd.Flags().String("github-token", "", "GitHub personal access token")
	diffCmd.Flags().String("owner", "", "Repository owner")
	diffCmd.Flags().String("repo", "", "Repository name")
	diffCmd.Flags().Int("pr", 0, "PR number")
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	path := "surge.yaml"
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("surge.yaml already exists in %s", cwd())
	}

	exampleConfig := `ai:
  # provider: litellm  # or "claude"
  # model: anthropic/claude-3-5-sonnet-20241022
  # baseUrl: http://localhost:4000  # litellm proxy URL
  # apiKey: "${LITELLM_API_KEY}"  # set via env var

contextDepth: diff-only  # diff-only | relevant | full

output:
  format: terminal  # terminal | markdown | json
  showStats: true

categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true

maxInlineComments: 20
maxTokens: 8192
temperature: 0.3

excludePaths:
  - "*.generated.go"
  - "vendor/**"
  - "**/_test.go"
`

	if err := os.WriteFile(path, []byte(exampleConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	fmt.Printf("Created %s\n", path)
	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}
	fmt.Println("Config is valid")
	return nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return err
	}

	// TODO: Implement diff fetching
	_ = cfg
	fmt.Println("Diff command not yet implemented")
	return nil
}

func cwd() string {
	dir, _ := getwd()
	return dir
}

// detectGitInfo attempts to detect owner/repo from the local git config.
func detectGitInfo() (owner, repo string, err error) {
	dir, err := getwd()
	if err != nil {
		return "", "", err
	}

	for {
		gitConfigPath := filepath.Join(dir, ".git", "config")
		data, err := os.ReadFile(gitConfigPath)
		if err == nil {
			content := string(data)
			for _, line := range strings.Split(content, "\n") {
				if strings.HasPrefix(line, "\turl = ") {
					url := strings.TrimPrefix(line, "\turl = ")
					if strings.Contains(url, "@") {
						url = strings.Split(url, ":")[1]
						url = strings.TrimSuffix(url, ".git")
						parts := strings.Split(url, "/")
						if len(parts) == 2 {
							return parts[0], parts[1], nil
						}
					}
					if strings.Contains(url, "github.com") {
						url = strings.TrimPrefix(url, "https://github.com/")
						url = strings.TrimPrefix(url, "http://github.com/")
						url = strings.TrimSuffix(url, ".git")
						parts := strings.Split(url, "/")
						if len(parts) == 2 {
							return parts[0], parts[1], nil
						}
					}
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if owner = os.Getenv("GITHUB_REPOSITORY"); owner != "" {
		parts := strings.Split(owner, "/")
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("could not detect git owner/repo from .git/config or GITHUB_REPOSITORY")
}
