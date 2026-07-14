package cliapp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ravan/agentenv/internal/cliapp"
)

func TestFirstProfileAdoptsExistingFileBasedOAuthCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTENV_HOME", "")

	existingCredentials := map[string][]byte{
		filepath.Join(".codex", "auth.json"):          []byte(`{"tokens":{"access_token":"existing-codex"}}`),
		filepath.Join(".claude", ".credentials.json"): []byte(`{"claudeAiOauth":{"accessToken":"existing-claude"}}`),
	}
	for path, credentials := range existingCredentials {
		credentialPath := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(credentialPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(credentialPath, credentials, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	command := cliapp.New(cliapp.Options{Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}

	profileCredentials := map[string][]byte{
		filepath.Join("codex", "auth.json"):          existingCredentials[filepath.Join(".codex", "auth.json")],
		filepath.Join("claude", ".credentials.json"): existingCredentials[filepath.Join(".claude", ".credentials.json")],
	}
	for path, want := range profileCredentials {
		got, err := os.ReadFile(filepath.Join(home, ".agent-profiles", "default", path))
		if err != nil {
			t.Fatalf("read adopted credentials %q: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("adopted credentials %q = %q, want %q", path, got, want)
		}
	}
}

func TestFirstProfileAdoptsKeychainClaudeCredentials(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credential := `{"claudeAiOauth":{"accessToken":"keychain-claude"}}`
	fakeSecurity := "#!/bin/sh\ntest \"$1\" = find-generic-password || exit 1\nprintf '%s\\n' '" + credential + "'\n"
	if err := os.WriteFile(filepath.Join(binDir, "security"), []byte(fakeSecurity), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("AGENTENV_HOME", "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	command := cliapp.New(cliapp.Options{Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, ".agent-profiles", "default", "claude", ".credentials.json"))
	if err != nil {
		t.Fatalf("read adopted credentials: %v", err)
	}
	if string(got) != credential {
		t.Fatalf("adopted credentials = %q, want %q", got, credential)
	}
}

func TestNewProfileSeedsClaudeLoginWithoutImportingDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTENV_HOME", "")

	existingDefaults := map[string][]byte{
		filepath.Join(".claude", "settings.json"): []byte(`{"model":"sonnet"}`),
		".claude.json": []byte(`{"hasCompletedOnboarding":true,"oauthAccount":{"emailAddress":"user@example.com"},"projects":{"/private":{}}}`),
	}
	for path, contents := range existingDefaults {
		defaultPath := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(defaultPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(defaultPath, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	command := cliapp.New(cliapp.Options{Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}

	settingsPath := filepath.Join(home, ".agent-profiles", "default", "claude", "settings.json")
	if _, err := os.Lstat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("profile imported Claude settings.json (lstat error: %v)", err)
	}
	state := readClaudeState(t, filepath.Join(home, ".agent-profiles", "default", "claude", ".claude.json"))
	if state["hasCompletedOnboarding"] != true {
		t.Fatalf("seeded hasCompletedOnboarding = %v, want true", state["hasCompletedOnboarding"])
	}
	account, ok := state["oauthAccount"].(map[string]any)
	if !ok || account["emailAddress"] != "user@example.com" {
		t.Fatalf("seeded oauthAccount = %v, want existing login", state["oauthAccount"])
	}
	if _, ok := state["projects"]; ok {
		t.Fatalf("profile imported per-project Claude state: %v", state["projects"])
	}
	for path, want := range existingDefaults {
		got, err := os.ReadFile(filepath.Join(home, path))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("real-home Claude default %q = %q, %v; want %q", path, got, err, want)
		}
	}
}

func readClaudeState(t *testing.T, path string) map[string]any {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded Claude state: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(contents, &state); err != nil {
		t.Fatalf("parse seeded Claude state: %v", err)
	}
	return state
}

func TestCodexLaunchAdoptsExistingOAuthForAnExistingProfile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(home, ".codex"),
		filepath.Join(profileRoot, "default", "codex"),
		filepath.Join(profileRoot, "shared"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"token":"existing-codex"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "shared", "codex-auth.json"), filepath.Join(profileRoot, "default", "codex", "auth.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeCodex := "#!/bin/sh\ngrep -q existing-codex \"$CODEX_HOME/auth.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, WorkingDir: projectRoot, Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex with adopted OAuth: %v", err)
	}
}

func TestClaudeLaunchSeedsLoginIntoAnExistingProfile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(home, ".claude"),
		filepath.Join(profileRoot, "default", "claude"),
		filepath.Join(profileRoot, "shared"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"setting":"inherited"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"theme":"existing-theme","oauthAccount":{"emailAddress":"user@example.com"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeClaude := "#!/bin/sh\ntest ! -e \"$CLAUDE_CONFIG_DIR/settings.json\" && grep -q existing-theme \"$CLAUDE_CONFIG_DIR/.claude.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, WorkingDir: projectRoot, Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "claude"}); err != nil {
		t.Fatalf("launch Claude with seeded login: %v", err)
	}
	state := readClaudeState(t, filepath.Join(profileRoot, "default", "claude", ".claude.json"))
	account, ok := state["oauthAccount"].(map[string]any)
	if !ok || account["emailAddress"] != "user@example.com" {
		t.Fatalf("seeded oauthAccount = %v, want existing login", state["oauthAccount"])
	}
	for path, want := range map[string]string{
		filepath.Join(home, ".claude", "settings.json"): `{"setting":"inherited"}`,
		filepath.Join(home, ".claude.json"):             `{"theme":"existing-theme","oauthAccount":{"emailAddress":"user@example.com"}}`,
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("real-home Claude default %q = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestClaudeLaunchKeepsProfileOnboardingStateAuthoritative(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		home,
		filepath.Join(profileRoot, "default", "claude"),
		filepath.Join(profileRoot, "shared"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"theme":"real-theme","oauthAccount":{"emailAddress":"user@example.com"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "default", "claude", ".claude.json"), []byte(`{"theme":"profile-theme"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, WorkingDir: projectRoot, Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "claude"}); err != nil {
		t.Fatalf("launch Claude: %v", err)
	}
	state := readClaudeState(t, filepath.Join(profileRoot, "default", "claude", ".claude.json"))
	if state["theme"] != "profile-theme" {
		t.Fatalf("profile theme = %v, want profile-theme kept over real-home value", state["theme"])
	}
	account, ok := state["oauthAccount"].(map[string]any)
	if !ok || account["emailAddress"] != "user@example.com" {
		t.Fatalf("seeded oauthAccount = %v, want existing login", state["oauthAccount"])
	}
}

func TestNewProfilesShareFileBasedOAuthCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})
	for _, name := range []string{"focused", "security-review"} {
		if err := command.Run(context.Background(), []string{"agentenv", "new", name}); err != nil {
			t.Fatalf("new profile %q: %v", name, err)
		}
	}

	codexCredentials := []byte(`{"tokens":{"access_token":"shared-codex"}}`)
	if err := os.WriteFile(filepath.Join(profileRoot, "focused", "codex", "auth.json"), codexCredentials, 0o600); err != nil {
		t.Fatalf("write Codex credentials: %v", err)
	}
	gotCodex, err := os.ReadFile(filepath.Join(profileRoot, "security-review", "codex", "auth.json"))
	if err != nil {
		t.Fatalf("read Codex credentials through second profile: %v", err)
	}
	if !bytes.Equal(gotCodex, codexCredentials) {
		t.Fatalf("Codex credentials = %q, want %q", gotCodex, codexCredentials)
	}

	claudeCredentials := []byte(`{"claudeAiOauth":{"accessToken":"shared-claude"}}`)
	if err := os.WriteFile(filepath.Join(profileRoot, "security-review", "claude", ".credentials.json"), claudeCredentials, 0o600); err != nil {
		t.Fatalf("write Claude credentials: %v", err)
	}
	gotClaude, err := os.ReadFile(filepath.Join(profileRoot, "focused", "claude", ".credentials.json"))
	if err != nil {
		t.Fatalf("read Claude credentials through first profile: %v", err)
	}
	if !bytes.Equal(gotClaude, claudeCredentials) {
		t.Fatalf("Claude credentials = %q, want %q", gotClaude, claudeCredentials)
	}
}

func TestClaudeLaunchRestoresADeletedCredentialLink(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, WorkingDir: projectRoot, Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}

	// A failed login makes Claude delete its credential file, which unlinks
	// the profile from the shared store.
	if err := os.Remove(filepath.Join(profileRoot, "default", "claude", ".credentials.json")); err != nil {
		t.Fatal(err)
	}
	credential := `{"claudeAiOauth":{"accessToken":"shared-claude"}}`
	if err := os.WriteFile(filepath.Join(profileRoot, "shared", "claude-credentials.json"), []byte(credential), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeClaude := "#!/bin/sh\ngrep -q shared-claude \"$CLAUDE_CONFIG_DIR/.credentials.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := command.Run(context.Background(), []string{"agentenv", "claude"}); err != nil {
		t.Fatalf("launch Claude with restored credential link: %v", err)
	}
}

func TestAgentLaunchPreservesSharingWhenCredentialsAreAtomicallyReplaced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})
	for _, name := range []string{"first", "second"} {
		if err := command.Run(context.Background(), []string{"agentenv", "new", name}); err != nil {
			t.Fatalf("new profile %q: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials := `{"tokens":{"access_token":"refreshed"}}`
	fakeCodex := "#!/bin/sh\ntmp=\"$CODEX_HOME/auth.json.tmp\"\nprintf '%s' '" + credentials + "' > \"$tmp\"\nmv \"$tmp\" \"$CODEX_HOME/auth.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch codex: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(profileRoot, "second", "codex", "auth.json"))
	if err != nil {
		t.Fatalf("read refreshed credentials through second profile: %v", err)
	}
	if string(got) != credentials {
		t.Fatalf("refreshed credentials = %q, want %q", got, credentials)
	}
	info, err := os.Lstat(filepath.Join(profileRoot, "first", "codex", "auth.json"))
	if err != nil {
		t.Fatalf("inspect first profile credential link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("first profile credential path was not relinked after atomic replacement")
	}
}

func TestAgentLaunchSecuresSharedOAuthFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sharedPath := filepath.Join(profileRoot, "shared", "codex-auth.json")
	if err := os.WriteFile(filepath.Join(profileRoot, "default", "codex", "auth.json"), []byte(`{"tokens":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sharedPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch codex: %v", err)
	}
	info, err := os.Stat(sharedPath)
	if err != nil {
		t.Fatalf("inspect shared credentials: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("shared credential permissions = %o, want %o", got, want)
	}
}
