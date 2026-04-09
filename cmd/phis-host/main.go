package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Phisys-Ltd/phis-host/internal/config"
	"github.com/Phisys-Ltd/phis-host/internal/phiscli"
	"github.com/Phisys-Ltd/phis-host/internal/postgres"
	"github.com/Phisys-Ltd/phis-host/internal/stageruntime"
	"github.com/Phisys-Ltd/phis-host/internal/version"
)

type flagMap map[string]string

type parsedArgs struct {
	flags       flagMap
	boolFlags   map[string]bool
	positionals []string
}

var shortFlagAliases = map[string]string{
	"c": "config",
	"h": "help",
	"j": "json",
	"k": "key",
	"r": "root",
	"s": "stage",
	"v": "version",
}

var knownLongFlags = map[string]bool{
	"config":      true,
	"db-password": true,
	"force":       true,
	"help":        true,
	"json":        true,
	"key":         true,
	"admin-uri":   true,
	"phis-path":   true,
	"root":        true,
	"stage":       true,
	"version":     true,
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if hasJSONFlag(os.Args[1:]) {
			_ = writeJSON(os.Stderr, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	parsed, err := parseArgs(args)
	if err != nil {
		return err
	}

	if len(parsed.positionals) == 0 || parsed.boolFlags["help"] || firstPositionalIsHelp(parsed.positionals) {
		printUsage()
		return nil
	}

	if parsed.boolFlags["version"] || parsed.positionals[0] == "version" {
		if parsed.boolFlags["json"] {
			return printJSON(map[string]string{
				"name":    version.Name,
				"version": version.Version,
			})
		}

		fmt.Println(version.String())
		return nil
	}

	if parsed.positionals[0] == "stage" && len(parsed.positionals) > 1 && parsed.positionals[1] == "init" {
		if len(parsed.positionals) > 2 && parsed.positionals[2] == "db" {
			cfg, err := loadResolvedConfig(parsed)
			if err != nil {
				return err
			}

			return runStageInitDB(parsed, cfg)
		}

		return runStageInit(parsed)
	}

	cfg, err := loadResolvedConfig(parsed)
	if err != nil {
		return err
	}

	stageName := resolveStageName(parsed, cfg)
	stage, err := cfg.Stage(stageName)
	if err != nil {
		return err
	}
	if phisPath := strings.TrimSpace(parsed.flags["phis-path"]); phisPath != "" {
		stage.PhisPath = phisPath
	}

	switch parsed.positionals[0] {
	case "stage":
		return runStageCommand(parsed, cfg)
	case "site":
		return runSiteCommand(parsed, stageName, stage)
	default:
		return fmt.Errorf("unknown command: %s", strings.Join(parsed.positionals, " "))
	}
}

func runStageCommand(parsed parsedArgs, cfg config.Config) error {
	if len(parsed.positionals) < 2 {
		return fmt.Errorf("missing stage command")
	}

	switch parsed.positionals[1] {
	case "list":
		stageNames := cfg.StageNames()
		if parsed.boolFlags["json"] {
			return printJSON(map[string]any{
				"defaultStage": cfg.EffectiveDefaultStage(),
				"stages":       stageNames,
			})
		}

		fmt.Printf("default=%s\n", cfg.EffectiveDefaultStage())
		for _, stageName := range stageNames {
			fmt.Println(stageName)
		}
		return nil
	case "init":
		if len(parsed.positionals) > 2 && parsed.positionals[2] == "db" {
			return runStageInitDB(parsed, cfg)
		}
		return runStageInit(parsed)
	default:
		return fmt.Errorf("unknown stage command: %s", parsed.positionals[1])
	}
}

func runStageInit(parsed parsedArgs) error {
	configPath := config.ResolvePath(parsed.flags["config"])
	stagesReadmeDir := filepath.Join(filepath.Dir(configPath), "stages")
	stagesReadmePath := filepath.Join(stagesReadmeDir, "README.md")
	cfg, _, err := config.LoadOptional(configPath)
	if err != nil {
		return err
	}

	if cfg.Stages == nil {
		cfg.Stages = map[string]config.StageConfig{}
	}

	stageName := strings.TrimSpace(parsed.flags["stage"])
	if stageName == "" {
		stageName = "prod"
	}

	stageRoot := strings.TrimSpace(parsed.flags["root"])
	if stageRoot == "" {
		stageRoot = config.DefaultStageRoot(stageName)
	}
	stageRoot = filepath.Clean(stageRoot)

	existingStage, exists := cfg.Stages[stageName]
	if exists && strings.TrimSpace(existingStage.Root) != "" && filepath.Clean(existingStage.Root) != stageRoot {
		return fmt.Errorf("stage %s already exists with root %s", stageName, existingStage.Root)
	}

	stageConfig := existingStage
	stageConfig.Root = stageRoot
	if phisPath := strings.TrimSpace(parsed.flags["phis-path"]); phisPath != "" {
		stageConfig.PhisPath = phisPath
	}
	cfg.Stages[stageName] = stageConfig

	if strings.TrimSpace(cfg.DefaultStage) == "" {
		cfg.DefaultStage = stageName
	}

	if err := config.Save(configPath, cfg); err != nil {
		return err
	}

	createdPaths := []string{}
	for _, targetPath := range []string{
		stageRoot,
		filepath.Join(stageRoot, "config"),
		stagesReadmeDir,
	} {
		if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
			createdPaths = append(createdPaths, targetPath)
		}

		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", targetPath, err)
		}
	}

	staticConfigPath := filepath.Join(stageRoot, "config", "phis-config.json")
	runtimeConfigPath := filepath.Join(stageRoot, "config", "phis-runtime.json")
	if _, statErr := os.Stat(staticConfigPath); os.IsNotExist(statErr) {
		staticConfigContent, err := phiscli.LoadBundledStaticConfig(stageConfig.PhisPath)
		if err != nil {
			return err
		}

		if err := os.WriteFile(staticConfigPath, staticConfigContent, 0o644); err != nil {
			return fmt.Errorf("failed to create %s: %w", staticConfigPath, err)
		}
		createdPaths = append(createdPaths, staticConfigPath)
	}

	if _, statErr := os.Stat(runtimeConfigPath); os.IsNotExist(statErr) {
		if err := os.WriteFile(runtimeConfigPath, []byte("{}\n"), 0o644); err != nil {
			return fmt.Errorf("failed to create %s: %w", runtimeConfigPath, err)
		}
		createdPaths = append(createdPaths, runtimeConfigPath)
	}

	if _, statErr := os.Stat(stagesReadmePath); os.IsNotExist(statErr) {
		createdPaths = append(createdPaths, stagesReadmePath)
	}
	if err := os.WriteFile(stagesReadmePath, []byte(buildStagesReadme()), 0o644); err != nil {
		return fmt.Errorf("failed to create %s: %w", stagesReadmePath, err)
	}

	ownerApplied, ownerError := tryApplyPhisOwnership(stageRoot)

	if parsed.boolFlags["json"] {
		return printJSON(map[string]any{
			"ok":           true,
			"action":       "stage.init",
			"configPath":   configPath,
			"stage":        stageName,
			"root":         stageRoot,
			"createdPaths": createdPaths,
			"ownerApplied": ownerApplied,
			"ownerError":   ownerError,
		})
	}

	fmt.Printf("Initialized stage %s at %s.\n", stageName, stageRoot)
	if ownerApplied {
		fmt.Println("Applied ownership phis:phis.")
	} else if ownerError != "" {
		fmt.Printf("Ownership skipped: %s\n", ownerError)
	}

	return nil
}

