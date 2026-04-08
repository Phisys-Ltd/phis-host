package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultConfigPath = "/var/lib/phis/phis-host/config.json"

type StageConfig struct {
	Root     string `json:"root"`
	PhisPath string `json:"phisPath,omitempty"`
}

type Config struct {
	DefaultStage string                 `json:"defaultStage,omitempty"`
	Stages       map[string]StageConfig `json:"stages"`
}

func DefaultStageRoot(stage string) string {
	return filepath.Join("/srv/phis/stages", strings.TrimSpace(stage))
}

func ResolvePath(explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}

	if envPath := strings.TrimSpace(os.Getenv("PHIS_HOST_CONFIG")); envPath != "" {
		return envPath
	}

	localPath := filepath.Join(".", "phis-host.json")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	return defaultConfigPath
}

func Load(configPath string) (Config, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config %s: %w", configPath, err)
	}

	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config %s: %w", configPath, err)
	}

	if len(cfg.Stages) == 0 {
		return Config{}, fmt.Errorf("config %s does not define any stages", configPath)
	}

	for stageName, stage := range cfg.Stages {
		if strings.TrimSpace(stage.Root) == "" {
			return Config{}, fmt.Errorf("stage %s is missing root", stageName)
		}
	}

	if cfg.DefaultStage != "" {
		if _, ok := cfg.Stages[cfg.DefaultStage]; !ok {
			return Config{}, fmt.Errorf("defaultStage %s is not configured", cfg.DefaultStage)
		}
	}

	return cfg, nil
}

func LoadOptional(configPath string) (Config, bool, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return Config{}, false, nil
		}

		return Config{}, false, fmt.Errorf("failed to read config %s: %w", configPath, err)
	}

	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, true, fmt.Errorf("failed to parse config %s: %w", configPath, err)
	}

	if len(cfg.Stages) == 0 {
		cfg.Stages = map[string]StageConfig{}
	}

	return cfg, true, nil
}

func Save(configPath string, cfg Config) error {
	if cfg.Stages == nil {
		cfg.Stages = map[string]StageConfig{}
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory for %s: %w", configPath, err)
	}

	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config %s: %w", configPath, err)
	}

	if err := os.WriteFile(configPath, append(content, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config %s: %w", configPath, err)
	}

	return nil
}

func (cfg Config) EffectiveDefaultStage() string {
	if strings.TrimSpace(cfg.DefaultStage) != "" {
		return cfg.DefaultStage
	}

	return "prod"
}

func (cfg Config) StageNames() []string {
	stageNames := make([]string, 0, len(cfg.Stages))
	for stageName := range cfg.Stages {
		stageNames = append(stageNames, stageName)
	}

	sort.Strings(stageNames)
	return stageNames
}

func (cfg Config) Stage(name string) (StageConfig, error) {
	stageName := strings.TrimSpace(name)
	if stageName == "" {
		stageName = cfg.EffectiveDefaultStage()
	}

	stage, ok := cfg.Stages[stageName]
	if !ok {
		return StageConfig{}, fmt.Errorf("stage %s is not configured", stageName)
	}

	if strings.TrimSpace(stage.PhisPath) == "" {
		stage.PhisPath = "phis"
	}

	return stage, nil
}
