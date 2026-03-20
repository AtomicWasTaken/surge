package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestLoadReadConfigErrorAndEnvExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("{"), 0644))

	_, err := Load(configPath)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read config")

	assert.NoError(t, os.Setenv("BASE_URL_ENV", "http://example.com"))
	defer func() { _ = os.Unsetenv("BASE_URL_ENV") }()

	cfgPath := filepath.Join(tmpDir, "expanded.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
ai:
  provider: litellm
  baseUrl: "${BASE_URL_ENV}"
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
	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "http://example.com", cfg.AI.BaseURL)
}

func TestBindEnvironmentErrorAggregation(t *testing.T) {
	prev := bindEnv
	t.Cleanup(func() { bindEnv = prev })
	bindEnv = func(v *viper.Viper, key string, envs ...string) error {
		if key == "github.token" {
			return errors.New("bind failed")
		}
		return nil
	}

	err := bindEnvironment(viper.New())
	require.Error(t, err)
	assert.ErrorContains(t, err, "github.token -> SURGE_GITHUB_TOKEN: bind failed")
}

func TestLoadUnmarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
ai: "bad"
`), 0644))

	_, err := Load(configPath)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to unmarshal config")
}

func TestLoadMissingExplicitConfigPath(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read config")
}

func TestLoadBindEnvironmentError(t *testing.T) {
	prev := bindEnv
	t.Cleanup(func() { bindEnv = prev })
	bindEnv = func(v *viper.Viper, key string, envs ...string) error {
		return errors.New("bind failed")
	}

	_, err := Load("")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to bind environment variables")
}
