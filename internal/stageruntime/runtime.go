package stageruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type RuntimeConfig map[string]any

func runtimeConfigPath(stageRoot string) string {
	return filepath.Join(filepath.Clean(stageRoot), "config", "phis-runtime.json")
}

func Load(stageRoot string) (RuntimeConfig, error) {
	configPath := runtimeConfigPath(stageRoot)
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read runtime config %s: %w", configPath, err)
	}

	var cfg RuntimeConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse runtime config %s: %w", configPath, err)
	}

	if cfg == nil {
		cfg = RuntimeConfig{}
	}

	return cfg, nil
}

func Save(stageRoot string, cfg RuntimeConfig) error {
	configPath := runtimeConfigPath(stageRoot)
	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode runtime config %s: %w", configPath, err)
	}

	if err := os.WriteFile(configPath, append(content, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write runtime config %s: %w", configPath, err)
	}

	return nil
}

func SetDatabaseURI(cfg RuntimeConfig, databaseURI string) RuntimeConfig {
	if cfg == nil {
		cfg = RuntimeConfig{}
	}

	database, ok := cfg["database"].(map[string]any)
	if !ok || database == nil {
		database = map[string]any{}
	}

	database["uri"] = databaseURI
	cfg["database"] = database
	return cfg
}

func GetDatabaseURI(cfg RuntimeConfig) string {
	if cfg == nil {
		return ""
	}

	database, ok := cfg["database"].(map[string]any)
	if !ok || database == nil {
		return ""
	}

	value, ok := database["uri"].(string)
	if !ok {
		return ""
	}

	return value
}
