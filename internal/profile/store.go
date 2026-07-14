// Package profile manages isolated agent profiles on disk: their directory
// layout, per-project selection, configuration, composed private homes, and
// shared credentials.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

// sharedDirectory is the reserved entry under the store root that holds
// credentials shared by every profile.
const sharedDirectory = "shared"

// Store manages the collection of profiles under a single root directory.
type Store struct {
	Root string
}

// Path returns the directory that holds the named profile.
func (s Store) Path(name string) string {
	return filepath.Join(s.Root, name)
}

// Create makes a new isolated profile with per-agent homes, a composed
// private home, and credential links into the shared store.
func (s Store) Create(name string) error {
	if err := os.MkdirAll(s.Root, 0o750); err != nil {
		return fmt.Errorf("create profile root: %w", err)
	}
	profilePath := s.Path(name)
	if err := os.Mkdir(profilePath, 0o750); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("profile %q already exists", name)
		}
		return fmt.Errorf("create profile %q: %w", name, err)
	}
	for _, agent := range Agents {
		if err := os.Mkdir(filepath.Join(profilePath, agent.Name), 0o750); err != nil {
			return fmt.Errorf("create profile %q: %w", name, err)
		}
	}
	if err := PrepareHome(profilePath); err != nil {
		return fmt.Errorf("create profile %q home: %w", name, err)
	}
	if err := os.MkdirAll(filepath.Join(s.Root, sharedDirectory), 0o700); err != nil {
		return fmt.Errorf("create shared credential store: %w", err)
	}
	if err := s.AdoptExistingCredentials(); err != nil {
		return err
	}
	if err := s.EnsureSharedCredentialLinks(profilePath); err != nil {
		return fmt.Errorf("share credentials for profile %q: %w", name, err)
	}
	if err := SeedClaudeOnboarding(filepath.Join(profilePath, "claude")); err != nil {
		return fmt.Errorf("share claude login for profile %q: %w", name, err)
	}
	return nil
}

// Delete removes the named profile and everything inside it.
func (s Store) Delete(name string) error {
	profilePath := s.Path(name)
	if _, err := os.Stat(profilePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q does not exist", name)
		}
		return fmt.Errorf("inspect profile %q: %w", name, err)
	}
	if err := os.RemoveAll(profilePath); err != nil {
		return fmt.Errorf("delete profile %q: %w", name, err)
	}
	return nil
}

// List returns the profile names in directory order, hiding the shared
// credential store. A missing root means no profiles.
func (s Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == sharedDirectory {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// Ensure verifies that the named profile exists and returns its path.
func (s Store) Ensure(name string) (string, error) {
	profilePath := s.Path(name)
	info, err := os.Stat(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("profile %q does not exist; create it with 'agentenv new %s'", name, name)
		}
		return "", fmt.Errorf("inspect profile %q: %w", name, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("profile %q is not a directory", name)
	}
	return profilePath, nil
}

// AgentHome returns the agent's configuration home inside the named profile,
// verifying that the profile provides it.
func (s Store) AgentHome(profile, agent string) (string, error) {
	home := filepath.Join(s.Path(profile), agent)
	info, err := os.Stat(home)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("profile %q does not exist; create it with 'agentenv new %s'", profile, profile)
		}
		return "", fmt.Errorf("inspect profile %q: %w", profile, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s home for profile %q is not a directory", agent, profile)
	}
	return home, nil
}