func runStageInitDB(parsed parsedArgs, cfg config.Config) error {
	stageName := resolveStageName(parsed, cfg)
	stage, err := cfg.Stage(stageName)
	if err != nil {
		return err
	}

	adminURI, err := requiredFlag(parsed, "admin-uri")
	if err != nil {
		return err
	}

	force := parsed.boolFlags["force"]

	runtimeConfig, err := stageruntime.Load(stage.Root)
	if err != nil {
		return err
	}

	existingDatabaseURI := stageruntime.GetDatabaseURI(runtimeConfig)
	if existingDatabaseURI != "" && !force {
		return fmt.Errorf(
			"stage %s already has database.uri configured. Re-run with --force to overwrite the runtime database connection.",
			stageName,
		)
	}

	result, err := postgres.BootstrapStage(adminURI, stageName, strings.TrimSpace(parsed.flags["db-password"]))
	if err != nil {
		return err
	}

	runtimeConfig = stageruntime.SetDatabaseURI(runtimeConfig, result.DatabaseURI)
	if err := stageruntime.Save(stage.Root, runtimeConfig); err != nil {
		return err
	}

	if parsed.boolFlags["json"] {
		return printJSON(map[string]any{
			"ok":          true,
			"action":      "stage.init.db",
			"stage":       stageName,
			"root":        stage.Root,
			"database":    result.DatabaseName,
			"user":        result.UserName,
			"databaseURI": result.DatabaseURI,
		})
	}

	fmt.Printf("Initialized database bootstrap for stage %s.\n", stageName)
	fmt.Printf("database: %s\n", result.DatabaseName)
	fmt.Printf("user:     %s\n", result.UserName)
	return nil
}

