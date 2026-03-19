package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	defaultAIProvider        = "litellm"
	defaultAIModel           = "claude-sonnet-4-6"
	defaultAIBaseURL         = "http://localhost:4000"
	defaultContextDepth      = "diff-only"
	defaultOutputFormat      = "terminal"
	defaultOutputColorize    = true
	defaultOutputShowStats   = false
	defaultMaxTokens         = 8192
	defaultTemperature       = 0.3
	defaultMaxInlineComments = 20
	defaultDisableInline     = false
	defaultDisableSummary    = false
	defaultCommentMarker     = "SURGE"
	defaultEnablePRLabels    = true
	defaultPRLabelPrefix     = "surge"
)

var (
	validAIProviders  = []string{"litellm", "claude"}
	validContextDepth = []string{"diff-only", "relevant", "full"}
	validOutputFormat = []string{"terminal", "markdown", "json"}
)

type envBinding struct {
	key string
	env string
}

// Config represents the full surge configuration.
type Config struct {
	AI           AIConfig         `mapstructure:"ai"`
	ContextDepth string           `mapstructure:"contextDepth"`
	Output       OutputConfig     `mapstructure:"output"`
	Categories   CategoriesConfig `mapstructure:"categories"`
	GitHub       GitHubConfig     `mapstructure:"github"`
	Verbose      bool             `mapstructure:"verbose"`

	// Inline comment settings
	MaxInlineComments     int  `mapstructure:"maxInlineComments"`
	DisableInlineComments bool `mapstructure:"disableInlineComments"`
	DisableSummaryComment bool `mapstructure:"disableSummaryComment"`

	// Model settings
	MaxTokens   int     `mapstructure:"maxTokens"`
	Temperature float64 `mapstructure:"temperature"`

	// Filtering
	ExcludePaths []string `mapstructure:"excludePaths"`
	IncludePaths []string `mapstructure:"includePaths"`
	MinSeverity  string   `mapstructure:"minSeverity"`

	// Comment marker
	CommentMarker string `mapstructure:"commentMarker"`

	// PR labeling
	EnablePRLabels bool   `mapstructure:"enablePRLabels"`
	PRLabelPrefix  string `mapstructure:"prLabelPrefix"`
}

// AIConfig configures the AI provider.
type AIConfig struct {
	Provider string `mapstructure:"provider"` // "litellm" or "claude"
	Model    string `mapstructure:"model"`
	BaseURL  string `mapstructure:"baseUrl"`
	APIKey   string `mapstructure:"apiKey"`
}

// OutputConfig configures output formatting.
type OutputConfig struct {
	Format    string `mapstructure:"format"`    // "markdown", "json", "terminal"
	Colorize  bool   `mapstructure:"colorize"`  // Terminal colors
	ShowStats bool   `mapstructure:"showStats"` // Token usage, timing
}

// CategoriesConfig enables/disables review categories.
type CategoriesConfig struct {
	Security        bool `mapstructure:"security"`
	Performance     bool `mapstructure:"performance"`
	Logic           bool `mapstructure:"logic"`
	Maintainability bool `mapstructure:"maintainability"`
	Vibe            bool `mapstructure:"vibe"`
}

// GitHubConfig holds GitHub-specific settings.
type GitHubConfig struct {
	Token    string `mapstructure:"token"` // Loaded from env
	Owner    string `mapstructure:"owner"`
	Repo     string `mapstructure:"repo"`
	PRNumber int    `mapstructure:"prNumber"`
}

// Load reads the configuration from file, environment, and flags.
// Precedence: CLI flags > env vars > config file > defaults.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	configureConfigSources(v, configPath)

	v.SetEnvPrefix("SURGE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := bindEnvironment(v); err != nil {
		return nil, fmt.Errorf("failed to bind environment variables: %w", err)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	applyDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.expandEnvVars()
	cfg.applyExplicitEnvOverrides()

	return &cfg, nil
}

func configureConfigSources(v *viper.Viper, configPath string) {
	if configPath != "" {
		v.SetConfigFile(configPath)
		return
	}

	v.SetConfigName("surge")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath(filepath.Join(os.Getenv("HOME"), ".config", "surge"))
	v.AddConfigPath("/etc/surge")
}

