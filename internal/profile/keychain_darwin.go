//go:build darwin

package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

// linkSystemKeychain exposes the user's real login keychain inside the
// composed private home. Claude Code on macOS persists its OAuth login in
// the login keychain, which the Security framework locates through
// $HOME/Library; without these links a profiled launch cannot store the
// login ("A keychain cannot be found") and the agent demands a fresh login
// on every start. Sharing the real keychain also keeps one rotating refresh
// token for all profiles instead of divergent copies that trip OAuth
// refresh-token-reuse revocation.
func linkSystemKeychain(profileHome string) error {
	realHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate real home: %w", err)
	}
	preferences := filepath.Join(profileHome, "Library", "Preferences")
	if err := os.MkdirAll(preferences, 0o750); err != nil {
		return fmt.Errorf("create profile Library directories: %w", err)
	}
	links := []struct {
		link   string
		target string
	}{
		{
			link:   filepath.Join(profileHome, "Library", "Keychains"),
			target: filepath.Join(realHome, "Library", "Keychains"),
		},
		{
			link:   filepath.Join(preferences, "com.apple.security.plist"),
			target: filepath.Join(realHome, "Library", "Preferences", "com.apple.security.plist"),
		},
	}
	for _, entry := range links {
		if err := ensureSymlink(entry.link, entry.target); err != nil {
			return err
		}
	}
	return nil
}

// ensureSymlink makes link point at target, replacing a symlink left behind
// by a previous home location. An existing non-link entry is a profile
// corruption the user has to resolve.
func ensureSymlink(link, target string) error {
	info, err := os.Lstat(link)
	if os.IsNotExist(err) {
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("create keychain link %s: %w", link, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect keychain link %s: %w", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("reserved keychain entry %s is not a symbolic link", link)
	}
	current, err := os.Readlink(link)
	if err != nil {
		return fmt.Errorf("read keychain link %s: %w", link, err)
	}
	if current == target {
		return nil
	}
	if err := os.Remove(link); err != nil {
		return fmt.Errorf("replace keychain link %s: %w", link, err)
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("create keychain link %s: %w", link, err)
	}
	return nil
}
