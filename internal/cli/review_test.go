package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/AtomicWasTaken/surge/internal/ai"
	"github.com/AtomicWasTaken/surge/internal/config"
	"github.com/AtomicWasTaken/surge/internal/github"
	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyPRClient struct{}

func (d *dummyPRClient) GetPR(ctx context.Context, owner, repo string, prNumber int) (*model.PR, error) {
	return nil, nil
}
func (d *dummyPRClient) GetDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	return "", nil
}
func (d *dummyPRClient) GetFiles(ctx context.Context, owner, repo string, prNumber int) ([]model.FileChange, error) {
	return nil, nil
}
func (d *dummyPRClient) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return "", nil
}
func (d *dummyPRClient) PostReview(ctx context.Context, owner, repo string, prNumber int, review *model.ReviewInput) error {
	return nil
}
func (d *dummyPRClient) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	return nil
}
func (d *dummyPRClient) ListComments(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRComment, error) {
	return nil, nil
}
func (d *dummyPRClient) DeleteComment(ctx context.Context, owner, repo string, commentID int64) error {
	return nil
}
func (d *dummyPRClient) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRReview, error) {
	return nil, nil
}
func (d *dummyPRClient) DeleteReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64) error {
	return nil
}
func (d *dummyPRClient) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	return nil
}
func (d *dummyPRClient) ListReviewComments(ctx context.Context, owner, repo string, prNumber int, reviewID int64) ([]*model.PRReviewComment, error) {
	return nil, nil
}
func (d *dummyPRClient) DeleteReviewComment(ctx context.Context, owner, repo string, commentID int64) error {
	return nil
}
func (d *dummyPRClient) ListLabels(ctx context.Context, owner, repo string, prNumber int) ([]string, error) {
	return nil, nil
}
func (d *dummyPRClient) AddLabels(ctx context.Context, owner, repo string, prNumber int, labels []string) error {
	return nil
}
func (d *dummyPRClient) RemoveLabel(ctx context.Context, owner, repo string, prNumber int, label string) error {
	return nil
}
func (d *dummyPRClient) UpsertLabel(ctx context.Context, owner, repo, name, color, description string) error {
	return nil
}

type stubReviewExecutor struct {
	result *model.ReviewResult
	err    error
}

