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