func bindEnvironment(v *viper.Viper) error {
	var errs []error
	for _, binding := range []envBinding{
		{key: "github.token", env: "SURGE_GITHUB_TOKEN"},
		{key: "github.owner", env: "SURGE_GITHUB_OWNER"},
		{key: "github.repo", env: "SURGE_GITHUB_REPO"},
		{key: "github.prNumber", env: "SURGE_PR_NUMBER"},
		{key: "ai.provider", env: "SURGE_AI_PROVIDER"},
		{key: "ai.model", env: "SURGE_AI_MODEL"},
		{key: "ai.baseUrl", env: "SURGE_AI_BASE_URL"},
		{key: "ai.apiKey", env: "SURGE_AI_API_KEY"},
		{key: "contextDepth", env: "SURGE_CONTEXT_DEPTH"},
		{key: "output.format", env: "SURGE_OUTPUT"},
		{key: "output.showStats", env: "SURGE_SHOW_STATS"},
		{key: "maxInlineComments", env: "SURGE_MAX_INLINE"},
		{key: "maxTokens", env: "SURGE_MAX_TOKENS"},
		{key: "temperature", env: "SURGE_TEMPERATURE"},
		{key: "dryRun", env: "SURGE_DRY_RUN"},
		{key: "verbose", env: "SURGE_VERBOSE"},
		{key: "noInline", env: "SURGE_NO_INLINE"},
		{key: "noSummary", env: "SURGE_NO_SUMMARY"},
		{key: "enablePRLabels", env: "SURGE_ENABLE_PR_LABELS"},
		{key: "prLabelPrefix", env: "SURGE_PR_LABEL_PREFIX"},
	} {
		if err := v.BindEnv(binding.key, binding.env); err != nil {
			errs = append(errs, fmt.Errorf("%s -> %s: %w", binding.key, binding.env, err))
		}
	}
	return errors.Join(errs...)
}

func applyDefaults(v *viper.Viper) {
	defaults := map[string]interface{}{
		"ai.provider":                defaultAIProvider,
		"ai.model":                   defaultAIModel,
		"ai.baseUrl":                 defaultAIBaseURL,
		"contextDepth":               defaultContextDepth,
		"output.format":              defaultOutputFormat,
		"output.colorize":            defaultOutputColorize,
		"output.showStats":           defaultOutputShowStats,
		"categories.security":        true,
		"categories.performance":     true,
		"categories.logic":           true,
		"categories.maintainability": true,
		"categories.vibe":            true,
		"maxTokens":                  defaultMaxTokens,
		"temperature":                defaultTemperature,
		"maxInlineComments":          defaultMaxInlineComments,
		"disableInlineComments":      defaultDisableInline,
		"disableSummaryComment":      defaultDisableSummary,
		"commentMarker":              defaultCommentMarker,
		"enablePRLabels":             defaultEnablePRLabels,
		"prLabelPrefix":              defaultPRLabelPrefix,
	}

	for key, value := range defaults {
		v.SetDefault(key, value)
	}
}

func (c *Config) expandEnvVars() {
	c.AI.BaseURL = expandEnv(c.AI.BaseURL)
	c.AI.APIKey = expandEnv(c.AI.APIKey)
}

func (c *Config) applyExplicitEnvOverrides() {
	if token := os.Getenv("SURGE_GITHUB_TOKEN"); token != "" {
		c.GitHub.Token = token
	}
	if apiKey := os.Getenv("SURGE_AI_API_KEY"); apiKey != "" {
		c.AI.APIKey = apiKey
	}
}

func expandEnv(s string) string {
	if len(s) < 4 {
		return s
	}
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		varName := s[2 : len(s)-1]
		if val := os.Getenv(varName); val != "" {
			return val
		}
	}
	return s
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if err := validateOneOf("ai.provider", c.AI.Provider, validAIProviders); err != nil {
		return err
	}
	if err := validateOneOf("contextDepth", c.ContextDepth, validContextDepth); err != nil {
		return err
	}
	if err := validateOneOf("output.format", c.Output.Format, validOutputFormat); err != nil {
		return err
	}
	return nil
}

func validateOneOf(field, value string, allowed []string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}

	quoted := make([]string, len(allowed))
	for i, candidate := range allowed {
		quoted[i] = fmt.Sprintf("%q", candidate)
	}

	return fmt.Errorf("%s must be one of %s, got %q", field, strings.Join(quoted, ", "), value)
}