func (s *stubReviewExecutor) Review(ctx context.Context, owner, repo string, prNumber int, dryRun bool) (*model.ReviewResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestApplyReviewFlagOverrides(t *testing.T) {
	cmd := &cobra.Command{Use: "review"}
	cmd.Flags().String("github-token", "", "")
	cmd.Flags().String("owner", "", "")
	cmd.Flags().String("repo", "", "")
	cmd.Flags().Int("pr", 0, "")
	cmd.Flags().String("ai-provider", "", "")
	cmd.Flags().String("ai-model", "", "")
	cmd.Flags().String("ai-base-url", "", "")
	cmd.Flags().String("ai-api-key", "", "")
	cmd.Flags().String("context-depth", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Int("max-inline", 0, "")
	cmd.Flags().Int("max-tokens", 0, "")
	cmd.Flags().Float64("temperature", 0, "")

	assert.NoError(t, cmd.Flags().Set("github-token", "gh-token"))
	assert.NoError(t, cmd.Flags().Set("owner", "octo"))
	assert.NoError(t, cmd.Flags().Set("repo", "surge"))
	assert.NoError(t, cmd.Flags().Set("pr", "12"))
	assert.NoError(t, cmd.Flags().Set("ai-provider", "litellm"))
	assert.NoError(t, cmd.Flags().Set("ai-model", "model-x"))
	assert.NoError(t, cmd.Flags().Set("ai-base-url", "https://litellm.example"))
	assert.NoError(t, cmd.Flags().Set("ai-api-key", "ai-key"))
	assert.NoError(t, cmd.Flags().Set("context-depth", "relevant"))
	assert.NoError(t, cmd.Flags().Set("output", "json"))
	assert.NoError(t, cmd.Flags().Set("max-inline", "7"))
	assert.NoError(t, cmd.Flags().Set("max-tokens", "4096"))
	assert.NoError(t, cmd.Flags().Set("temperature", "0.6"))

	cfg := &config.Config{}
	applyReviewFlagOverrides(cmd, cfg)

	assert.Equal(t, "gh-token", cfg.GitHub.Token)
	assert.Equal(t, "octo", cfg.GitHub.Owner)
	assert.Equal(t, "surge", cfg.GitHub.Repo)
	assert.Equal(t, 12, cfg.GitHub.PRNumber)
	assert.Equal(t, "litellm", cfg.AI.Provider)
	assert.Equal(t, "model-x", cfg.AI.Model)
	assert.Equal(t, "https://litellm.example", cfg.AI.BaseURL)
	assert.Equal(t, "ai-key", cfg.AI.APIKey)
	assert.Equal(t, "relevant", cfg.ContextDepth)
	assert.Equal(t, "json", cfg.Output.Format)
	assert.Equal(t, 7, cfg.MaxInlineComments)
	assert.Equal(t, 4096, cfg.MaxTokens)
	assert.Equal(t, 0.6, cfg.Temperature)
}

func TestApplyReviewBooleanOverrides(t *testing.T) {
	prevVerbose := flagVerbose
	prevNoInline := flagNoInline
	prevNoSummary := flagNoSummary
	t.Cleanup(func() {
		flagVerbose = prevVerbose
		flagNoInline = prevNoInline
		flagNoSummary = prevNoSummary
	})

	cmd := &cobra.Command{Use: "review"}
	cmd.Flags().Bool("verbose", false, "")
	require.NoError(t, cmd.Flags().Set("verbose", "true"))

	flagVerbose = true
	flagNoInline = true
	flagNoSummary = true

	cfg := &config.Config{}
	applyReviewBooleanOverrides(cmd, cfg)

	assert.True(t, cfg.Verbose)
	assert.True(t, cfg.DisableInlineComments)
	assert.True(t, cfg.DisableSummaryComment)
}

func TestResolveReviewTarget(t *testing.T) {
	t.Run("uses config when complete", func(t *testing.T) {
		owner, repo, pr, err := resolveReviewTarget(&config.Config{
			GitHub: config.GitHubConfig{Owner: "octo", Repo: "surge", PRNumber: 12},
		})
		require.NoError(t, err)
		assert.Equal(t, "octo", owner)
		assert.Equal(t, "surge", repo)
		assert.Equal(t, 12, pr)
	})

	t.Run("errors when pr missing", func(t *testing.T) {
		_, _, _, err := resolveReviewTarget(&config.Config{
			GitHub: config.GitHubConfig{Owner: "octo", Repo: "surge"},
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "PR number is required")
	})

	t.Run("fills missing owner from detection", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		require.NoError(t, os.Mkdir(gitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[remote \"origin\"]\n\turl = https://github.com/octo/surge.git\n"), 0644))
		prevWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })

		owner, repo, pr, err := resolveReviewTarget(&config.Config{
			GitHub: config.GitHubConfig{Repo: "custom", PRNumber: 7},
		})
		require.NoError(t, err)
		assert.Equal(t, "octo", owner)
		assert.Equal(t, "custom", repo)
		assert.Equal(t, 7, pr)
	})

	t.Run("fills missing repo from detection", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		require.NoError(t, os.Mkdir(gitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[remote \"origin\"]\n\turl = https://github.com/octo/surge.git\n"), 0644))
		prevWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })

		owner, repo, pr, err := resolveReviewTarget(&config.Config{
			GitHub: config.GitHubConfig{Owner: "custom", PRNumber: 7},
		})
		require.NoError(t, err)
		assert.Equal(t, "custom", owner)
		assert.Equal(t, "surge", repo)
		assert.Equal(t, 7, pr)
	})

	t.Run("returns detection error when git info lookup fails", func(t *testing.T) {
		prevGetwd := getwd
		getwd = func() (string, error) {
			return "", errors.New("boom")
		}
		t.Cleanup(func() {
			getwd = prevGetwd
		})

		_, _, _, err := resolveReviewTarget(&config.Config{
			GitHub: config.GitHubConfig{PRNumber: 1},
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "could not detect owner/repo")
		assert.ErrorContains(t, err, "boom")
	})
}