func runSiteCommand(parsed parsedArgs, stageName string, stage config.StageConfig) error {
	if len(parsed.positionals) < 3 {
		return fmt.Errorf("missing site command")
	}

	switch parsed.positionals[1] {
	case "show":
		if len(parsed.positionals) < 3 {
			return fmt.Errorf("missing site show target")
		}

		switch parsed.positionals[2] {
		case "runtime":
			siteKey, err := requiredFlag(parsed, "key")
			if err != nil {
				return err
			}

			summary, err := phiscli.LoadSiteRuntimeSummary(stage, siteKey)
			if err != nil {
				return err
			}

			if parsed.boolFlags["json"] {
				return printJSON(map[string]any{
					siteKey: map[string]any{
						"stage": stageName,
						"db":    summary.DB,
						"config": map[string]any{
							"source":    summary.Config.Source,
							"instances": summary.Config.Instances,
						},
					},
				})
			}

			fmt.Printf("stage: %s\n", stageName)
			fmt.Printf("site:  %s\n", siteKey)
			fmt.Printf("name:  %s\n", summary.DB.Name)
			fmt.Printf("host:  %s\n", summary.DB.Hostname)
			fmt.Printf("instances: %d\n", len(summary.Config.Instances))
			return nil
		case "source":
			siteKey, err := requiredFlag(parsed, "key")
			if err != nil {
				return err
			}

			summary, err := phiscli.LoadSiteRuntimeSummary(stage, siteKey)
			if err != nil {
				return err
			}

			if parsed.boolFlags["json"] {
				return printJSON(map[string]any{
					siteKey: map[string]any{
						"stage":  stageName,
						"source": summary.Config.Source,
					},
				})
			}

			if summary.Config.Source == nil {
				fmt.Println("No source override configured.")
				return nil
			}

			fmt.Println(phiscli.FormatLocation(*summary.Config.Source))
			return nil
		case "instances":
			siteKey, err := requiredFlag(parsed, "key")
			if err != nil {
				return err
			}

			summary, err := phiscli.LoadSiteRuntimeSummary(stage, siteKey)
			if err != nil {
				return err
			}

			if parsed.boolFlags["json"] {
				return printJSON(map[string]any{
					siteKey: map[string]any{
						"stage":     stageName,
						"instances": summary.Config.Instances,
					},
				})
			}

			if len(summary.Config.Instances) == 0 {
				fmt.Println("No instances configured.")
				return nil
			}

			for _, instance := range summary.Config.Instances {
				fmt.Println(phiscli.FormatLocation(instance))
			}
			return nil
		default:
			return fmt.Errorf("unknown site show target: %s", parsed.positionals[2])
		}
	default:
		return fmt.Errorf("unknown site command: %s", parsed.positionals[1])
	}
}

func loadResolvedConfig(parsed parsedArgs) (config.Config, error) {
	configPath := parsed.flags["config"]
	resolvedPath := config.ResolvePath(configPath)
	cfg, exists, err := config.LoadOptional(resolvedPath)
	if err != nil {
		return config.Config{}, err
	}

	if !exists {
		stageName := strings.TrimSpace(parsed.flags["stage"])
		if stageName == "" {
			stageName = "prod"
		}

		return config.Config{}, fmt.Errorf(
			"phis-host is not initialized for config %s. Run `phis-host stage init --stage %s` first.",
			resolvedPath,
			stageName,
		)
	}

	if len(cfg.Stages) == 0 {
		stageName := strings.TrimSpace(parsed.flags["stage"])
		if stageName == "" {
			stageName = "prod"
		}

		return config.Config{}, fmt.Errorf(
			"phis-host config %s does not define any stages. Run `phis-host stage init --stage %s` first.",
			resolvedPath,
			stageName,
		)
	}

	return cfg, nil
}

func resolveStageName(parsed parsedArgs, cfg config.Config) string {
	if stage := parsed.flags["stage"]; stage != "" {
		return strings.TrimSpace(stage)
	}

	return cfg.EffectiveDefaultStage()
}

func printUsage() {
	fmt.Println(`phis-host

Usage:
  Append [--json|-j] to any command for JSON output.

  phis-host version
  phis-host stage list [--config|-c <path>]
  phis-host stage init [--stage|-s <stage>] [--root|-r <path>] [--phis-path <path>] [--config|-c <path>]
  phis-host stage init db [--stage|-s <stage>] [--config|-c <path>] --admin-uri <postgres-uri> [--db-password <password>] [--force|-f]
  phis-host site show runtime --key|-k <site key> [--stage|-s <stage>] [--phis-path <path>] [--config|-c <path>]
  phis-host site show source --key|-k <site key> [--stage|-s <stage>] [--phis-path <path>] [--config|-c <path>]
  phis-host site show instances --key|-k <site key> [--stage|-s <stage>] [--phis-path <path>] [--config|-c <path>]

Notes:
  --stage defaults to the configured default stage or 'prod'.
  --config defaults to ./phis-host.json, then PHIS_HOST_CONFIG, then /var/lib/phis/phis-host/config.json.
`)
}

