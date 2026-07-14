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
