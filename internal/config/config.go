// Package config manages application configuration loading, defaults, and persistence.
// It uses Viper for layered config (file → env → defaults) so any setting can be
// overridden by an environment variable without touching the config file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

const (
	AppName    = "cmdctx"
	ConfigDir  = ".cmdctx"
	ConfigFile = "config"
	EnvPrefix  = "CMDCTX"
)

// Provider holds settings for a single AI provider.
type Provider struct {
	Name    string `mapstructure:"name"     yaml:"name"`
	Type    string `mapstructure:"type"     yaml:"type"` // openai | ollama | anthropic
	BaseURL string `mapstructure:"base_url" yaml:"base_url"`
	APIKey  string `mapstructure:"api_key"  yaml:"api_key"`
	Model   string `mapstructure:"model"    yaml:"model"`
}

// Config is the root application configuration struct.
// All durations are stored as strings in YAML and parsed by Viper.
type Config struct {
	ActiveProvider   string            `mapstructure:"active_provider"   yaml:"active_provider"`
	Providers        []Provider        `mapstructure:"providers"         yaml:"providers"`
	DefaultScanMode  string            `mapstructure:"default_scan_mode" yaml:"default_scan_mode"` // safe | deep
	ExecutionTimeout time.Duration     `mapstructure:"execution_timeout" yaml:"execution_timeout"`
	OutputMaxBytes   int               `mapstructure:"output_max_bytes"  yaml:"output_max_bytes"`
	DefaultExcludes  []string          `mapstructure:"default_excludes"  yaml:"default_excludes"`
	PreferredTools   map[string]string `mapstructure:"preferred_tools"   yaml:"preferred_tools"`
	HistoryRetention int               `mapstructure:"history_retention" yaml:"history_retention"` // days
	Theme            string            `mapstructure:"theme"             yaml:"theme"`
	InstallDir       string            `mapstructure:"install_dir"       yaml:"install_dir"`
	LogLevel         string            `mapstructure:"log_level"         yaml:"log_level"`
	// AIPermissionMode controls whether summarized metadata may be sent to the AI.
	// Values: "local_only" | "allow_metadata"
	AIPermissionMode string `mapstructure:"ai_permission_mode" yaml:"ai_permission_mode"`
}

// GlobalDir returns the global ~/.cmdctx directory path.
func GlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ConfigDir)
	}
	return filepath.Join(home, ConfigDir)
}

// GlobalConfigPath returns the path to the global config file.
func GlobalConfigPath() string {
	return filepath.Join(GlobalDir(), ConfigFile+".yaml")
}

// ProjectDir returns the project-local .cmdctx directory if cwd is inside a project.
func ProjectDir(cwd string) string {
	return filepath.Join(cwd, ConfigDir)
}

// DefaultConfig returns production-safe defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ActiveProvider:   "ollama",
		DefaultScanMode:  "safe",
		ExecutionTimeout: 30 * time.Second,
		OutputMaxBytes:   512 * 1024, // 512 KiB
		DefaultExcludes: []string{
			".git", "node_modules", "vendor", "dist", "build", ".cache",
			".next", ".nuxt", "target", "__pycache__", ".tox", ".venv",
			"venv", ".idea", ".vscode",
		},
		PreferredTools: map[string]string{
			"text_search": "rg",
			"file_search": "fd",
			"json_search": "jq",
		},
		HistoryRetention: 90,
		Theme:            "default",
		InstallDir:       filepath.Join(home, ".local", "bin"),
		LogLevel:         "warn",
		AIPermissionMode: "local_only",
	}
}

// Load reads the global config file and returns a populated Config.
// It applies defaults so callers always receive a valid Config even on first run.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName(ConfigFile)
	v.SetConfigType("yaml")
	v.AddConfigPath(GlobalDir())

	// Environment variable override: CMDCTX_ACTIVE_PROVIDER etc.
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	applyDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		// Config file not found on first run is acceptable.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Watch for live changes (useful in TUI sessions).
	v.WatchConfig()
	v.OnConfigChange(func(_ fsnotify.Event) {
		// Intentionally empty: callers may re-call Load() or use the watcher signal.
	})

	return cfg, nil
}

// Save persists the given Config to the global config file.
func Save(cfg *Config) error {
	dir := GlobalDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	v := viper.New()
	v.SetConfigName(ConfigFile)
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)

	v.Set("active_provider", cfg.ActiveProvider)
	v.Set("default_scan_mode", cfg.DefaultScanMode)
	v.Set("execution_timeout", cfg.ExecutionTimeout.String())
	v.Set("output_max_bytes", cfg.OutputMaxBytes)
	v.Set("default_excludes", cfg.DefaultExcludes)
	v.Set("preferred_tools", cfg.PreferredTools)
	v.Set("history_retention", cfg.HistoryRetention)
	v.Set("theme", cfg.Theme)
	v.Set("install_dir", cfg.InstallDir)
	v.Set("log_level", cfg.LogLevel)
	v.Set("ai_permission_mode", cfg.AIPermissionMode)
	v.Set("providers", cfg.Providers)

	path := GlobalConfigPath()
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// ActiveProviderConfig returns the Provider config for the currently active provider.
// Returns nil if not found.
func (c *Config) ActiveProviderConfig() *Provider {
	for i := range c.Providers {
		if c.Providers[i].Name == c.ActiveProvider {
			return &c.Providers[i]
		}
	}
	return nil
}

// IsFirstRun returns true when the global config directory does not yet exist.
func IsFirstRun() bool {
	_, err := os.Stat(GlobalDir())
	return os.IsNotExist(err)
}

// EnsureGlobalDir creates the global config directory if it does not exist.
func EnsureGlobalDir() error {
	return os.MkdirAll(GlobalDir(), 0o700)
}

func applyDefaults(v *viper.Viper) {
	d := DefaultConfig()
	v.SetDefault("active_provider", d.ActiveProvider)
	v.SetDefault("default_scan_mode", d.DefaultScanMode)
	v.SetDefault("execution_timeout", d.ExecutionTimeout.String())
	v.SetDefault("output_max_bytes", d.OutputMaxBytes)
	v.SetDefault("default_excludes", d.DefaultExcludes)
	v.SetDefault("preferred_tools", d.PreferredTools)
	v.SetDefault("history_retention", d.HistoryRetention)
	v.SetDefault("theme", d.Theme)
	v.SetDefault("install_dir", d.InstallDir)
	v.SetDefault("log_level", d.LogLevel)
	v.SetDefault("ai_permission_mode", d.AIPermissionMode)
}