func TestValidateReviewCredentials(t *testing.T) {
	require.Error(t, validateReviewCredentials(&config.Config{}))
	require.Error(t, validateReviewCredentials(&config.Config{
		GitHub: config.GitHubConfig{Token: "gh"},
		AI:     config.AIConfig{Provider: "claude"},
	}))
	require.NoError(t, validateReviewCredentials(&config.Config{
		GitHub: config.GitHubConfig{Token: "gh"},
		AI:     config.AIConfig{Provider: "litellm"},
	}))
}

func TestNewAIClient(t *testing.T) {
	client, err := newAIClient(&config.Config{
		AI: config.AIConfig{Provider: "litellm", BaseURL: "http://localhost:4000", APIKey: "key", Model: "m"},
	})
	require.NoError(t, err)
	assert.Equal(t, "litellm", client.Name())

	client, err = newAIClient(&config.Config{
		AI: config.AIConfig{Provider: "claude", APIKey: "key", Model: "claude-3-7-sonnet"},
	})
	require.NoError(t, err)
	assert.IsType(t, &ai.ClaudeClient{}, client)

	_, err = newAIClient(&config.Config{AI: config.AIConfig{Provider: "unknown"}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown AI provider")
}

func TestLoadReviewConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
github:
  token: gh
  owner: octo
  repo: surge
  prNumber: 15
ai:
  provider: litellm
  model: test-model
  baseUrl: http://localhost:4000
output:
  format: markdown
contextDepth: diff-only
categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true
`), 0644))

	prevConfig := flagConfig
	prevVerbose := flagVerbose
	prevNoInline := flagNoInline
	prevNoSummary := flagNoSummary
	t.Cleanup(func() {
		flagConfig = prevConfig
		flagVerbose = prevVerbose
		flagNoInline = prevNoInline
		flagNoSummary = prevNoSummary
	})

	flagConfig = cfgPath
	flagVerbose = false
	flagNoInline = false
	flagNoSummary = false

	cmd := &cobra.Command{Use: "review"}
	cmd.Flags().String("github-token", "", "")
	cmd.Flags().String("owner", "", "")
	cmd.Flags().String("repo", "", "")
	cmd.Flags().Int("pr", 0, "")
	cmd.Flags().String("ai-provider", "", "")
	cmd.Flags().String("ai-model", "", "")
	cmd.Flags().String("ai-base-url", "", "")
	cmd.Flags().String("ai-api-key", "", "")
	cmd.Flags().String("context-depth", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Int("max-inline", 0, "")
	cmd.Flags().Int("max-tokens", 0, "")
	cmd.Flags().Float64("temperature", 0, "")
	cmd.Flags().Bool("verbose", false, "")

	require.NoError(t, cmd.Flags().Set("owner", "override"))

	cfg, err := loadReviewConfig(cmd)
	require.NoError(t, err)
	assert.Equal(t, "override", cfg.GitHub.Owner)
	assert.Equal(t, "surge", cfg.GitHub.Repo)
	assert.Equal(t, 15, cfg.GitHub.PRNumber)
}

func TestRunReviewEarlyErrors(t *testing.T) {
	cmd := &cobra.Command{Use: "review"}

	prevConfig := flagConfig
	t.Cleanup(func() { flagConfig = prevConfig })

	flagConfig = filepath.Join(t.TempDir(), "missing.yaml")
	err := runReview(cmd, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to load config")
}

func TestLoadReviewConfigInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
ai:
  provider: invalid
categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true
output:
  format: terminal
contextDepth: diff-only
`), 0644))

	prevConfig := flagConfig
	t.Cleanup(func() { flagConfig = prevConfig })
	flagConfig = cfgPath

	cmd := &cobra.Command{Use: "review"}
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().String("github-token", "", "")
	cmd.Flags().String("owner", "", "")
	cmd.Flags().String("repo", "", "")
	cmd.Flags().Int("pr", 0, "")
	cmd.Flags().String("ai-provider", "", "")
	cmd.Flags().String("ai-model", "", "")
	cmd.Flags().String("ai-base-url", "", "")
	cmd.Flags().String("ai-api-key", "", "")
	cmd.Flags().String("context-depth", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Int("max-inline", 0, "")
	cmd.Flags().Int("max-tokens", 0, "")
	cmd.Flags().Float64("temperature", 0, "")

	_, err := loadReviewConfig(cmd)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid config")
}

