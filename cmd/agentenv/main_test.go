package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunExecutesTheAgentenvCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	t.Setenv("AGENTENV_HOME", profileRoot)

	if err := run(context.Background(), []string{"agentenv", "new", "from-entrypoint"}); err != nil {
		t.Fatalf("run agentenv: %v", err)
	}

	for _, tool := range []string{"codex", "claude"} {
		if info, err := os.Stat(filepath.Join(profileRoot, "from-entrypoint", tool)); err != nil || !info.IsDir() {
			t.Fatalf("%s profile directory was not created (info: %v, error: %v)", tool, info, err)
		}
	}
}

func TestCompiledCommandIsolatesCodexPluginsAcrossProjectProfiles(t *testing.T) {
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed")
	}

	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	binDir := filepath.Join(root, "bin")
	marketplaceRoot := filepath.Join(root, "marketplace")
	firstProject := filepath.Join(root, "first-project")
	secondProject := filepath.Join(root, "second-project")
	for _, path := range []string{
		hostHome,
		binDir,
		firstProject,
		secondProject,
		filepath.Join(marketplaceRoot, ".agents", "plugins"),
		filepath.Join(marketplaceRoot, "plugins", "private", ".codex-plugin"),
		filepath.Join(marketplaceRoot, "plugins", "private", "skills", "private"),
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	marketplace := `{
	"name": "agentenv-test",
	"plugins": [{
		"name": "private",
		"source": {"source": "local", "path": "./plugins/private"},
		"policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"}
	}]
}`
	pluginManifest := `{
	"name": "private",
	"version": "1.0.0",
	"description": "Agentenv profile-isolation fixture",
	"skills": "./skills/"
}`
	skill := `---
name: private
description: Proves that Codex plugin state stays in one agentenv profile.
---

# Private fixture
`
	for path, contents := range map[string]string{
		filepath.Join(marketplaceRoot, ".agents", "plugins", "marketplace.json"):              marketplace,
		filepath.Join(marketplaceRoot, "plugins", "private", ".codex-plugin", "plugin.json"):  pluginManifest,
		filepath.Join(marketplaceRoot, "plugins", "private", "skills", "private", "SKILL.md"): skill,
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	agentenvBinary := filepath.Join(binDir, "agentenv")
	build := exec.Command("go", "build", "-o", agentenvBinary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agentenv: %v\n%s", err, output)
	}

	t.Setenv("AGENTENV_HOME", profileRoot)
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", filepath.Dir(codexPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	environment := os.Environ()
	run := func(directory string, arguments ...string) string {
		t.Helper()
		command := exec.Command(agentenvBinary, arguments...)
		command.Dir = directory
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("agentenv %s in %s: %v\n%s", strings.Join(arguments, " "), directory, err, output)
		}
		return string(output)
	}

	run(firstProject, "new", "first")
	run(firstProject, "new", "second")
	run(firstProject, "use", "first")
	run(secondProject, "use", "second")
	run(firstProject, "run", "codex", "--", "codex", "plugin", "marketplace", "add", marketplaceRoot, "--json")
	run(firstProject, "run", "codex", "--", "codex", "plugin", "add", "private@agentenv-test", "--json")

	type plugin struct {
		Name      string `json:"name"`
		Installed bool   `json:"installed"`
		Enabled   bool   `json:"enabled"`
	}
	type pluginList struct {
		Installed []plugin `json:"installed"`
	}
	list := func(directory string) pluginList {
		t.Helper()
		output := run(directory, "codex", "plugin", "list", "--available", "--json")
		var plugins pluginList
		if err := json.Unmarshal([]byte(output), &plugins); err != nil {
			t.Fatalf("decode Codex plugin list %q: %v", output, err)
		}
		return plugins
	}
	hasPrivatePlugin := func(plugins pluginList) bool {
		for _, candidate := range plugins.Installed {
			if candidate.Name == "private" && candidate.Installed && candidate.Enabled {
				return true
			}
		}
		return false
	}
	if !hasPrivatePlugin(list(firstProject)) {
		t.Fatalf("installing profile does not report the private plugin as installed and enabled")
	}
	if hasPrivatePlugin(list(secondProject)) {
		t.Fatalf("private plugin leaked into the second profile")
	}
	if _, err := os.Lstat(filepath.Join(hostHome, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("real Codex wrote plugin state to host home (lstat error: %v)", err)
	}
}

func TestCompiledCommandIsolatesClaudePluginsAcrossProjectProfiles(t *testing.T) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("Claude CLI is not installed")
	}

	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	binDir := filepath.Join(root, "bin")
	marketplaceRoot := filepath.Join(root, "marketplace")
	firstProject := filepath.Join(root, "first-project")
	secondProject := filepath.Join(root, "second-project")
	for _, path := range []string{
		hostHome,
		binDir,
		firstProject,
		secondProject,
		filepath.Join(marketplaceRoot, ".claude-plugin"),
		filepath.Join(marketplaceRoot, "plugins", "private", ".claude-plugin"),
	} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	marketplace := `{
	"name": "agentenv-test",
	"owner": {"name": "agentenv"},
	"plugins": [{
		"name": "private",
		"description": "Agentenv profile-isolation fixture",
		"source": "./plugins/private"
	}]
}`
	pluginManifest := `{
	"name": "private",
	"description": "Agentenv profile-isolation fixture",
	"version": "1.0.0"
}`
	for path, contents := range map[string]string{
		filepath.Join(marketplaceRoot, ".claude-plugin", "marketplace.json"):                  marketplace,
		filepath.Join(marketplaceRoot, "plugins", "private", ".claude-plugin", "plugin.json"): pluginManifest,
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	agentenvBinary := filepath.Join(binDir, "agentenv")
	build := exec.Command("go", "build", "-o", agentenvBinary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agentenv: %v\n%s", err, output)
	}

	t.Setenv("AGENTENV_HOME", profileRoot)
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", filepath.Dir(claudePath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	environment := os.Environ()
	run := func(directory string, arguments ...string) string {
		t.Helper()
		command := exec.Command(agentenvBinary, arguments...)
		command.Dir = directory
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("agentenv %s in %s: %v\n%s", strings.Join(arguments, " "), directory, err, output)
		}
		return string(output)
	}

	run(firstProject, "new", "first")
	run(firstProject, "new", "second")
	run(firstProject, "use", "first")
	run(secondProject, "use", "second")
	run(firstProject, "claude", "plugin", "marketplace", "add", marketplaceRoot)
	run(firstProject, "claude", "plugin", "install", "private@agentenv-test")

	type plugin struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	list := func(directory string) []plugin {
		t.Helper()
		output := run(directory, "claude", "plugin", "list", "--json")
		var plugins []plugin
		if err := json.Unmarshal([]byte(output), &plugins); err != nil {
			t.Fatalf("decode Claude plugin list %q: %v", output, err)
		}
		return plugins
	}
	hasPrivatePlugin := func(plugins []plugin) bool {
		for _, candidate := range plugins {
			if candidate.ID == "private@agentenv-test" && candidate.Enabled {
				return true
			}
		}
		return false
	}
	if !hasPrivatePlugin(list(firstProject)) {
		t.Fatal("installing profile does not report the private Claude plugin as installed and enabled")
	}
	if hasPrivatePlugin(list(secondProject)) {
		t.Fatal("private Claude plugin leaked into the second profile")
	}
	if _, err := os.Lstat(filepath.Join(hostHome, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("real Claude wrote plugin state to host home (lstat error: %v)", err)
	}
}

