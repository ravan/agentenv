package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// skillLinkDirs lists the directories inside a profile that agents read
// skills from: Claude's config-home skills and the agentskills.io standard
// location in the composed private home.
func skillLinkDirs(profilePath string) []string {
	return []string{
		filepath.Join(profilePath, "claude", "skills"),
		filepath.Join(profilePath, "home", ".agents", "skills"),
	}
}

// SyncSkills reconciles the profile's skill links against its enabled list:
// every enabled skill gets a symlink into canonicalDir (the golden skills
// store) and links into the store that are no longer enabled are removed.
// Entries the user created themselves are left alone. It returns the
// enabled skills that are missing from the store.
func SyncSkills(profilePath, canonicalDir string, enabled []string) ([]string, error) {
	canonical := filepath.Clean(canonicalDir)
	present := map[string]bool{}
	var missing []string
	for _, name := range enabled {
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
			return nil, fmt.Errorf("invalid skill name %q in profile configuration", name)
		}
		if info, err := os.Stat(filepath.Join(canonical, name)); err == nil && info.IsDir() {
			present[name] = true
		} else {
			missing = append(missing, name)
		}
	}

	for _, directory := range skillLinkDirs(profilePath) {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return nil, fmt.Errorf("create skill directory: %w", err)
		}
		if err := removeStaleSkillLinks(directory, canonical, present); err != nil {
			return nil, err
		}
		for _, name := range enabled {
			if !present[name] {
				continue
			}
			if err := ensureSkillLink(directory, canonical, name); err != nil {
				return nil, err
			}
		}
	}
	return missing, nil
}

// removeStaleSkillLinks deletes symlinks in directory that point into the
// golden store but are not enabled anymore, including links left dangling
// by skills that were removed upstream. Anything that is not a symlink into
// the store is user-owned and kept.
func removeStaleSkillLinks(directory, canonical string, present map[string]bool) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("list skill links: %w", err)
	}
	for _, entry := range entries {
		link := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(link)
		if err != nil {
			return fmt.Errorf("inspect skill link %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := resolveSkillLink(directory, link)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(target, canonical+string(filepath.Separator)) {
			continue
		}
		if present[entry.Name()] && target == filepath.Join(canonical, entry.Name()) {
			continue
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove stale skill link %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// ensureSkillLink creates the symlink for one enabled skill, preferring a
// relative target so the profile store stays relocatable. An existing entry
// is either the link removeStaleSkillLinks already vetted or a user-owned
// file, and is left alone either way.
func ensureSkillLink(directory, canonical, name string) error {
	link := filepath.Join(directory, name)
	if _, err := os.Lstat(link); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect skill link %q: %w", name, err)
	}
	source := filepath.Join(canonical, name)
	target, err := filepath.Rel(directory, source)
	if err != nil {
		target = source
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("link skill %q: %w", name, err)
	}
	return nil
}

// resolveSkillLink resolves a symlink's target lexically against the
// directory that holds it, without requiring the target to exist.
func resolveSkillLink(directory, link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("read skill link %q: %w", filepath.Base(link), err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(directory, target)
	}
	return filepath.Clean(target), nil
}