func TestNewAIClientNames(t *testing.T) {
	litellmClient, err := newAIClient(&config.Config{
		AI: config.AIConfig{Provider: "litellm", BaseURL: "http://localhost:4000", APIKey: "key", Model: "m"},
	})
	require.NoError(t, err)
	assert.Equal(t, "litellm", litellmClient.Name())
}

func TestRunReviewPrintsVerboseBeforeCredentialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
github:
  owner: octo
  repo: surge
  prNumber: 5
ai:
  provider: litellm
  model: test-model
output:
  format: terminal
contextDepth: diff-only
categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true
verbose: true
`), 0644))

	prevConfig := flagConfig
	t.Cleanup(func() { flagConfig = prevConfig })
	flagConfig = cfgPath

	cmd := &cobra.Command{Use: "review"}
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().String("github-token", "", "")
	cmd.Flags().String("owner", "", "")
	cmd.Flags().String("repo", "", "")
	cmd.Flags().Int("pr", 0, "")
	cmd.Flags().String("ai-provider", "", "")
	cmd.Flags().String("ai-model", "", "")
	cmd.Flags().String("ai-base-url", "", "")
	cmd.Flags().String("ai-api-key", "", "")
	cmd.Flags().String("context-depth", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Int("max-inline", 0, "")
	cmd.Flags().Int("max-tokens", 0, "")
	cmd.Flags().Float64("temperature", 0, "")

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	err = runReview(cmd, nil)

	require.NoError(t, w.Close())
	os.Stdout = stdout
	require.Error(t, err)
	assert.ErrorContains(t, err, "GitHub token is required")

	var buf bytes.Buffer
	_, readErr := io.Copy(&buf, r)
	require.NoError(t, readErr)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "[debug] review config owner=octo repo=surge pr=5")
}

func TestRunReviewSuccessAndReviewFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
github:
  token: gh
  owner: octo
  repo: surge
  prNumber: 5
ai:
  provider: litellm
  model: test-model
  baseUrl: http://localhost:4000
output:
  format: terminal
contextDepth: diff-only
categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true
`), 0644))

	prevConfig := flagConfig
	prevDryRun := flagDryRun
	prevGitHubFactory := newGitHubPRClient
	prevExecFactory := newReviewExecutor
	prevBuildAIClient := buildAIClient
	t.Cleanup(func() {
		flagConfig = prevConfig
		flagDryRun = prevDryRun
		newGitHubPRClient = prevGitHubFactory
		newReviewExecutor = prevExecFactory
		buildAIClient = prevBuildAIClient
	})
	flagConfig = cfgPath
	newGitHubPRClient = func(token string) github.PRClient { return &dummyPRClient{} }

	makeCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "review"}
		cmd.Flags().Bool("verbose", false, "")
		cmd.Flags().String("github-token", "", "")
		cmd.Flags().String("owner", "", "")
		cmd.Flags().String("repo", "", "")
		cmd.Flags().Int("pr", 0, "")
		cmd.Flags().String("ai-provider", "", "")
		cmd.Flags().String("ai-model", "", "")
		cmd.Flags().String("ai-base-url", "", "")
		cmd.Flags().String("ai-api-key", "", "")
		cmd.Flags().String("context-depth", "", "")
		cmd.Flags().String("output", "", "")
		cmd.Flags().Int("max-inline", 0, "")
		cmd.Flags().Int("max-tokens", 0, "")
		cmd.Flags().Float64("temperature", 0, "")
		return cmd
	}

	t.Run("dry run success", func(t *testing.T) {
		flagDryRun = true
		newReviewExecutor = func(aiClient ai.AIClient, ghClient github.PRClient, cfg *config.Config) reviewExecutor {
			return &stubReviewExecutor{result: &model.ReviewResult{Approve: true}}
		}

		stdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		err = runReview(makeCmd(), nil)

		require.NoError(t, w.Close())
		os.Stdout = stdout
		require.NoError(t, err)

		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Contains(t, buf.String(), "[DRY RUN] No changes posted to the PR.")
	})

	t.Run("post success", func(t *testing.T) {
		flagDryRun = false
		newReviewExecutor = func(aiClient ai.AIClient, ghClient github.PRClient, cfg *config.Config) reviewExecutor {
			return &stubReviewExecutor{result: &model.ReviewResult{Approve: false}}
		}

		stdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		err = runReview(makeCmd(), nil)

		require.NoError(t, w.Close())
		os.Stdout = stdout
		require.NoError(t, err)

		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Contains(t, buf.String(), "Review posted to octo/surge#5")
	})

	t.Run("review failure", func(t *testing.T) {
		newReviewExecutor = func(aiClient ai.AIClient, ghClient github.PRClient, cfg *config.Config) reviewExecutor {
			return &stubReviewExecutor{err: errors.New("boom")}
		}

		err := runReview(makeCmd(), nil)
		require.Error(t, err)
		assert.ErrorContains(t, err, "review failed: boom")
	})
}

