package cliapp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ravan/agentenv/internal/cliapp"
)

func TestNewCreatesAnIsolatedProfile(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdout:      &stdout,
	})

	err := command.Run(context.Background(), []string{"agentenv", "new", "superpowers"})
	if err != nil {
		t.Fatalf("new profile: %v", err)
	}

	for _, tool := range []string{"codex", "claude"} {
		path := filepath.Join(profileRoot, "superpowers", tool)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("profile directory %q: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("profile path %q is not a directory", path)
		}
	}

	if got, want := stdout.String(), "Created profile superpowers\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestListPrintsProfilesInNameOrder(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	for _, name := range []string{"security-review", "default"} {
		if err := os.MkdirAll(filepath.Join(profileRoot, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "README.txt"), []byte("not a profile"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdout:      &stdout,
	})

	err := command.Run(context.Background(), []string{"agentenv", "list"})
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}

	if got, want := stdout.String(), "default\nsecurity-review\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestDeleteRemovesAProfile(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	profilePath := filepath.Join(profileRoot, "old", "codex")
	if err := os.MkdirAll(profilePath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilePath, "config.toml"), []byte("model = \"test\""), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdout:      &stdout,
	})

	err := command.Run(context.Background(), []string{"agentenv", "delete", "old"})
	if err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profileRoot, "old")); !os.IsNotExist(err) {
		t.Fatalf("deleted profile still exists (stat error: %v)", err)
	}
	if got, want := stdout.String(), "Deleted profile old\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestUseActivatesAnExistingProfileForTheProject(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(profileRoot, "superpowers"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})

	err := command.Run(context.Background(), []string{"agentenv", "use", "superpowers"})
	if err != nil {
		t.Fatalf("activate profile: %v", err)
	}

	selection, err := os.ReadFile(filepath.Join(projectRoot, ".agentenv"))
	if err != nil {
		t.Fatalf("read project selection: %v", err)
	}
	if got, want := string(selection), "superpowers\n"; got != want {
		t.Fatalf("selection = %q, want %q", got, want)
	}
	if got, want := stdout.String(), "Using profile superpowers\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCodexLaunchUsesTheProjectProfileFromAParentDirectory(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	workingDir := filepath.Join(projectRoot, "nested")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(profileRoot, "superpowers", "codex"),
		workingDir,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("superpowers\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeCodex := "#!/bin/sh\nprintf 'home=%s\\n' \"$CODEX_HOME\"\nprintf 'args=%s\\n' \"$*\"\nprintf 'pwd=%s\\n' \"$PWD\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "wrong-home"))
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  workingDir,
		Stdout:      &stdout,
	})

	err := command.Run(context.Background(), []string{"agentenv", "codex", "--model", "gpt-5"})
	if err != nil {
		t.Fatalf("launch codex: %v", err)
	}

	physicalWorkingDir, err := filepath.EvalSymlinks(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	want := "home=" + filepath.Join(profileRoot, "superpowers", "codex") + "\n" +
		"args=--model gpt-5\n" +
		"pwd=" + physicalWorkingDir + "\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunLaunchesAWrapperInsideTheSelectedCodexProfile(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeHeadroom := `#!/bin/sh
printf 'codex_home=%s\n' "$CODEX_HOME"
printf 'home=%s\n' "$HOME"
printf 'agentenv_home=%s\n' "$AGENTENV_HOME"
printf 'args=%s\n' "$*"
`
	if err := os.WriteFile(filepath.Join(binDir, "headroom"), []byte(fakeHeadroom), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "superpowers"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "superpowers"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	stdout.Reset()

	err := command.Run(context.Background(), []string{
		"agentenv", "run", "codex", "--", "headroom", "wrap", "codex", "--yolo",
	})
	if err != nil {
		t.Fatalf("run profiled wrapper: %v", err)
	}

	profilePath := filepath.Join(profileRoot, "superpowers")
	want := "codex_home=" + filepath.Join(profilePath, "codex") + "\n" +
		"home=" + filepath.Join(profilePath, "home") + "\n" +
		"agentenv_home=" + profileRoot + "\n" +
		"args=wrap codex --yolo\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCodexPluginRunsTheCodexPluginCommandInsideTheSelectedProfile(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeCodex := `#!/bin/sh
printf 'codex_home=%s\n' "$CODEX_HOME"
printf 'home=%s\n' "$HOME"
printf 'args=%s\n' "$*"
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "superpowers"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "superpowers"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	stdout.Reset()

	err := command.Run(context.Background(), []string{
		"agentenv", "codex-plugin", "add", "private@agentenv-test", "--json",
	})
	if err != nil {
		t.Fatalf("run profiled plugin command: %v", err)
	}

	profilePath := filepath.Join(profileRoot, "superpowers")
	want := "codex_home=" + filepath.Join(profilePath, "codex") + "\n" +
		"home=" + filepath.Join(profilePath, "home") + "\n" +
		"args=plugin add private@agentenv-test --json\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCodexGuardStopsAnUnprofiledLaunchInASelectedProject(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(profileRoot, "default", "codex"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", filepath.Join(root, "global-codex-home"))
	input := `{"cwd":` + strconv.Quote(projectRoot) + `,"hook_event_name":"SessionStart","source":"startup"}`
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdin:       strings.NewReader(input),
		Stdout:      &stdout,
	})

	if err := command.Run(context.Background(), []string{"agentenv", "guard", "codex"}); err != nil {
		t.Fatalf("guard Codex launch: %v", err)
	}
	var result struct {
		Continue      bool   `json:"continue"`
		StopReason    string `json:"stopReason"`
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode guard output %q: %v", stdout.String(), err)
	}
	if result.Continue {
		t.Fatalf("guard allowed an unprofiled Codex launch: %+v", result)
	}
	wantHome := filepath.Join(profileRoot, "default", "codex")
	if !strings.Contains(result.StopReason, wantHome) || !strings.Contains(result.SystemMessage, "agentenv codex") {
		t.Fatalf("guard result = %+v, want expected home %q and relaunch guidance", result, wantHome)
	}
}

