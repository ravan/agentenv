package cliapp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ravan/agentenv/internal/cliapp"
)

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
	want := "✓ Set codex proxy http://localhost:4000 for profile gateway\n" +
		"✓ Set claude proxy https://gateway.example.com/anthropic for profile gateway\n"
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
	if got, want := stdout.String(), "✓ Removed codex proxy for profile gateway\n"; got != want {
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
