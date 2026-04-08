package postgres

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

var identifierCleaner = regexp.MustCompile(`[^a-z0-9_]+`)

type BootstrapResult struct {
	DatabaseName string
	UserName     string
	Password     string
	DatabaseURI  string
}

func BootstrapStage(adminURI, stageName, password string) (BootstrapResult, error) {
	if strings.TrimSpace(adminURI) == "" {
		return BootstrapResult{}, fmt.Errorf("missing --admin-uri")
	}

	databaseName := buildIdentifier("phis_" + stageName)
	userName := buildIdentifier("phis_" + stageName + "_user")
	if databaseName == "" || userName == "" {
		return BootstrapResult{}, fmt.Errorf("could not derive valid database identifiers from stage %s", stageName)
	}

	if strings.TrimSpace(password) == "" {
		generatedPassword, err := generatePassword(24)
		if err != nil {
			return BootstrapResult{}, err
		}
		password = generatedPassword
	}

	if err := ensureRole(adminURI, userName, password); err != nil {
		return BootstrapResult{}, err
	}

	if err := ensureDatabase(adminURI, databaseName, userName); err != nil {
		return BootstrapResult{}, err
	}

	databaseURI, err := buildDatabaseURI(adminURI, databaseName, userName, password)
	if err != nil {
		return BootstrapResult{}, err
	}

	return BootstrapResult{
		DatabaseName: databaseName,
		UserName:     userName,
		Password:     password,
		DatabaseURI:  databaseURI,
	}, nil
}

func buildIdentifier(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	normalized = identifierCleaner.ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	normalized = strings.TrimPrefix(normalized, "_")

	return normalized
}

func generatePassword(byteLength int) (string, error) {
	buffer := make([]byte, byteLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("failed to generate database password: %w", err)
	}

	return hex.EncodeToString(buffer), nil
}

func ensureRole(adminURI, userName, password string) error {
	sql := fmt.Sprintf(
		`DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN
    ALTER ROLE %s WITH LOGIN PASSWORD %s;
  ELSE
    CREATE ROLE %s WITH LOGIN PASSWORD %s;
  END IF;
END $$;`,
		quoteLiteral(userName),
		quoteIdentifier(userName),
		quoteLiteral(password),
		quoteIdentifier(userName),
		quoteLiteral(password),
	)

	return execPSQL(adminURI, sql)
}

func ensureDatabase(adminURI, databaseName, userName string) error {
	existsOutput, err := execPSQLRead(adminURI, fmt.Sprintf(
		`SELECT 1 FROM pg_database WHERE datname = %s;`,
		quoteLiteral(databaseName),
	))
	if err != nil {
		return err
	}

	if strings.TrimSpace(existsOutput) == "1" {
		return nil
	}

	sql := fmt.Sprintf(
		`CREATE DATABASE %s OWNER %s;`,
		quoteIdentifier(databaseName),
		quoteIdentifier(userName),
	)

	return execPSQL(adminURI, sql)
}

func buildDatabaseURI(adminURI, databaseName, userName, password string) (string, error) {
	parsed, err := url.Parse(adminURI)
	if err != nil {
		return "", fmt.Errorf("failed to parse --admin-uri: %w", err)
	}

	parsed.User = url.UserPassword(userName, password)
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func execPSQL(adminURI, sql string) error {
	cmd := exec.Command("psql", adminURI, "-v", "ON_ERROR_STOP=1", "-c", sql)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("psql failed: %w", err)
		}

		return fmt.Errorf("psql failed: %s", trimmed)
	}

	return nil
}

func execPSQLRead(adminURI, sql string) (string, error) {
	cmd := exec.Command("psql", adminURI, "-v", "ON_ERROR_STOP=1", "-tA", "-c", sql)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", fmt.Errorf("psql failed: %w", err)
		}

		return "", fmt.Errorf("psql failed: %s", trimmed)
	}

	return string(output), nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}
