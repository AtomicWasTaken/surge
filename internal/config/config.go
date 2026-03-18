package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the full surge configuration.
type Config struct {
	AI           AIConfig          `mapstructure:"ai"`
	ContextDepth string            `mapstructure:"contextDepth"`
	Output       OutputConfig      `mapstructure:"output"`
	Categories   CategoriesConfig  `mapstructure:"categories"`
	GitHub       GitHubConfig      `mapstructure:"github"`

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
	MinSeverity string   `mapstructure:"minSeverity"`

	// Comment marker
	CommentMarker string `mapstructure:"commentMarker"`
}

// AIConfig configures the AI provider.
type AIConfig struct {
	Provider string `mapstructure:"provider"` // "litellm" or "claude"
	Model   string `mapstructure:"model"`
	BaseURL string `mapstructure:"baseUrl"`
	APIKey  string `mapstructure:"apiKey"`
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
	Token    string `mapstructure:"token"`    // Loaded from env
	Owner    string `mapstructure:"owner"`
	Repo     string `mapstructure:"repo"`
	PRNumber int    `mapstructure:"prNumber"`
}

// Load reads the configuration from file, environment, and flags.
// Precedence: CLI flags > env vars > config file > defaults.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("surge")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath(filepath.Join(os.Getenv("HOME"), ".config", "surge"))
		v.AddConfigPath("/etc/surge")
	}

	// Environment variables
	v.SetEnvPrefix("SURGE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific env vars
	_ = v.BindEnv("github.token", "SURGE_GITHUB_TOKEN", "GITHUB_TOKEN")
	_ = v.BindEnv("github.owner", "SURGE_GITHUB_OWNER")
	_ = v.BindEnv("github.repo", "SURGE_GITHUB_REPO")
	_ = v.BindEnv("github.prNumber", "SURGE_PR_NUMBER")
	_ = v.BindEnv("ai.provider", "SURGE_AI_PROVIDER")
	_ = v.BindEnv("ai.model", "SURGE_AI_MODEL")
	_ = v.BindEnv("ai.baseUrl", "SURGE_AI_BASE_URL")
	_ = v.BindEnv("ai.apiKey", "SURGE_AI_API_KEY")
	_ = v.BindEnv("contextDepth", "SURGE_CONTEXT_DEPTH")
	_ = v.BindEnv("output.format", "SURGE_OUTPUT")
	_ = v.BindEnv("output.showStats", "SURGE_SHOW_STATS")
	_ = v.BindEnv("maxInlineComments", "SURGE_MAX_INLINE")
	_ = v.BindEnv("maxTokens", "SURGE_MAX_TOKENS")
	_ = v.BindEnv("temperature", "SURGE_TEMPERATURE")
	_ = v.BindEnv("dryRun", "SURGE_DRY_RUN")
	_ = v.BindEnv("verbose", "SURGE_VERBOSE")
	_ = v.BindEnv("noInline", "SURGE_NO_INLINE")
	_ = v.BindEnv("noSummary", "SURGE_NO_SUMMARY")

	// Read config file if it exists
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	// Apply defaults
	applyDefaults(v)

	// Unmarshal
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand env var references in config values
	cfg.expandEnvVars()

	// Override with env vars that were explicitly set
	if token := os.Getenv("SURGE_GITHUB_TOKEN"); token != "" {
		cfg.GitHub.Token = token
	} else if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		cfg.GitHub.Token = token
	}
	if apiKey := os.Getenv("SURGE_AI_API_KEY"); apiKey != "" {
		cfg.AI.APIKey = apiKey
	}

	return &cfg, nil
}

func applyDefaults(v *viper.Viper) {
	// AI defaults
	v.SetDefault("ai.provider", "litellm")
	v.SetDefault("ai.model", "anthropic/claude-3-5-sonnet-20241022")
	v.SetDefault("ai.baseUrl", "http://localhost:4000")

	// Context defaults
	v.SetDefault("contextDepth", "diff-only")

	// Output defaults
	v.SetDefault("output.format", "terminal")
	v.SetDefault("output.colorize", true)
	v.SetDefault("output.showStats", false)

	// Category defaults (all enabled)
	v.SetDefault("categories.security", true)
	v.SetDefault("categories.performance", true)
	v.SetDefault("categories.logic", true)
	v.SetDefault("categories.maintainability", true)
	v.SetDefault("categories.vibe", true)

	// Model settings
	v.SetDefault("maxTokens", 8192)
	v.SetDefault("temperature", 0.3)

	// Inline comments
	v.SetDefault("maxInlineComments", 20)
	v.SetDefault("disableInlineComments", false)
	v.SetDefault("disableSummaryComment", false)

	// Comment marker
	v.SetDefault("commentMarker", "SURGE")
}

func (c *Config) expandEnvVars() {
	// Expand ${VAR} patterns in string fields
	c.AI.BaseURL = expandEnv(c.AI.BaseURL)
	c.AI.APIKey = expandEnv(c.AI.APIKey)
}

func expandEnv(s string) string {
	if len(s) < 4 {
		return s
	}
	// Simple ${VAR} expansion
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
	if c.AI.Provider != "litellm" && c.AI.Provider != "claude" {
		return fmt.Errorf("ai.provider must be 'litellm' or 'claude', got %q", c.AI.Provider)
	}
	if c.ContextDepth != "diff-only" && c.ContextDepth != "relevant" && c.ContextDepth != "full" {
		return fmt.Errorf("contextDepth must be 'diff-only', 'relevant', or 'full', got %q", c.ContextDepth)
	}
	if c.Output.Format != "terminal" && c.Output.Format != "markdown" && c.Output.Format != "json" {
		return fmt.Errorf("output.format must be 'terminal', 'markdown', or 'json', got %q", c.Output.Format)
	}
	return nil
}