func TestCodexGuardStopsALaunchWithTheWrongPrivateHome(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	profilePath := filepath.Join(profileRoot, "default")
	if err := os.MkdirAll(filepath.Join(profilePath, "codex"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", filepath.Join(profilePath, "codex"))
	t.Setenv("HOME", filepath.Join(root, "global-home"))
	input := `{"cwd":` + strconv.Quote(projectRoot) + `,"hook_event_name":"SessionStart","source":"resume"}`
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdin:       strings.NewReader(input),
		Stdout:      &stdout,
	})

	if err := command.Run(context.Background(), []string{"agentenv", "guard", "codex"}); err != nil {
		t.Fatalf("guard Codex launch: %v", err)
	}
	var result struct {
		Continue   bool   `json:"continue"`
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode guard output %q: %v", stdout.String(), err)
	}
	if result.Continue || !strings.Contains(result.StopReason, filepath.Join(profilePath, "home")) {
		t.Fatalf("guard result = %+v, want rejection naming the private home", result)
	}
}

func TestCodexGuardAllowsGlobalLaunchesOutsideSelectedProjects(t *testing.T) {
	projectRoot := t.TempDir()
	input := `{"cwd":` + strconv.Quote(projectRoot) + `,"hook_event_name":"SessionStart","source":"startup"}`
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: filepath.Join(t.TempDir(), "profiles"),
		Stdin:       strings.NewReader(input),
		Stdout:      &stdout,
	})

	if err := command.Run(context.Background(), []string{"agentenv", "guard", "codex"}); err != nil {
		t.Fatalf("guard global Codex launch: %v", err)
	}
	var result struct {
		Continue bool `json:"continue"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode guard output %q: %v", stdout.String(), err)
	}
	if !result.Continue {
		t.Fatalf("guard stopped Codex outside an agentenv project")
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

func TestClaudeLaunchUsesTheProfileConfigurationDirectory(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(profileRoot, "focused", "claude"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("focused\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeClaude := "#!/bin/sh\nprintf 'config=%s\\n' \"$CLAUDE_CONFIG_DIR\"\nprintf 'args=%s\\n' \"$*\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "wrong-config"))
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})

	err := command.Run(context.Background(), []string{"agentenv", "claude", "--permission-mode", "plan"})
	if err != nil {
		t.Fatalf("launch claude: %v", err)
	}

	want := "config=" + filepath.Join(profileRoot, "focused", "claude") + "\n" +
		"args=--permission-mode plan\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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

func TestConcurrentProfilesResolveDistinctAgentRoots(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profileRoot := filepath.Join(root, "profiles")
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nprintf '%s|%s\\n' \"$HOME\" \"$CODEX_HOME\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	admin := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})
	projects := make(map[string]string)
	for _, profile := range []string{"first", "second"} {
		if err := admin.Run(context.Background(), []string{"agentenv", "new", profile}); err != nil {
			t.Fatalf("new profile %q: %v", profile, err)
		}
		project := filepath.Join(root, profile+"-project")
		if err := os.MkdirAll(project, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, ".agentenv"), []byte(profile+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		projects[profile] = project
	}

	type launchResult struct {
		profile string
		output  string
		err     error
	}
	results := make(chan launchResult, len(projects))
	for profile, project := range projects {
		go func() {
			var stdout bytes.Buffer
			command := cliapp.New(cliapp.Options{
				ProfileRoot: profileRoot,
				WorkingDir:  project,
				Stdout:      &stdout,
			})
			err := command.Run(context.Background(), []string{"agentenv", "codex"})
			results <- launchResult{profile: profile, output: stdout.String(), err: err}
		}()
	}
	for range projects {
		result := <-results
		if result.err != nil {
			t.Fatalf("launch profile %q: %v", result.profile, result.err)
		}
		profilePath := filepath.Join(profileRoot, result.profile)
		want := filepath.Join(profilePath, "home") + "|" + filepath.Join(profilePath, "codex") + "\n"
		if result.output != want {
			t.Fatalf("profile %q output = %q, want %q", result.profile, result.output, want)
		}
	}
}

func TestNewUsesAgentenvHomeWhenNoProfileRootIsConfigured(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "portable-profiles")
	t.Setenv("AGENTENV_HOME", profileRoot)
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{Stdout: &stdout})

	err := command.Run(context.Background(), []string{"agentenv", "new", "portable"})
	if err != nil {
		t.Fatalf("new profile: %v", err)
	}
	for _, tool := range []string{"codex", "claude"} {
		if info, err := os.Stat(filepath.Join(profileRoot, "portable", tool)); err != nil || !info.IsDir() {
			t.Fatalf("%s profile directory was not created (info: %v, error: %v)", tool, info, err)
		}
	}
}

func TestAgentLaunchConnectsStandardStreams(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{
		filepath.Join(profileRoot, "interactive", "codex"),
		projectRoot,
		binDir,
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("interactive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeCodex := "#!/bin/sh\nread input\nprintf 'received=%s\\n' \"$input\"\nprintf 'diagnostic=%s\\n' \"$input\" >&2\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdin:       strings.NewReader("hello agent\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
	})

	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch codex: %v", err)
	}
	if got, want := stdout.String(), "received=hello agent\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "diagnostic=hello agent\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestNewRejectsAnExistingProfileWithoutChangingIt(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	configPath := filepath.Join(profileRoot, "existing", "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"keep-me\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})
	err := command.Run(context.Background(), []string{"agentenv", "new", "existing"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("new existing profile error = %v, want an already-exists error", err)
	}

	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), "model = \"keep-me\"\n"; got != want {
		t.Fatalf("existing config = %q, want %q", got, want)
	}
}

func TestDeleteRejectsAProfileNameOutsideTheProfileRoot(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	outsidePath := filepath.Join(root, "outside")
	if err := os.MkdirAll(outsidePath, 0o750); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outsidePath, "keep")
	if err := os.WriteFile(sentinel, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})
	err := command.Run(context.Background(), []string{"agentenv", "delete", "../outside"})
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("delete unsafe profile error = %v, want an invalid-name error", err)
	}
	if contents, err := os.ReadFile(sentinel); err != nil || string(contents) != "safe" {
		t.Fatalf("delete touched path outside profile root (contents: %q, error: %v)", contents, err)
	}
}

func TestNewRejectsANestedProfileName(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})
	err := command.Run(context.Background(), []string{"agentenv", "new", "../outside"})
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("new unsafe profile error = %v, want an invalid-name error", err)
	}
	if _, err := os.Stat(filepath.Join(root, "outside")); !os.IsNotExist(err) {
		t.Fatalf("new created a path outside the profile root (stat error: %v)", err)
	}
}

func TestUseRejectsAProfileOutsideTheProfileRoot(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	for _, path := range []string{filepath.Join(root, "outside"), projectRoot} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})
	err := command.Run(context.Background(), []string{"agentenv", "use", "../outside"})
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("use unsafe profile error = %v, want an invalid-name error", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".agentenv")); !os.IsNotExist(err) {
		t.Fatalf("use wrote an unsafe profile selection (stat error: %v)", err)
	}
}

func TestAgentLaunchRejectsAnUnsafeProjectSelection(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("../outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	command := cliapp.New(cliapp.Options{
		ProfileRoot: filepath.Join(root, "profiles"),
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})
	err := command.Run(context.Background(), []string{"agentenv", "codex"})
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("launch with unsafe selection error = %v, want an invalid-name error", err)
	}
}

func TestCurrentPrintsTheProfileSelectedByAParentProject(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	workingDir := filepath.Join(projectRoot, "nested")
	if err := os.MkdirAll(workingDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("security-review\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{WorkingDir: workingDir, Stdout: &stdout})
	if err := command.Run(context.Background(), []string{"agentenv", "current"}); err != nil {
		t.Fatalf("show current profile: %v", err)
	}
	if got, want := stdout.String(), "security-review\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

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

func TestNewProfileDoesNotImportExistingClaudeDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTENV_HOME", "")

	existingDefaults := map[string][]byte{
		filepath.Join(".claude", "settings.json"): []byte(`{"model":"sonnet"}`),
		".claude.json": []byte(`{"hasCompletedOnboarding":true}`),
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

	for _, path := range []string{
		filepath.Join("claude", "settings.json"),
		filepath.Join("claude", ".claude.json"),
	} {
		if _, err := os.Lstat(filepath.Join(home, ".agent-profiles", "default", path)); !os.IsNotExist(err) {
			t.Fatalf("profile imported Claude default %q (lstat error: %v)", path, err)
		}
	}
	for path, want := range existingDefaults {
		got, err := os.ReadFile(filepath.Join(home, path))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("real-home Claude default %q = %q, %v; want %q", path, got, err, want)
		}
	}
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

func TestClaudeLaunchDoesNotImportDefaultsIntoAnExistingProfile(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"theme":"existing-theme"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeClaude := "#!/bin/sh\ntest ! -e \"$CLAUDE_CONFIG_DIR/.claude.json\" && test ! -e \"$CLAUDE_CONFIG_DIR/settings.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, WorkingDir: projectRoot, Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "claude"}); err != nil {
		t.Fatalf("launch Claude without imported defaults: %v", err)
	}
	for path, want := range map[string]string{
		filepath.Join(home, ".claude", "settings.json"): `{"setting":"inherited"}`,
		filepath.Join(home, ".claude.json"):             `{"theme":"existing-theme"}`,
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("real-home Claude default %q = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestNewProfilesShareFileBasedOAuthCredentials(t *testing.T) {
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

func TestListHidesTheSharedCredentialStore(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &stdout})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	stdout.Reset()

	if err := command.Run(context.Background(), []string{"agentenv", "list"}); err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if got, want := stdout.String(), "default\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestDeleteCannotRemoveTheSharedCredentialStore(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "default"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	credentials := []byte(`{"tokens":{"access_token":"keep-shared"}}`)
	if err := os.WriteFile(filepath.Join(profileRoot, "default", "codex", "auth.json"), credentials, 0o600); err != nil {
		t.Fatalf("write shared credentials: %v", err)
	}

	err := command.Run(context.Background(), []string{"agentenv", "delete", "shared"})
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("delete shared store error = %v, want an invalid-name error", err)
	}
	got, err := os.ReadFile(filepath.Join(profileRoot, "shared", "codex-auth.json"))
	if err != nil {
		t.Fatalf("read preserved shared credentials: %v", err)
	}
	if !bytes.Equal(got, credentials) {
		t.Fatalf("shared credentials = %q, want %q", got, credentials)
	}
}

func TestAgentLaunchPreservesSharingWhenCredentialsAreAtomicallyReplaced(t *testing.T) {
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

func TestSharedProfileNameIsReservedCaseInsensitively(t *testing.T) {
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	command := cliapp.New(cliapp.Options{ProfileRoot: profileRoot, Stdout: &bytes.Buffer{}})

	for _, name := range []string{"shared", "Shared", "SHARED"} {
		err := command.Run(context.Background(), []string{"agentenv", "new", name})
		if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
			t.Fatalf("new reserved profile %q error = %v, want an invalid-name error", name, err)
		}
	}
}

func TestAgentLaunchSecuresSharedOAuthFiles(t *testing.T) {
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

func TestEnableRunsToolInstallersInsideTheSelectedProfile(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeRtk := `#!/bin/sh
printf 'args=%s home=%s codex_home=%s claude_config_dir=%s\n' "$*" "$HOME" "$CODEX_HOME" "$CLAUDE_CONFIG_DIR"
`
	if err := os.WriteFile(filepath.Join(binDir, "rtk"), []byte(fakeRtk), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "superpowers"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "superpowers"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	stdout.Reset()

	if err := command.Run(context.Background(), []string{"agentenv", "enable", "rtk"}); err != nil {
		t.Fatalf("enable rtk: %v", err)
	}

	profilePath := filepath.Join(profileRoot, "superpowers")
	environment := " home=" + filepath.Join(profilePath, "home") +
		" codex_home=" + filepath.Join(profilePath, "codex") +
		" claude_config_dir=" + filepath.Join(profilePath, "claude") + "\n"
	want := "args=init -g --auto-patch" + environment +
		"args=init -g --codex" + environment
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestDisableRunsToolUninstallersInsideTheSelectedProfile(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fakeTokensave := `#!/bin/sh
printf 'args=%s home=%s\n' "$*" "$HOME"
`
	if err := os.WriteFile(filepath.Join(binDir, "tokensave"), []byte(fakeTokensave), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "superpowers"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "superpowers"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	stdout.Reset()

	if err := command.Run(context.Background(), []string{"agentenv", "disable", "tokensave"}); err != nil {
		t.Fatalf("disable tokensave: %v", err)
	}

	profileHome := filepath.Join(profileRoot, "superpowers", "home")
	want := "args=uninstall --agent claude home=" + profileHome + "\n" +
		"args=uninstall --agent codex home=" + profileHome + "\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestEnableRejectsAnUnknownTool(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})

	err := command.Run(context.Background(), []string{"agentenv", "enable", "mystery"})
	if err == nil || !strings.Contains(err.Error(), `unsupported tool "mystery"`) {
		t.Fatalf("enable mystery = %v, want unsupported tool error", err)
	}
}

func TestEnableFailsWhenAStepFails(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "rtk"), []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "superpowers"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "superpowers"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}

	err := command.Run(context.Background(), []string{"agentenv", "enable", "rtk"})
	if err == nil || !strings.Contains(err.Error(), "rtk init -g --auto-patch") {
		t.Fatalf("enable rtk = %v, want step failure error", err)
	}
}

func TestProxySetStoresTheProxyURLForTheActiveProfile(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	for _, path := range []string{hostHome, projectRoot} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", hostHome)
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "gateway"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "gateway"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	stdout.Reset()

	for _, arguments := range [][]string{
		{"agentenv", "proxy", "set", "codex", "http://localhost:4000"},
		{"agentenv", "proxy", "set", "claude", "https://gateway.example.com/anthropic"},
	} {
		if err := command.Run(context.Background(), arguments); err != nil {
			t.Fatalf("%s: %v", strings.Join(arguments, " "), err)
		}
	}
	want := "Set codex proxy http://localhost:4000 for profile gateway\n" +
		"Set claude proxy https://gateway.example.com/anthropic for profile gateway\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	contents, err := os.ReadFile(filepath.Join(profileRoot, "gateway", "config.json"))
	if err != nil {
		t.Fatalf("read profile configuration: %v", err)
	}
	var config struct {
		Proxy map[string]string `json:"proxy"`
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		t.Fatalf("parse profile configuration %q: %v", contents, err)
	}
	if config.Proxy["codex"] != "http://localhost:4000" || config.Proxy["claude"] != "https://gateway.example.com/anthropic" {
		t.Fatalf("stored proxies = %+v, want both agents configured", config.Proxy)
	}

	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "proxy", "show"}); err != nil {
		t.Fatalf("show proxies: %v", err)
	}
	want = "codex http://localhost:4000\nclaude https://gateway.example.com/anthropic\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAgentLaunchExportsTheConfiguredProxyURL(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nprintf 'proxy=%s\\n' \"$OPENAI_BASE_URL\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nprintf 'proxy=%s\\n' \"$ANTHROPIC_BASE_URL\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENAI_BASE_URL", "http://wrong-codex-proxy")
	t.Setenv("ANTHROPIC_BASE_URL", "http://wrong-claude-proxy")
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "gateway"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "gateway"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	for _, arguments := range [][]string{
		{"agentenv", "proxy", "set", "codex", "http://localhost:4000/openai"},
		{"agentenv", "proxy", "set", "claude", "http://localhost:4000/anthropic"},
	} {
		if err := command.Run(context.Background(), arguments); err != nil {
			t.Fatalf("%s: %v", strings.Join(arguments, " "), err)
		}
	}

	for agent, want := range map[string]string{
		"codex":  "proxy=http://localhost:4000/openai\n",
		"claude": "proxy=http://localhost:4000/anthropic\n",
	} {
		stdout.Reset()
		if err := command.Run(context.Background(), []string{"agentenv", agent}); err != nil {
			t.Fatalf("launch %s: %v", agent, err)
		}
		if got := stdout.String(); got != want {
			t.Fatalf("%s stdout = %q, want %q", agent, got, want)
		}
	}
}

func TestProxyUnsetRestoresTheInheritedEndpoint(t *testing.T) {
	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nprintf 'proxy=%s\\n' \"$OPENAI_BASE_URL\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENAI_BASE_URL", "http://inherited-proxy")
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "new", "gateway"}); err != nil {
		t.Fatalf("new profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "use", "gateway"}); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	if err := command.Run(context.Background(), []string{"agentenv", "proxy", "set", "codex", "http://localhost:4000"}); err != nil {
		t.Fatalf("set codex proxy: %v", err)
	}
	stdout.Reset()

	if err := command.Run(context.Background(), []string{"agentenv", "proxy", "unset", "codex"}); err != nil {
		t.Fatalf("unset codex proxy: %v", err)
	}
	if got, want := stdout.String(), "Removed codex proxy for profile gateway\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := command.Run(context.Background(), []string{"agentenv", "codex"}); err != nil {
		t.Fatalf("launch codex: %v", err)
	}
	if got, want := stdout.String(), "proxy=http://inherited-proxy\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestProxySetRejectsInvalidInput(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("gateway\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  projectRoot,
		Stdout:      &bytes.Buffer{},
	})

	err := command.Run(context.Background(), []string{"agentenv", "proxy", "set", "gemini", "http://localhost:4000"})
	if err == nil || !strings.Contains(err.Error(), `unsupported agent "gemini"`) {
		t.Fatalf("set gemini proxy = %v, want unsupported agent error", err)
	}
	err = command.Run(context.Background(), []string{"agentenv", "proxy", "set", "codex", "localhost:4000"})
	if err == nil || !strings.Contains(err.Error(), "invalid proxy URL") {
		t.Fatalf("set schemeless proxy = %v, want invalid URL error", err)
	}
	if _, err := os.Stat(filepath.Join(profileRoot, "gateway", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("rejected input still wrote configuration (stat error: %v)", err)
	}
}