func TestCompiledCommandSharesCodexAccountPluginStateWithOAuth(t *testing.T) {
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed")
	}

	remoteInstalledResponse := func(scope string) string {
		if scope != "GLOBAL" {
			return `{"plugins":[],"pagination":{"limit":50,"next_page_token":null}}`
		}
		return `{
	"plugins": [{
		"id": "plugins~Plugin_superpowers",
		"name": "superpowers",
		"scope": "GLOBAL",
		"installation_policy": "AVAILABLE",
		"authentication_policy": "ON_INSTALL",
		"release": {
			"version": "1.0.0",
			"display_name": "Superpowers",
			"description": "Account-scoped regression fixture",
			"bundle_download_url": "",
			"app_ids": [],
			"interface": {},
			"skills": []
		},
		"enabled": true,
		"disabled_skill_names": []
	}],
	"pagination": {"limit": 50, "next_page_token": null}
}`
	}
	backend := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/backend-api/ps/plugins/installed" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(remoteInstalledResponse(request.URL.Query().Get("scope"))))
	}))
	defer backend.Close()

	root := t.TempDir()
	hostHome := filepath.Join(root, "host-home")
	profileRoot := filepath.Join(root, "profiles")
	project := filepath.Join(root, "project")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, project, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	agentenvBinary := filepath.Join(binDir, "agentenv")
	build := exec.Command("go", "build", "-o", agentenvBinary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agentenv: %v\n%s", err, output)
	}

	t.Setenv("AGENTENV_HOME", profileRoot)
	t.Setenv("HOME", hostHome)
	t.Setenv("PATH", filepath.Dir(codexPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	environment := os.Environ()
	run := func(arguments ...string) {
		t.Helper()
		command := exec.Command(agentenvBinary, arguments...)
		command.Dir = project
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("agentenv %s: %v\n%s", strings.Join(arguments, " "), err, output)
		}
	}
	run("new", "clean")
	run("use", "clean")

	auth := fakeChatGPTAuth(t)
	sharedAuth := filepath.Join(profileRoot, "shared", "codex-auth.json")
	if err := os.WriteFile(sharedAuth, auth, 0o600); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("chatgpt_base_url = %q\n", backend.URL+"/backend-api")
	if err := os.WriteFile(filepath.Join(profileRoot, "clean", "codex", "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, agentenvBinary, "codex", "app-server")
	command.Dir = project
	command.Env = environment
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = command.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	writeAppServerMessage(t, stdin, map[string]any{
		"method": "initialize",
		"id":     0,
		"params": map[string]any{
			"clientInfo":   map[string]any{"name": "agentenv_test", "title": "agentenv test", "version": "0.1.0"},
			"capabilities": map[string]any{"experimentalApi": true},
		},
	})
	readAppServerResponse(t, scanner, 0, &stderr)
	writeAppServerMessage(t, stdin, map[string]any{"method": "initialized", "params": map[string]any{}})
	writeAppServerMessage(t, stdin, map[string]any{
		"method": "plugin/installed",
		"id":     1,
		"params": map[string]any{"cwds": []string{project}},
	})
	response := readAppServerResponse(t, scanner, 1, &stderr)
	if !appServerResponseHasPlugin(response, "superpowers") {
		t.Fatalf("fresh profile did not receive the account-scoped plugin associated with shared OAuth: %s", mustJSON(t, response))
	}
	if _, err := os.Lstat(filepath.Join(hostHome, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("Codex account-plugin check wrote state to host home (lstat error: %v)", err)
	}
}

func fakeChatGPTAuth(t *testing.T) []byte {
	t.Helper()
	encode := func(value any) string {
		contents, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(contents)
	}
	idToken := strings.Join([]string{
		encode(map[string]any{"alg": "none", "typ": "JWT"}),
		encode(map[string]any{
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": "account-123",
				"chatgpt_user_id":    "user-123",
			},
		}),
		base64.RawURLEncoding.EncodeToString([]byte("signature")),
	}, ".")
	auth, err := json.Marshal(map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"id_token":      idToken,
			"access_token":  "chatgpt-token",
			"refresh_token": "refresh-token",
			"account_id":    "account-123",
		},
		"last_refresh": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func writeAppServerMessage(t *testing.T, writer io.Writer, message any) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(message); err != nil {
		t.Fatal(err)
	}
}

func readAppServerResponse(t *testing.T, scanner *bufio.Scanner, id float64, stderr *bytes.Buffer) map[string]any {
	t.Helper()
	for scanner.Scan() {
		var message map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			t.Fatalf("decode app-server message %q: %v", scanner.Text(), err)
		}
		if message["id"] == id {
			return message
		}
	}
	t.Fatalf("app-server ended before response %v: %v\n%s", id, scanner.Err(), stderr.String())
	return nil
}

func appServerResponseHasPlugin(response map[string]any, name string) bool {
	result, _ := response["result"].(map[string]any)
	marketplaces, _ := result["marketplaces"].([]any)
	for _, marketplaceValue := range marketplaces {
		marketplace, _ := marketplaceValue.(map[string]any)
		plugins, _ := marketplace["plugins"].([]any)
		for _, pluginValue := range plugins {
			plugin, _ := pluginValue.(map[string]any)
			if plugin["name"] == name && plugin["installed"] == true {
				return true
			}
		}
	}
	return false
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
