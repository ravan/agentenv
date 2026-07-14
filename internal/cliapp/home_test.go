package cliapp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ravan/agentenv/internal/cliapp"
)

func TestNewCreatesOnlyPrivateReservedHomeEntries(t *testing.T) {
	home := t.TempDir()
	profileRoot := filepath.Join(home, "agentenv-state", "profiles")
	for _, path := range []string{".ssh", ".config", ".codex", ".claude"} {
		if err := os.MkdirAll(filepath.Join(home, path), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("host state"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdout:      &bytes.Buffer{},
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}

	profileHome := filepath.Join(profileRoot, "default", "home")
	entries, err := os.ReadDir(profileHome)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if got, want := strings.Join(names, ","), ".agents,.claude,.claude.json,.codex"; got != want {
		t.Fatalf("profile home entries = %q, want %q", got, want)
	}
	for name, target := range map[string]string{
		".codex":       filepath.Join("..", "codex"),
		".claude":      filepath.Join("..", "claude"),
		".claude.json": filepath.Join("..", "claude", ".claude.json"),
	} {
		got, err := os.Readlink(filepath.Join(profileHome, name))
		if err != nil || got != target {
			t.Fatalf("%s alias = %q, %v; want %q", name, got, err, target)
		}
	}
	if info, err := os.Stat(filepath.Join(profileHome, ".agents")); err != nil || !info.IsDir() {
		t.Fatalf("private .agents directory: info=%v err=%v", info, err)
	}
}

func TestAgentLaunchSetsTheComposedHomeEnvironment(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "real-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{realHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := `#!/bin/sh
printf 'home=%s\n' "$HOME"
printf 'userprofile=%s\n' "$USERPROFILE"
printf 'homedrive=%s\n' "$HOMEDRIVE"
printf 'homepath=%s\n' "$HOMEPATH"
printf 'xdg_config=%s\n' "$XDG_CONFIG_HOME"
printf 'xdg_cache=%s\n' "$XDG_CACHE_HOME"
printf 'xdg_data=%s\n' "$XDG_DATA_HOME"
printf 'xdg_state=%s\n' "$XDG_STATE_HOME"
printf 'appdata=%s\n' "$APPDATA"
printf 'localappdata=%s\n' "$LOCALAPPDATA"
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", realHome)
	t.Setenv("USERPROFILE", "wrong-user-profile")
	t.Setenv("HOMEDRIVE", "wrong-drive")
	t.Setenv("HOMEPATH", "wrong-path")
	t.Setenv("XDG_CONFIG_HOME", "wrong-xdg-config")
	t.Setenv("XDG_CACHE_HOME", "wrong-xdg-cache")
	t.Setenv("XDG_DATA_HOME", "wrong-xdg-data")
	t.Setenv("XDG_STATE_HOME", "wrong-xdg-state")
	t.Setenv("APPDATA", "wrong-appdata")
	t.Setenv("LOCALAPPDATA", "wrong-localappdata")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex: %v", err)
	}
	profileHome := filepath.Join(profileRoot, "default", "home")
	volume := filepath.VolumeName(profileHome)
	want := "home=" + profileHome + "\n" +
		"userprofile=" + profileHome + "\n" +
		"homedrive=" + volume + "\n" +
		"homepath=" + strings.TrimPrefix(profileHome, volume) + "\n" +
		"xdg_config=" + filepath.Join(profileHome, ".config") + "\n" +
		"xdg_cache=" + filepath.Join(profileHome, ".cache") + "\n" +
		"xdg_data=" + filepath.Join(profileHome, ".local", "share") + "\n" +
		"xdg_state=" + filepath.Join(profileHome, ".local", "state") + "\n" +
		"appdata=" + filepath.Join(profileHome, "AppData", "Roaming") + "\n" +
		"localappdata=" + filepath.Join(profileHome, "AppData", "Local") + "\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAgentLaunchDoesNotExposeRealHomeFiles(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for path, contents := range map[string]string{
		".gitconfig":                                 "[user]\n\tname = Host User\n",
		filepath.Join(".ssh", "config"):              "Host private\n",
		filepath.Join(".config", "tool", "settings"): "host settings\n",
	} {
		fullPath := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := `#!/bin/sh
test ! -e "$HOME/.gitconfig" || exit 1
test ! -e "$HOME/.ssh" || exit 1
test ! -e "$HOME/.config/tool/settings" || exit 1
if [ "$HOME" = "$USERPROFILE" ]; then
	printf 'userprofile=matches\n'
else
	printf 'userprofile=%s\n' "$USERPROFILE"
fi
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", filepath.Join(root, "wrong-user-profile"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex: %v", err)
	}
	want := "userprofile=matches\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAgentLaunchUpgradesProfilesCreatedBeforeComposedHomes(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".agents"),
		filepath.Join(profileRoot, "legacy", "codex"),
		filepath.Join(profileRoot, "legacy", "claude"),
		filepath.Join(profileRoot, "legacy", "home"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("host Claude state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profileHome := filepath.Join(profileRoot, "legacy", "home")
	for _, name := range []string{".codex", ".claude", ".claude.json", ".agents"} {
		if err := os.Symlink(filepath.Join(home, name), filepath.Join(profileHome, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeCodex := `#!/bin/sh
test "$(readlink "$HOME/.codex")" = ../codex || exit 1
test "$(readlink "$HOME/.claude")" = ../claude || exit 1
test "$(readlink "$HOME/.claude.json")" = ../claude/.claude.json || exit 1
test -d "$HOME/.agents" || exit 1
printf 'upgraded\n'
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex with legacy profile: %v", err)
	}
	if got, want := stdout.String(), "upgraded\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	for _, name := range []string{".codex", ".claude", ".claude.json", ".agents"} {
		if _, err := os.Lstat(filepath.Join(home, name)); err != nil {
			t.Fatalf("real-home target %q was removed: %v", name, err)
		}
	}
}

