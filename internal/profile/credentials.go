package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

// credentialLink maps an agent's credential file to its shared counterpart
// so OAuth logins are reused across profiles. Agents that keep their login
// in the macOS keychain share it directly through the composed home's
// Library links instead; copying a keychain token into a file would go
// stale as the agent rotates its refresh token, and replaying the stale
// token revokes the whole login.
type credentialLink struct {
	tool        string
	profileFile string
	sharedFile  string
}

var sharedCredentialLinks = []credentialLink{
	{tool: "codex", profileFile: "auth.json", sharedFile: "codex-auth.json"},
	{tool: "claude", profileFile: ".credentials.json", sharedFile: "claude-credentials.json"},
}

// AdoptExistingCredentials seeds the shared credential store from the
// user's real home so existing OAuth logins carry over. Credentials already
// in the shared store are left alone.
func (s Store) AdoptExistingCredentials() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate existing agent credentials: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(s.Root, sharedDirectory), 0o700); err != nil {
		return fmt.Errorf("create shared credential store: %w", err)
	}

	for _, credential := range sharedCredentialLinks {
		sharedPath := filepath.Join(s.Root, sharedDirectory, credential.sharedFile)
		if _, err := os.Lstat(sharedPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect shared %s credentials: %w", credential.tool, err)
		}

		existingPath := filepath.Join(home, "."+credential.tool, credential.profileFile)
		contents, err := os.ReadFile(existingPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("read existing %s credentials: %w", credential.tool, err)
		}
		if err := os.WriteFile(sharedPath, contents, 0o600); err != nil {
			return fmt.Errorf("adopt existing %s credentials: %w", credential.tool, err)
		}
	}
	return nil
}

// EnsureSharedCredentialLinks re-creates missing credential links in the
// profile's agent homes. Agents that delete their credential file on a
// failed login get re-attached to the shared store on the next launch.
func (s Store) EnsureSharedCredentialLinks(profilePath string) error {
	for _, credential := range sharedCredentialLinks {
		agentHome := filepath.Join(profilePath, credential.tool)
		if _, err := os.Stat(agentHome); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect %s home: %w", credential.tool, err)
		}
		link := filepath.Join(agentHome, credential.profileFile)
		if _, err := os.Lstat(link); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect %s credential link: %w", credential.tool, err)
		}
		target := filepath.Join("..", "..", sharedDirectory, credential.sharedFile)
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("share %s credentials: %w", credential.tool, err)
		}
	}
	return nil
}

// RestoreSharedCredential re-links the agent's credential file in agentHome
// to the shared store after a launch. Agents that atomically replace the
// credential symlink with a refreshed file get their refresh copied back to
// the shared store, and shared files are kept private.
func (s Store) RestoreSharedCredential(agentHome, tool string) error {
	var credential credentialLink
	for _, candidate := range sharedCredentialLinks {
		if candidate.tool == tool {
			credential = candidate
			break
		}
	}
	if credential.tool == "" {
		return fmt.Errorf("no shared credential mapping for %q", tool)
	}

	profilePath := filepath.Join(agentHome, credential.profileFile)
	sharedStore := filepath.Join(s.Root, sharedDirectory)
	sharedPath := filepath.Join(sharedStore, credential.sharedFile)
	info, err := os.Lstat(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s credentials: %w", tool, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := os.Chmod(sharedPath, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("secure shared %s credentials: %w", tool, err)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s credential path is neither a file nor a symbolic link", tool)
	}

	contents, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("read refreshed %s credentials: %w", tool, err)
	}
	if err := os.MkdirAll(sharedStore, 0o700); err != nil {
		return fmt.Errorf("create shared credential store: %w", err)
	}
	if err := os.WriteFile(sharedPath, contents, 0o600); err != nil {
		return fmt.Errorf("store refreshed %s credentials: %w", tool, err)
	}
	if err := os.Chmod(sharedPath, 0o600); err != nil {
		return fmt.Errorf("secure shared %s credentials: %w", tool, err)
	}
	if err := os.Remove(profilePath); err != nil {
		return fmt.Errorf("replace refreshed %s credential file: %w", tool, err)
	}
	target, err := filepath.Rel(agentHome, sharedPath)
	if err != nil {
		return fmt.Errorf("locate shared %s credentials: %w", tool, err)
	}
	if err := os.Symlink(target, profilePath); err != nil {
		return fmt.Errorf("restore shared %s credential link: %w", tool, err)
	}
	return nil
}
