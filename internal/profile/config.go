package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds per-profile settings stored in <profile>/config.json.
type Config struct {
	Proxy        map[string]string `json:"proxy,omitempty"`
	Integrations map[string]bool   `json:"integrations,omitempty"`
}

// LoadConfig reads the profile's configuration; a missing file means an
// empty configuration.
func LoadConfig(profilePath string) (Config, error) {
	var config Config
	contents, err := os.ReadFile(filepath.Join(profilePath, "config.json"))
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("read profile configuration: %w", err)
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		return config, fmt.Errorf("parse profile configuration: %w", err)
	}
	return config, nil
}

// SaveConfig writes the profile's configuration.
func SaveConfig(profilePath string, config Config) error {
	contents, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(profilePath, "config.json"), append(contents, '\n'), 0o600); err != nil {
		return fmt.Errorf("write profile configuration: %w", err)
	}
	return nil
}