func TestRunReviewAdditionalErrorBranches(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
github:
  token: gh
  prNumber: 5
ai:
  provider: litellm
  model: test-model
  baseUrl: http://localhost:4000
output:
  format: terminal
contextDepth: diff-only
categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true
`), 0644))

	prevConfig := flagConfig
	prevBuildAIClient := buildAIClient
	t.Cleanup(func() {
		flagConfig = prevConfig
		buildAIClient = prevBuildAIClient
	})
	flagConfig = cfgPath

	makeCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "review"}
		cmd.Flags().Bool("verbose", false, "")
		cmd.Flags().String("github-token", "", "")
		cmd.Flags().String("owner", "", "")
		cmd.Flags().String("repo", "", "")
		cmd.Flags().Int("pr", 0, "")
		cmd.Flags().String("ai-provider", "", "")
		cmd.Flags().String("ai-model", "", "")
		cmd.Flags().String("ai-base-url", "", "")
		cmd.Flags().String("ai-api-key", "", "")
		cmd.Flags().String("context-depth", "", "")
		cmd.Flags().String("output", "", "")
		cmd.Flags().Int("max-inline", 0, "")
		cmd.Flags().Int("max-tokens", 0, "")
		cmd.Flags().Float64("temperature", 0, "")
		return cmd
	}

	t.Run("resolve target error", func(t *testing.T) {
		prevWD, chErr := os.Getwd()
		require.NoError(t, chErr)
		require.NoError(t, os.Chdir(t.TempDir()))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })
		t.Setenv("GITHUB_REPOSITORY", "")

		err := runReview(makeCmd(), nil)
		require.Error(t, err)
		assert.ErrorContains(t, err, "could not detect owner/repo")
	})

	t.Run("build ai client error", func(t *testing.T) {
		require.NoError(t, os.WriteFile(cfgPath, []byte(`
github:
  token: gh
  owner: octo
  repo: surge
  prNumber: 5
ai:
  provider: litellm
  model: test-model
  baseUrl: http://localhost:4000
output:
  format: terminal
contextDepth: diff-only
categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true
`), 0644))
		buildAIClient = func(cfg *config.Config) (ai.AIClient, error) {
			return nil, errors.New("boom")
		}
		err := runReview(makeCmd(), nil)
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	})
}

func TestDefaultFactories(t *testing.T) {
	client := defaultGitHubPRClient("token")
	require.NotNil(t, client)
	exec := defaultReviewExecutor(nil, client, &config.Config{CommentMarker: "SURGE"})
	require.NotNil(t, exec)
}
