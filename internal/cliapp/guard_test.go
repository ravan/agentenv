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

func TestClaudeGuardStopsAnUnprofiledLaunchInASelectedProject(t *testing.T) {
	root := t.TempDir()
	profileRoot := filepath.Join(root, "profiles")
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(profileRoot, "default", "claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".agentenv"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "global-claude-home"))
	input := `{"cwd":` + strconv.Quote(projectRoot) + `,"hook_event_name":"SessionStart","source":"startup"}`
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: profileRoot,
		Stdin:       strings.NewReader(input),
		Stdout:      &stdout,
	})

	if err := command.Run(context.Background(), []string{"agentenv", "guard", "claude"}); err != nil {
		t.Fatalf("guard Claude launch: %v", err)
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
		t.Fatalf("guard allowed an unprofiled Claude launch: %+v", result)
	}
	wantHome := filepath.Join(profileRoot, "default", "claude")
	if !strings.Contains(result.StopReason, wantHome) ||
		!strings.Contains(result.StopReason, "CLAUDE_CONFIG_DIR") ||
		!strings.Contains(result.SystemMessage, "agentenv claude") {
		t.Fatalf("guard result = %+v, want expected home %q and relaunch guidance", result, wantHome)
	}
}

func TestClaudeGuardAllowsGlobalLaunchesOutsideSelectedProjects(t *testing.T) {
	projectRoot := t.TempDir()
	input := `{"cwd":` + strconv.Quote(projectRoot) + `,"hook_event_name":"SessionStart","source":"startup"}`
	var stdout bytes.Buffer
	command := cliapp.New(cliapp.Options{
		ProfileRoot: filepath.Join(t.TempDir(), "profiles"),
		Stdin:       strings.NewReader(input),
		Stdout:      &stdout,
	})

	if err := command.Run(context.Background(), []string{"agentenv", "guard", "claude"}); err != nil {
		t.Fatalf("guard global Claude launch: %v", err)
	}
	var result struct {
		Continue bool `json:"continue"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode guard output %q: %v", stdout.String(), err)
	}
	if !result.Continue {
		t.Fatalf("guard stopped Claude outside an agentenv project")
	}
}