func parseArgs(args []string) (parsedArgs, error) {
	parsed := parsedArgs{
		flags:       flagMap{},
		boolFlags:   map[string]bool{},
		positionals: make([]string, 0),
	}

	for index := 0; index < len(args); index++ {
		current := args[index]
		if !strings.HasPrefix(current, "-") {
			parsed.positionals = append(parsed.positionals, current)
			continue
		}

		if current == "--" {
			parsed.positionals = append(parsed.positionals, args[index+1:]...)
			break
		}

		isLong := strings.HasPrefix(current, "--")
		rawKey := strings.TrimLeft(current, "-")
		key := rawKey
		if isLong {
			if !knownLongFlags[key] {
				return parsed, fmt.Errorf("unknown flag: %s", current)
			}
		} else {
			alias, ok := shortFlagAliases[rawKey]
			if !ok {
				return parsed, fmt.Errorf("unknown flag: %s", current)
			}
			key = alias
		}

		nextIndex := index + 1
		if nextIndex >= len(args) || strings.HasPrefix(args[nextIndex], "-") {
			parsed.boolFlags[key] = true
			continue
		}

		parsed.flags[key] = strings.TrimSpace(args[nextIndex])
		index++
	}

	return parsed, nil
}

func requiredFlag(parsed parsedArgs, key string) (string, error) {
	value, ok := parsed.flags[key]
	if !ok || value == "" {
		return "", fmt.Errorf("missing --%s", key)
	}

	return value, nil
}

func firstPositionalIsHelp(positionals []string) bool {
	if len(positionals) == 0 {
		return false
	}

	first := positionals[0]
	return first == "help" || first == "--help" || first == "-h"
}

func printJSON(value any) error {
	return writeJSON(os.Stdout, value)
}

func writeJSON(target *os.File, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	fmt.Fprintln(target, string(content))
	return nil
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "-j" {
			return true
		}
	}

	return false
}

func tryApplyPhisOwnership(stageRoot string) (bool, string) {
	phisUser, err := user.Lookup("phis")
	if err != nil {
		return false, "user phis does not exist"
	}

	phisGroup, err := user.LookupGroup("phis")
	if err != nil {
		return false, "group phis does not exist"
	}

	uid, err := strconv.Atoi(phisUser.Uid)
	if err != nil {
		return false, "invalid phis uid"
	}

	gid, err := strconv.Atoi(phisGroup.Gid)
	if err != nil {
		return false, "invalid phis gid"
	}

	for _, targetPath := range []string{
		stageRoot,
		filepath.Join(stageRoot, "config"),
		filepath.Join(stageRoot, "config", "phis-config.json"),
		filepath.Join(stageRoot, "config", "phis-runtime.json"),
	} {
		if err := os.Chown(targetPath, uid, gid); err != nil {
			return false, err.Error()
		}
	}

	return true, ""
}

func buildStagesReadme() string {
	return `# Stage Config

This directory documents the ` + "`phis`" + ` stage configuration layout managed by ` + "`phis-host`" + `.

Per-stage files live under:

- ` + "`<stage-root>/config/phis-config.json`" + `
- ` + "`<stage-root>/config/phis-runtime.json`" + `

Guidance:

- ` + "`phis-host`" + ` is optional. It is only a host-side helper for bootstrap and lifecycle tasks.
- ` + "`phis-host`" + ` may be versioned and installed separately from ` + "`phis`" + `.
- ` + "`phis-host`" + ` is the place where the host-side automation of ` + "`phi-server`" + ` lifecycle can live.
- ` + "`phis-host`" + ` should support both Debian-based host setups and Docker-based lifecycle flows.
- The recommended loopback port convention for the central ` + "`phis`" + ` runtime starts at ` + "`5301`" + ` for the first stage and increments by ` + "`1`" + ` for each additional stage.
- Port allocation is host-local deployment state and must not be derived from the stage name itself; stage names remain free-form.
- Do not edit ` + "`phis-config.json`" + ` manually unless you deliberately replace the bundled defaults.
- ` + "`phis-config.json`" + ` comes from the installed ` + "`phis`" + ` version and may be replaced again during stage initialization or packaging workflows.
- Put local, stage-specific, or host-specific overrides into ` + "`phis-runtime.json`" + `.
- Prefer changing ` + "`phis-runtime.json`" + ` through ` + "`phis`" + ` or ` + "`phis-host`" + ` instead of editing JSON by hand.

CLI split:

- ` + "`phis`" + `
  - application and database actions
  - merged config inspection
  - runtime config writes owned by the app/control-plane
- ` + "`phis-host`" + `
  - stage roots
  - host/bootstrap tasks
  - PostgreSQL bootstrap
  - future systemd/nginx/apache wiring
`
}
