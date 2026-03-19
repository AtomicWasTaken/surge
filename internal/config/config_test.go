package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	assert.NoError(t, err)
	assert.Equal(t, "litellm", cfg.AI.Provider)
	assert.Equal(t, "diff-only", cfg.ContextDepth)
	assert.Equal(t, 8192, cfg.MaxTokens)
	assert.Equal(t, 0.3, cfg.Temperature)
	assert.True(t, cfg.Categories.Security)
	assert.True(t, cfg.Categories.Performance)
	assert.True(t, cfg.Categories.Logic)
	assert.True(t, cfg.Categories.Maintainability)
	assert.True(t, cfg.Categories.Vibe)
	assert.Equal(t, 20, cfg.MaxInlineComments)
}

func TestLoad_EnvOverride(t *testing.T) {
	assert.NoError(t, os.Setenv("SURGE_GITHUB_TOKEN", "test-token"))
	assert.NoError(t, os.Setenv("SURGE_AI_API_KEY", "test-api-key"))
	defer func() {
		assert.NoError(t, os.Unsetenv("SURGE_GITHUB_TOKEN"))
		assert.NoError(t, os.Unsetenv("SURGE_AI_API_KEY"))
	}()

	cfg, err := Load("")
	assert.NoError(t, err)
	assert.Equal(t, "test-token", cfg.GitHub.Token)
	assert.Equal(t, "test-api-key", cfg.AI.APIKey)
}

func TestLoad_ConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "surge.yaml")
	err := os.WriteFile(configPath, []byte(`
ai:
  provider: claude
  model: claude-3-5-sonnet
contextDepth: relevant
maxInlineComments: 10
`), 0644)
	assert.NoError(t, err)

	cfg, err := Load(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "claude", cfg.AI.Provider)
	assert.Equal(t, "claude-3-5-sonnet", cfg.AI.Model)
	assert.Equal(t, "relevant", cfg.ContextDepth)
	assert.Equal(t, 10, cfg.MaxInlineComments)
}

func TestConfig_Validate(t *testing.T) {
	cfg := &Config{
		AI:           AIConfig{Provider: "litellm"},
		ContextDepth: "diff-only",
		Output:       OutputConfig{Format: "terminal"},
		Categories: CategoriesConfig{
			Security: true,
		},
	}
	assert.NoError(t, cfg.Validate())

	cfg.AI.Provider = "invalid"
	assert.Error(t, cfg.Validate())

	cfg.AI.Provider = "litellm"
	cfg.ContextDepth = "invalid"
	assert.Error(t, cfg.Validate())

	cfg.ContextDepth = "diff-only"
	cfg.Output.Format = "invalid"
	assert.Error(t, cfg.Validate())

	cfg.Output.Format = "terminal"
	cfg.Categories = CategoriesConfig{}
	assert.EqualError(t, cfg.Validate(), "at least one review category must be enabled")
}

func TestExpandEnvVar(t *testing.T) {
	assert.NoError(t, os.Setenv("TEST_VAR", "test-value"))
	defer func() {
		assert.NoError(t, os.Unsetenv("TEST_VAR"))
	}()

	result := expandEnv("${TEST_VAR}")
	assert.Equal(t, "test-value", result)

	result = expandEnv("plain-string")
	assert.Equal(t, "plain-string", result)

	result = expandEnv("${UNSET_VAR}")
	assert.Equal(t, "${UNSET_VAR}", result)
}
