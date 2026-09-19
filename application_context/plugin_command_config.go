package application_context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mahresources/plugin_commands"
)

const (
	DefaultPluginCommandRunQuota          int64 = 8 << 30
	DefaultPluginCommandStagingQuota      int64 = 50 << 30
	DefaultPluginCommandExchangeRetention       = 7 * 24 * time.Hour
	DefaultPluginCommandOutputRetention         = 30 * 24 * time.Hour
)

// PluginCommandConfigInput preserves whether values were explicitly supplied,
// which is required to distinguish an omitted default from an explicit zero or
// empty trust boundary.
type PluginCommandConfigInput struct {
	CommandPath          string
	CommandPathExplicit  bool
	InheritedPath        string
	StagingPath          string
	StagingPathExplicit  bool
	FileSavePath         string
	MemoryFS             bool
	RunQuota             int64
	RunQuotaSet          bool
	StagingQuota         int64
	StagingQuotaSet      bool
	ExchangeRetention    time.Duration
	ExchangeRetentionSet bool
	OutputRetention      time.Duration
	OutputRetentionSet   bool
}

type PluginCommandConfig struct {
	CommandPath       string
	StagingPath       string
	RunQuota          int64
	StagingQuota      int64
	ExchangeRetention time.Duration
	OutputRetention   time.Duration
	TemporaryStaging  bool
}

// ResolvePluginCommandConfig validates the executable trust boundary and
// creates the private staging root before any plugin command goroutine starts.
func ResolvePluginCommandConfig(input PluginCommandConfigInput) (PluginCommandConfig, error) {
	commandPath := input.InheritedPath
	if input.CommandPathExplicit {
		commandPath = input.CommandPath
	}
	if err := validatePluginCommandPath(commandPath); err != nil {
		return PluginCommandConfig{}, err
	}

	runQuota := input.RunQuota
	if !input.RunQuotaSet {
		runQuota = DefaultPluginCommandRunQuota
	}
	stagingQuota := input.StagingQuota
	if !input.StagingQuotaSet {
		stagingQuota = DefaultPluginCommandStagingQuota
	}
	exchangeRetention := input.ExchangeRetention
	if !input.ExchangeRetentionSet {
		exchangeRetention = DefaultPluginCommandExchangeRetention
	}
	outputRetention := input.OutputRetention
	if !input.OutputRetentionSet {
		outputRetention = DefaultPluginCommandOutputRetention
	}
	if runQuota <= 0 {
		return PluginCommandConfig{}, fmt.Errorf("plugin command run quota must be positive")
	}
	if stagingQuota <= 0 {
		return PluginCommandConfig{}, fmt.Errorf("plugin command staging quota must be positive")
	}
	if exchangeRetention <= 0 {
		return PluginCommandConfig{}, fmt.Errorf("plugin command exchange retention must be positive")
	}
	if outputRetention <= 0 {
		return PluginCommandConfig{}, fmt.Errorf("plugin command output retention must be positive")
	}

	stagingPath := input.StagingPath
	temporary := false
	if input.StagingPathExplicit {
		if strings.TrimSpace(stagingPath) == "" {
			return PluginCommandConfig{}, fmt.Errorf("plugin command staging path must not be empty")
		}
	} else if input.MemoryFS {
		var err error
		stagingPath, err = os.MkdirTemp("", "mahresources-plugin-commands-")
		if err != nil {
			return PluginCommandConfig{}, fmt.Errorf("create plugin command staging root: %w", err)
		}
		temporary = true
	} else {
		if strings.TrimSpace(input.FileSavePath) == "" {
			return PluginCommandConfig{}, fmt.Errorf("plugin command staging requires a file save path")
		}
		stagingPath = filepath.Join(input.FileSavePath, "_plugin_commands")
	}
	if err := ensurePluginCommandStagingRoot(stagingPath); err != nil {
		if temporary {
			_ = os.RemoveAll(stagingPath)
		}
		return PluginCommandConfig{}, err
	}
	return PluginCommandConfig{
		CommandPath: commandPath, StagingPath: stagingPath,
		RunQuota: runQuota, StagingQuota: stagingQuota,
		ExchangeRetention: exchangeRetention, OutputRetention: outputRetention,
		TemporaryStaging: temporary,
	}, nil
}

func validatePluginCommandPath(value string) error {
	if value == "" {
		return fmt.Errorf("plugin command path must not be empty")
	}
	entries := filepath.SplitList(value)
	if len(entries) == 0 {
		return fmt.Errorf("plugin command path must contain an absolute directory")
	}
	for _, entry := range entries {
		if entry == "" {
			return fmt.Errorf("plugin command path contains an empty entry")
		}
		if !filepath.IsAbs(entry) {
			return fmt.Errorf("plugin command path entry %q is not absolute", entry)
		}
		info, err := os.Stat(entry)
		if err != nil {
			return fmt.Errorf("plugin command path entry %q: %w", entry, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("plugin command path entry %q is not a directory", entry)
		}
	}
	return nil
}

func ensurePluginCommandStagingRoot(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create plugin command staging root: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect plugin command staging root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("plugin command staging root %q is not a directory", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("make plugin command staging root private: %w", err)
	}
	return nil
}

type configPluginCommandSettings struct{ config *MahresourcesConfig }

func (s configPluginCommandSettings) StagingRoot() string        { return s.config.PluginCommandStagingPath }
func (s configPluginCommandSettings) PendingPerPluginLimit() int { return 0 }
func (s configPluginCommandSettings) PerRunQuota() int64         { return s.config.PluginCommandRunQuota }
func (s configPluginCommandSettings) GlobalStagingQuota() int64 {
	return s.config.PluginCommandStagingQuota
}
func (s configPluginCommandSettings) ExchangeRetention() time.Duration {
	return s.config.PluginCommandExchangeRetention
}
func (s configPluginCommandSettings) OutputRetention() time.Duration {
	return s.config.PluginCommandOutputRetention
}
func (s configPluginCommandSettings) CommandPath() string { return s.config.PluginCommandPath }

func (ctx *MahresourcesContext) PluginCommandSettings() plugin_commands.Settings {
	return configPluginCommandSettings{config: ctx.Config}
}
