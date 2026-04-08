package phiscli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Phisys-Ltd/phis-host/internal/config"
)

type SiteLocation struct {
	Host string `json:"host"`
	Port *int   `json:"port"`
	Path string `json:"path"`
}

type SiteRuntimeSummary struct {
	DB struct {
		Key              string   `json:"key"`
		Name             string   `json:"name"`
		Hostname         string   `json:"hostname"`
		Enabled          bool     `json:"enabled"`
		PublicBaseURL    string   `json:"publicBaseUrl"`
		SupportEmail     string   `json:"supportEmail"`
		DefaultLocale    string   `json:"defaultLocale"`
		AvailableLocales []string `json:"availableLocales"`
		MailFrom         string   `json:"mailFrom"`
		ContactRecipient string   `json:"contactRecipient"`
	} `json:"db"`
	Config struct {
		Source    *SiteLocation  `json:"source,omitempty"`
		Instances []SiteLocation `json:"instances"`
	} `json:"config"`
}

func LoadSiteRuntimeSummary(stage config.StageConfig, siteKey string) (SiteRuntimeSummary, error) {
	cmd := exec.Command(
		stage.PhisPath,
		"--root",
		stage.Root,
		"site",
		"config",
		"runtime",
		"--key",
		strings.TrimSpace(siteKey),
		"--json",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput == "" {
			return SiteRuntimeSummary{}, fmt.Errorf("failed to run phis: %w", err)
		}

		return SiteRuntimeSummary{}, fmt.Errorf("failed to run phis: %s", trimmedOutput)
	}

	var summary SiteRuntimeSummary
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil {
		return SiteRuntimeSummary{}, fmt.Errorf("failed to parse phis runtime output: %w", err)
	}

	return summary, nil
}

func LoadBundledStaticConfig(phisPath string) ([]byte, error) {
	staticConfigPath, err := ResolveBundledStaticConfigPath(phisPath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(staticConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundled phis-config.json from %s: %w", staticConfigPath, err)
	}

	return content, nil
}

func ResolveBundledStaticConfigPath(phisPath string) (string, error) {
	resolvedBinary, err := resolvePhisBinary(phisPath)
	if err != nil {
		return "", err
	}

	appRoot := filepath.Dir(resolvedBinary)
	return filepath.Join(appRoot, "config", "phis-config.json"), nil
}

func FormatLocation(location SiteLocation) string {
	port := "-"
	if location.Port != nil {
		port = fmt.Sprintf("%d", *location.Port)
	}

	return fmt.Sprintf("host=%s port=%s path=%s", location.Host, port, location.Path)
}

func phisBinary(path string) string {
	if strings.TrimSpace(path) != "" {
		return path
	}

	return "phis"
}

func resolvePhisBinary(path string) (string, error) {
	lookedUpPath, err := exec.LookPath(phisBinary(path))
	if err != nil {
		return "", fmt.Errorf("failed to locate phis binary %s: %w", phisBinary(path), err)
	}

	resolvedPath, err := filepath.EvalSymlinks(lookedUpPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve phis binary %s: %w", lookedUpPath, err)
	}

	return resolvedPath, nil
}
