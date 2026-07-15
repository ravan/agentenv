package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

// PrepareHome builds the profile's composed private home: an empty HOME
// containing only symlink aliases into the profile's agent directories, a
// private .agents directory, and, on macOS, links that expose the real login
// keychain. Existing profile-owned entries are preserved, while legacy
// passthrough links into the real home are removed.
func PrepareHome(profilePath string) error {
	profileHome := filepath.Join(profilePath, "home")
	info, err := os.Lstat(profileHome)
	if os.IsNotExist(err) {
		if err := os.Mkdir(profileHome, 0o750); err != nil {
			return fmt.Errorf("create profile home: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect profile home: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("profile home is not a directory")
	}
	if err := removeLegacyHomePassthroughs(profileHome); err != nil {
		return err
	}
	aliases := []struct {
		name   string
		target string
	}{
		{name: ".codex", target: filepath.Join("..", "codex")},
		{name: ".claude", target: filepath.Join("..", "claude")},
		{name: ".claude.json", target: filepath.Join("..", "claude", ".claude.json")},
	}
	for _, alias := range aliases {
		link := filepath.Join(profileHome, alias.name)
		info, err := os.Lstat(link)
		if os.IsNotExist(err) {
			if err := os.Symlink(alias.target, link); err != nil {
				return fmt.Errorf("create %s alias: %w", alias.name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s alias: %w", alias.name, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("reserved home alias %s is not a symbolic link", alias.name)
		}
		target, err := os.Readlink(link)
		if err != nil {
			return fmt.Errorf("read %s alias: %w", alias.name, err)
		}
		if target != alias.target {
			return fmt.Errorf("reserved home alias %s has unexpected target %q", alias.name, target)
		}
	}

	agentsHome := filepath.Join(profileHome, ".agents")
	info, err = os.Lstat(agentsHome)
	if os.IsNotExist(err) {
		if err := os.Mkdir(agentsHome, 0o750); err != nil {
			return fmt.Errorf("create .agents directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect .agents directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("reserved home entry .agents is not a directory")
	}

	return linkSystemKeychain(profileHome)
}

// removeLegacyHomePassthroughs deletes symlinks that older agentenv versions
// created from the profile home into the user's real home, without touching
// their targets.
func removeLegacyHomePassthroughs(profileHome string) error {
	realHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate real home: %w", err)
	}
	entries, err := os.ReadDir(profileHome)
	if err != nil {
		return fmt.Errorf("list profile home: %w", err)
	}
	for _, entry := range entries {
		link := filepath.Join(profileHome, entry.Name())
		info, err := os.Lstat(link)
		if err != nil {
			return fmt.Errorf("inspect profile home entry %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolvedLink, err := filepath.EvalSymlinks(link)
		if err != nil {
			continue
		}
		resolvedRealEntry, err := filepath.EvalSymlinks(filepath.Join(realHome, entry.Name()))
		if err != nil {
			continue
		}
		if filepath.Clean(resolvedLink) != filepath.Clean(resolvedRealEntry) {
			continue
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove legacy home passthrough %q: %w", entry.Name(), err)
		}
	}
	return nil
}
