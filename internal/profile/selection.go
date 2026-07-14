package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SelectionFile is the per-project marker naming the active profile.
const SelectionFile = ".agentenv"

// ValidName reports whether name is safe to use as a profile directory
// name. It rejects path traversal, surrounding whitespace, and the reserved
// shared credential store.
func ValidName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.EqualFold(name, sharedDirectory) &&
		strings.TrimSpace(name) == name && filepath.Base(name) == name
}

// WriteSelection records name as the active profile for the project at dir.
func WriteSelection(dir, name string) error {
	return os.WriteFile(filepath.Join(dir, SelectionFile), []byte(name+"\n"), 0o600)
}

// FindActive resolves the active profile for start, walking up parent
// directories, and fails when no project selects one.
func FindActive(start string) (string, error) {
	profile, found, err := FindActiveOptional(start)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no active profile; run 'agentenv use <name>'")
	}
	return profile, nil
}

// FindActiveOptional resolves the active profile for start, walking up
// parent directories until a selection file is found.
func FindActiveOptional(start string) (string, bool, error) {
	directory := start
	for {
		contents, err := os.ReadFile(filepath.Join(directory, SelectionFile))
		if err == nil {
			profile := strings.TrimSpace(string(contents))
			if profile == "" {
				return "", false, fmt.Errorf("profile selection %q is empty", filepath.Join(directory, SelectionFile))
			}
			if !ValidName(profile) {
				return "", false, fmt.Errorf("invalid profile name %q in %s", profile, filepath.Join(directory, SelectionFile))
			}
			return profile, true, nil
		}
		if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("read profile selection: %w", err)
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false, nil
		}
		directory = parent
	}
}
