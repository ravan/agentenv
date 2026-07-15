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

	if got, want := stdout.String(), "✓ Created profile superpowers\n"; got != want {
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
	if got, want := stdout.String(), "✓ Deleted profile old\n"; got != want {
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
	if got, want := stdout.String(), "✓ Using profile superpowers\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCurrentPrintsTheProfileSelectedByAParentProject(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	workingDir := filepath.Join(projectRoot, "nested")
	if err := os.MkdirAll(workingDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("security-review\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer

	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		WorkingDir:  workingDir,
		Stdout:      &stdout,
	})
	if err := command.Run(context.Background(), []string{"agentenv", "current"}); err != nil {
		t.Fatalf("show current profile: %v", err)
	}
	want := " security-review\n" +
		"   rtk           ○ disabled\n" +
		"   tokensave     ○ disabled\n" +
		"   codex proxy   (not set)\n" +
		"   claude proxy  (not set)\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCurrentSummarizesIntegrationsAndProxies(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(binDir, "rtk"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
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
	for _, arguments := range [][]string{
		{"agentenv", "new", "superpowers"},
		{"agentenv", "use", "superpowers"},
		{"agentenv", "enable", "rtk"},
		{"agentenv", "proxy", "set", "claude", "http://localhost:4000/anthropic"},
	} {
		if err := command.Run(context.Background(), arguments); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
	}
	stdout.Reset()

	if err := command.Run(context.Background(), []string{"agentenv", "current"}); err != nil {
		t.Fatalf("show current profile: %v", err)
	}
	want := " superpowers\n" +
		"   rtk           ● enabled\n" +
		"   tokensave     ○ disabled\n" +
		"   codex proxy   (not set)\n" +
		"   claude proxy  http://localhost:4000/anthropic\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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