func TestAgentLaunchRemovesLegacyHomePassthroughsWithoutRemovingTargets(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	profileHome := filepath.Join(profileRoot, "default", "home")
	legacyTargets := map[string]string{
		".gitconfig": "host git config\n",
		".toolrc":    "host tool config\n",
	}
	for name, contents := range legacyTargets {
		target := filepath.Join(home, name)
		if err := os.WriteFile(target, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(profileHome, name)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex: %v", err)
	}
	for name, contents := range legacyTargets {
		if _, err := os.Lstat(filepath.Join(profileHome, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy passthrough %q still exists (lstat error: %v)", name, err)
		}
		targetContents, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || string(targetContents) != contents {
			t.Fatalf("real-home target %q = %q, %v; want %q", name, targetContents, err, contents)
		}
	}
}

func TestAgentLaunchDoesNotReplaceProfileOwnedHomeEntries(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{".toolrc", ".linkedrc"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("global\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\ncat \"$HOME/.toolrc\"\ncat \"$HOME/.linkedrc\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	profileHome := filepath.Join(profileRoot, "default", "home")
	localFile := filepath.Join(profileHome, ".toolrc")
	if err := os.Remove(localFile); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(localFile, []byte("profile file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localTarget := filepath.Join(profileHome, "local-target")
	if err := os.WriteFile(localTarget, []byte("profile link\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localLink := filepath.Join(profileHome, ".linkedrc")
	if err := os.Remove(localLink); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink("local-target", localLink); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex: %v", err)
	}
	if got, want := stdout.String(), "profile file\nprofile link\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAgentLaunchRejectsAnUnexpectedReservedHomeAlias(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	marker := filepath.Join(root, "agent-ran")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := "#!/bin/sh\nprintf ran > \"" + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	alias := filepath.Join(profileRoot, "default", "home", ".codex")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "claude"), alias); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := command.Run(context.Background(), []string{"agentenv", "codex"})
	if err == nil || !strings.Contains(err.Error(), "unexpected target") {
		t.Fatalf("launch error = %v, want an unexpected-target error", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("agent ran despite unsafe reserved alias (stat error: %v)", err)
	}
}

func TestAgentLaunchRejectsASymlinkedComposedHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	profilePath := filepath.Join(profileRoot, "legacy")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	marker := filepath.Join(root, "agent-ran")
	for _, path := range []string{
		home,
		filepath.Join(profilePath, "codex"),
		filepath.Join(profilePath, "claude"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(home, filepath.Join(profilePath, "home")); err != nil {
		t.Fatal(err)
	}
	fakeCodex := "#!/bin/sh\nprintf ran > \"" + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})

	err := command.Run(context.Background(), []string{"agentenv", "codex"})
	if err == nil || !strings.Contains(err.Error(), "profile home is not a directory") {
		t.Fatalf("launch error = %v, want a profile-home safety error", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("agent ran despite symlinked profile home (stat error: %v)", err)
	}
	for _, path := range []string{".codex", ".claude", ".claude.json", ".agents"} {
		if _, err := os.Lstat(filepath.Join(home, path)); !os.IsNotExist(err) {
			t.Fatalf("profile preparation escaped into the real home at %q (lstat error: %v)", path, err)
		}
	}
}

func TestAgentLaunchDoesNotImportGlobalAgentState(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(home, ".codex", "plugins", "leaked", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".agents", "skills"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	globalAgentFiles := []string{
		filepath.Join(home, ".codex", "plugins", "leaked", "skills", "skill.md"),
		filepath.Join(home, ".claude", "skills", "leaked.md"),
		filepath.Join(home, ".agents", "skills", "leaked.md"),
	}
	for _, path := range globalAgentFiles {
		if err := os.WriteFile(path, []byte("global agent state\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := `#!/bin/sh
test ! -e "$HOME/.codex/plugins/leaked/skills/skill.md" || exit 1
test ! -e "$HOME/.claude/skills/leaked.md" || exit 1
test ! -e "$HOME/.agents/skills/leaked.md" || exit 1
printf 'isolated\n'
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch Codex: %v", err)
	}
	if got, want := stdout.String(), "isolated\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	for _, path := range globalAgentFiles {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != "global agent state\n" {
			t.Fatalf("global agent state %q = %q, %v; want unchanged", path, contents, err)
		}
	}
}

func TestWrappedAgentsDoNotChangeDirectAgentHomes(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".claude"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	directFiles := map[string]string{
		filepath.Join(home, ".codex", "direct.txt"):  "direct Codex\n",
		filepath.Join(home, ".claude", "direct.txt"): "direct Claude\n",
		filepath.Join(home, ".claude.json"):          "direct Claude root\n",
	}
	for path, contents := range directFiles {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nprintf wrapped > \"$HOME/.codex/wrapped.txt\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeClaude := "#!/bin/sh\nprintf wrapped > \"$HOME/.claude/wrapped.txt\"\nprintf wrapped > \"$HOME/.claude.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	for _, agent := range []string{"codex", "claude"} {
		if err := command.Run(context.Background(), []string{"agentenv", agent}); err != nil {
			t.Fatalf("launch %s: %v", agent, err)
		}
	}
	for path, want := range directFiles {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read direct agent file %q: %v", path, err)
		}
		if got := string(contents); got != want {
			t.Fatalf("direct agent file %q = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{
		filepath.Join(home, ".codex", "wrapped.txt"),
		filepath.Join(home, ".claude", "wrapped.txt"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("wrapped agent changed direct home at %q (stat error: %v)", path, err)
		}
	}
}

func TestCodexPluginsInstalledThroughHomeAreIsolatedPerProfile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := `#!/bin/sh
skill="$HOME/.codex/plugins/private/skills/private.md"
if [ "$1" = install ]; then
	mkdir -p "$(dirname "$skill")"
	printf 'private skill' > "$skill"
fi
if [ -e "$skill" ]; then
	printf 'skill=present\n'
else
	printf 'skill=absent\n'
fi
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	for _, profile := range []string{"first", "second"} {
		if err := command.Run(context.Background(), []string{"agentenv", "new", profile}); err != nil {
			t.Fatalf("new profile %q: %v", profile, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex", "install"}); err != nil {
		t.Fatalf("install Codex plugin: %v", err)
	}
	if got, want := stdout.String(), "skill=present\n"; got != want {
		t.Fatalf("first profile output = %q, want %q", got, want)
	}

	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex", "check"}); err != nil {
		t.Fatalf("check Codex plugin isolation: %v", err)
	}
	if got, want := stdout.String(), "skill=absent\n"; got != want {
		t.Fatalf("second profile output = %q, want %q", got, want)
	}
}

func TestClaudeSkillsInstalledThroughHomeAreIsolatedPerProfile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeClaude := `#!/bin/sh
home_skill="$HOME/.claude/skills/private.md"
config_skill="$CLAUDE_CONFIG_DIR/skills/private.md"
if [ "$1" = install ]; then
	mkdir -p "$(dirname "$home_skill")"
	printf 'private skill' > "$home_skill"
fi
if [ -e "$config_skill" ]; then
	printf 'skill=present\n'
else
	printf 'skill=absent\n'
fi
`
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	for _, profile := range []string{"first", "second"} {
		if err := command.Run(context.Background(), []string{"agentenv", "new", profile}); err != nil {
			t.Fatalf("new profile %q: %v", profile, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "claude", "install"}); err != nil {
		t.Fatalf("install Claude skill: %v", err)
	}
	if got, want := stdout.String(), "skill=present\n"; got != want {
		t.Fatalf("first profile output = %q, want %q", got, want)
	}

	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "claude", "check"}); err != nil {
		t.Fatalf("check Claude skill isolation: %v", err)
	}
	if got, want := stdout.String(), "skill=absent\n"; got != want {
		t.Fatalf("second profile output = %q, want %q", got, want)
	}
}

func TestAgentSkillsInstalledThroughHomeAreIsolatedPerProfile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{home, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := `#!/bin/sh
if [ ! -d "$HOME/.agents" ]; then
	printf 'missing profile .agents directory\n' >&2
	exit 1
fi
skill="$HOME/.agents/skills/private.md"
if [ "$1" = install ]; then
	mkdir -p "$(dirname "$skill")"
	printf 'private skill' > "$skill"
fi
if [ -e "$skill" ]; then
	printf 'skill=present\n'
else
	printf 'skill=absent\n'
fi
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	for _, profile := range []string{"first", "second"} {
		if err := command.Run(context.Background(), []string{"agentenv", "new", profile}); err != nil {
			t.Fatalf("new profile %q: %v", profile, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex", "install"}); err != nil {
		t.Fatalf("install shared agent skill: %v (%s)", err, stderr.String())
	}
	if got, want := stdout.String(), "skill=present\n"; got != want {
		t.Fatalf("first profile output = %q, want %q", got, want)
	}

	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex", "check"}); err != nil {
		t.Fatalf("check shared agent skill isolation: %v (%s)", err, stderr.String())
	}
	if got, want := stdout.String(), "skill=absent\n"; got != want {
		t.Fatalf("second profile output = %q, want %q", got, want)
	}
}
