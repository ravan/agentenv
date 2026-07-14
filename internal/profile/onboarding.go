package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// claudeOnboardingFields are the login identity and onboarding markers in the
// user's real ~/.claude.json that gate Claude's first-run setup and login
// screens. The OAuth token itself is adopted into the shared credential
// store, so copying these markers is all a profile needs to reuse an
// existing login. Everything else in the file stays isolated per profile.
var claudeOnboardingFields = []string{"oauthAccount", "hasCompletedOnboarding", "theme"}

// SeedClaudeOnboarding copies the login identity and onboarding markers from
// the user's real ~/.claude.json into the profile's Claude home so launches
// reuse the existing OAuth login instead of rerunning first-time setup.
// Fields already present in the profile are left alone, and a missing Claude
// home or real ~/.claude.json means there is nothing to seed.
func SeedClaudeOnboarding(claudeHome string) error {
	info, err := os.Stat(claudeHome)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect claude home: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("claude home is not a directory")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate existing claude onboarding state: %w", err)
	}
	existing, err := readJSONObject(filepath.Join(home, ".claude.json"))
	if err != nil {
		return fmt.Errorf("read existing claude onboarding state: %w", err)
	}
	if existing == nil {
		return nil
	}

	statePath := filepath.Join(claudeHome, ".claude.json")
	contents, err := os.ReadFile(statePath)
	state := map[string]any{}
	if err == nil {
		if json.Unmarshal(contents, &state) != nil {
			// The profile file is not a JSON object; leave it alone.
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read profile claude state: %w", err)
	}

	changed := false
	for _, field := range claudeOnboardingFields {
		value, ok := existing[field]
		if !ok || value == nil {
			continue
		}
		if current, ok := state[field]; ok && current != nil {
			continue
		}
		state[field] = value
		changed = true
	}
	if !changed {
		return nil
	}

	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile claude state: %w", err)
	}
	if err := os.WriteFile(statePath, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("seed claude onboarding state: %w", err)
	}
	return nil
}

// readJSONObject parses the JSON object at path; a missing or non-JSON file
// means nil.
func readJSONObject(path string) (map[string]any, error) {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if json.Unmarshal(contents, &object) != nil {
		return nil, nil
	}
	return object, nil
}
