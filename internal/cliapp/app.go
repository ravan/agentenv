package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

// Options supplies the operating-system boundaries used by the CLI.
type Options struct {
	ProfileRoot string
	WorkingDir  string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

type credentialLink struct {
	tool        string
	profileFile string
	sharedFile  string
}

var sharedCredentialLinks = []credentialLink{
	{tool: "codex", profileFile: "auth.json", sharedFile: "codex-auth.json"},
	{tool: "claude", profileFile: ".credentials.json", sharedFile: "claude-credentials.json"},
}

// proxyEnvironmentVariables maps each agent to the environment variable its
// client reads for an alternative API endpoint.
var proxyEnvironmentVariables = map[string]string{
	"codex":  "OPENAI_BASE_URL",
	"claude": "ANTHROPIC_BASE_URL",
}

// profileConfig holds per-profile settings stored in <profile>/config.json.
type profileConfig struct {
	Proxy map[string]string `json:"proxy,omitempty"`
}

// New constructs the agentenv command.
func New(options Options) *cli.Command {
	options = withDefaults(options)

	return &cli.Command{
		Name:  "agentenv",
		Usage: "manage isolated profiles for AI coding agents",
		Commands: []*cli.Command{
			{
				Name:      "new",
				Usage:     "create a profile",
				ArgsUsage: "<name>",
				Action: func(_ context.Context, command *cli.Command) error {
					name := command.Args().First()
					if name == "" {
						return cli.Exit("profile name is required", 2)
					}
					if !validProfileName(name) {
						return fmt.Errorf("invalid profile name %q", name)
					}

					if err := os.MkdirAll(options.ProfileRoot, 0o750); err != nil {
						return fmt.Errorf("create profile root: %w", err)
					}
					profilePath := filepath.Join(options.ProfileRoot, name)
					if err := os.Mkdir(profilePath, 0o750); err != nil {
						if os.IsExist(err) {
							return fmt.Errorf("profile %q already exists", name)
						}
						return fmt.Errorf("create profile %q: %w", name, err)
					}
					for _, tool := range []string{"codex", "claude"} {
						if err := os.Mkdir(filepath.Join(profilePath, tool), 0o750); err != nil {
							return fmt.Errorf("create profile %q: %w", name, err)
						}
					}
					if err := prepareProfileHome(profilePath); err != nil {
						return fmt.Errorf("create profile %q home: %w", name, err)
					}
					if err := os.MkdirAll(filepath.Join(options.ProfileRoot, "shared"), 0o700); err != nil {
						return fmt.Errorf("create shared credential store: %w", err)
					}
					if err := adoptExistingCredentials(options.ProfileRoot); err != nil {
						return err
					}
					for _, credential := range sharedCredentialLinks {
						target := filepath.Join("..", "..", "shared", credential.sharedFile)
						link := filepath.Join(profilePath, credential.tool, credential.profileFile)
						if err := os.Symlink(target, link); err != nil {
							return fmt.Errorf("share %s credentials for profile %q: %w", credential.tool, name, err)
						}
					}

					_, err := fmt.Fprintf(options.Stdout, "Created profile %s\n", name)
					return err
				},
			},
			{
				Name:  "list",
				Usage: "list profiles",
				Action: func(_ context.Context, _ *cli.Command) error {
					entries, err := os.ReadDir(options.ProfileRoot)
					if os.IsNotExist(err) {
						return nil
					}
					if err != nil {
						return fmt.Errorf("list profiles: %w", err)
					}

					for _, entry := range entries {
						if !entry.IsDir() || entry.Name() == "shared" {
							continue
						}
						if _, err := fmt.Fprintln(options.Stdout, entry.Name()); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				Name:      "delete",
				Usage:     "delete a profile",
				ArgsUsage: "<name>",
				Action: func(_ context.Context, command *cli.Command) error {
					name := command.Args().First()
					if name == "" {
						return cli.Exit("profile name is required", 2)
					}
					if !validProfileName(name) {
						return fmt.Errorf("invalid profile name %q", name)
					}

					profilePath := filepath.Join(options.ProfileRoot, name)
					if _, err := os.Stat(profilePath); err != nil {
						if os.IsNotExist(err) {
							return fmt.Errorf("profile %q does not exist", name)
						}
						return fmt.Errorf("inspect profile %q: %w", name, err)
					}
					if err := os.RemoveAll(profilePath); err != nil {
						return fmt.Errorf("delete profile %q: %w", name, err)
					}

					_, err := fmt.Fprintf(options.Stdout, "Deleted profile %s\n", name)
					return err
				},
			},
			{
				Name:      "use",
				Aliases:   []string{"activate"},
				Usage:     "select a profile for the current project",
				ArgsUsage: "<name>",
				Action: func(_ context.Context, command *cli.Command) error {
					name := command.Args().First()
					if name == "" {
						return cli.Exit("profile name is required", 2)
					}
					if !validProfileName(name) {
						return fmt.Errorf("invalid profile name %q", name)
					}

					info, err := os.Stat(filepath.Join(options.ProfileRoot, name))
					if err != nil {
						if os.IsNotExist(err) {
							return fmt.Errorf("profile %q does not exist", name)
						}
						return fmt.Errorf("inspect profile %q: %w", name, err)
					}
					if !info.IsDir() {
						return fmt.Errorf("profile %q is not a directory", name)
					}

					selectionPath := filepath.Join(options.WorkingDir, ".agentenv")
					if err := os.WriteFile(selectionPath, []byte(name+"\n"), 0o600); err != nil {
						return fmt.Errorf("activate profile %q: %w", name, err)
					}
					_, err = fmt.Fprintf(options.Stdout, "Using profile %s\n", name)
					return err
				},
			},
			{
				Name:  "current",
				Usage: "print the active profile",
				Action: func(_ context.Context, _ *cli.Command) error {
					profile, err := findActiveProfile(options.WorkingDir)
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(options.Stdout, profile)
					return err
				},
			},
			guardCommand(options),
			runCommand(options),
			integrationCommand("enable", options),
			integrationCommand("disable", options),
			proxyCommand(options),
			codexPluginCommand(options),
			agentCommand("codex", "CODEX_HOME", options),
			agentCommand("claude", "CLAUDE_CONFIG_DIR", options),
		},
	}
}

func guardCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "guard",
		Usage:     "validate a profiled agent startup hook",
		ArgsUsage: "<codex>",
		Hidden:    true,
		Action: func(_ context.Context, command *cli.Command) error {
			name := command.Args().First()
			if name != "codex" {
				return fmt.Errorf("unsupported agent %q", name)
			}
			var input struct {
				WorkingDir string `json:"cwd"`
			}
			if err := json.NewDecoder(options.Stdin).Decode(&input); err != nil {
				return fmt.Errorf("read Codex SessionStart hook input: %w", err)
			}
			profile, found, err := findActiveProfileOptional(input.WorkingDir)
			if err != nil {
				return err
			}
			if !found {
				return json.NewEncoder(options.Stdout).Encode(struct {
					Continue bool `json:"continue"`
				}{Continue: true})
			}
			expectedHome := filepath.Join(options.ProfileRoot, profile, "codex")
			actualHome := os.Getenv("CODEX_HOME")
			expectedPrivateHome := filepath.Join(options.ProfileRoot, profile, "home")
			actualPrivateHome := os.Getenv("HOME")
			result := struct {
				Continue      bool   `json:"continue"`
				StopReason    string `json:"stopReason,omitempty"`
				SystemMessage string `json:"systemMessage,omitempty"`
			}{Continue: actualHome == expectedHome && actualPrivateHome == expectedPrivateHome}
			if !result.Continue {
				result.StopReason = fmt.Sprintf("project selects profile %q and requires CODEX_HOME=%s and HOME=%s; got CODEX_HOME=%q and HOME=%q", profile, expectedHome, expectedPrivateHome, actualHome, actualPrivateHome)
				result.SystemMessage = fmt.Sprintf("Codex was launched outside agentenv profile %q. Exit and relaunch with `agentenv codex`.", profile)
			}
			if err := json.NewEncoder(options.Stdout).Encode(result); err != nil {
				return fmt.Errorf("write Codex SessionStart hook result: %w", err)
			}
			return nil
		},
	}
}

func adoptExistingCredentials(profileRoot string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate existing agent credentials: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(profileRoot, "shared"), 0o700); err != nil {
		return fmt.Errorf("create shared credential store: %w", err)
	}

	for _, credential := range sharedCredentialLinks {
		sharedPath := filepath.Join(profileRoot, "shared", credential.sharedFile)
		if _, err := os.Lstat(sharedPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect shared %s credentials: %w", credential.tool, err)
		}

		existingPath := filepath.Join(home, "."+credential.tool, credential.profileFile)
		contents, err := os.ReadFile(existingPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read existing %s credentials: %w", credential.tool, err)
		}
		if err := os.WriteFile(sharedPath, contents, 0o600); err != nil {
			return fmt.Errorf("adopt existing %s credentials: %w", credential.tool, err)
		}
	}
	return nil
}

func validProfileName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.EqualFold(name, "shared") &&
		strings.TrimSpace(name) == name && filepath.Base(name) == name
}

func withDefaults(options Options) Options {
	if options.ProfileRoot == "" {
		options.ProfileRoot = os.Getenv("AGENTENV_HOME")
		if options.ProfileRoot == "" {
			home, _ := os.UserHomeDir()
			options.ProfileRoot = filepath.Join(home, ".agent-profiles")
		}
	}
	if options.WorkingDir == "" {
		options.WorkingDir, _ = os.Getwd()
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	return options
}

func agentCommand(name, homeVariable string, options Options) *cli.Command {
	return &cli.Command{
		Name:            name,
		Usage:           "launch " + name + " with the active profile",
		ArgsUsage:       "[arguments...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, command *cli.Command) error {
			return runProfileProcess(ctx, name, homeVariable, name, command.Args().Slice(), options)
		},
	}
}

func runCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:            "run",
		Usage:           "run a wrapper command inside the active agent profile",
		ArgsUsage:       "<codex|claude> -- <command> [arguments...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, command *cli.Command) error {
			arguments := command.Args().Slice()
			if len(arguments) == 0 {
				return cli.Exit("agent name is required", 2)
			}
			name := arguments[0]
			homeVariable := ""
			switch name {
			case "codex":
				homeVariable = "CODEX_HOME"
			case "claude":
				homeVariable = "CLAUDE_CONFIG_DIR"
			default:
				return fmt.Errorf("unsupported agent %q", name)
			}
			arguments = arguments[1:]
			if len(arguments) > 0 && arguments[0] == "--" {
				arguments = arguments[1:]
			}
			if len(arguments) == 0 {
				return cli.Exit("wrapper command is required", 2)
			}
			return runProfileProcess(ctx, name, homeVariable, arguments[0], arguments[1:], options)
		},
	}
}

// integrationSteps maps a helper tool and action to the tool's own installer
// invocations, one per agent home. The installers resolve every path through
// HOME (or the agent home variables), so running them inside the profile
// environment writes to the profile instead of the real global config.
var integrationSteps = map[string]map[string][][]string{
	"rtk": {
		"enable": {
			{"rtk", "init", "-g", "--auto-patch"},
			{"rtk", "init", "-g", "--codex"},
		},
		"disable": {
			{"rtk", "init", "-g", "--uninstall"},
			{"rtk", "init", "-g", "--codex", "--uninstall"},
		},
	},
	"tokensave": {
		"enable": {
			{"tokensave", "install", "--agent", "claude", "--git-hook", "no"},
			{"tokensave", "install", "--agent", "codex", "--git-hook", "no"},
		},
		"disable": {
			{"tokensave", "uninstall", "--agent", "claude"},
			{"tokensave", "uninstall", "--agent", "codex"},
		},
	},
}

func integrationCommand(action string, options Options) *cli.Command {
	return &cli.Command{
		Name:      action,
		Usage:     action + " a helper tool integration in the active profile",
		ArgsUsage: "<rtk|tokensave>",
		Action: func(ctx context.Context, command *cli.Command) error {
			tool := command.Args().First()
			if tool == "" {
				return cli.Exit("tool name is required", 2)
			}
			steps, ok := integrationSteps[tool][action]
			if !ok {
				return fmt.Errorf("unsupported tool %q", tool)
			}
			return runIntegrationSteps(ctx, steps, options)
		},
	}
}

func runIntegrationSteps(ctx context.Context, steps [][]string, options Options) error {
	profile, err := findActiveProfile(options.WorkingDir)
	if err != nil {
		return err
	}
	profilePath := filepath.Join(options.ProfileRoot, profile)
	for _, agent := range []string{"codex", "claude"} {
		if info, err := os.Stat(filepath.Join(profilePath, agent)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("profile %q does not exist; create it with 'agentenv new %s'", profile, profile)
			}
			return fmt.Errorf("inspect profile %q: %w", profile, err)
		} else if !info.IsDir() {
			return fmt.Errorf("%s home for profile %q is not a directory", agent, profile)
		}
	}
	if err := prepareProfileHome(profilePath); err != nil {
		return fmt.Errorf("prepare home for profile %q: %w", profile, err)
	}
	environment := replaceEnvironment(os.Environ(), "CODEX_HOME", filepath.Join(profilePath, "codex"))
	environment = replaceEnvironment(environment, "CLAUDE_CONFIG_DIR", filepath.Join(profilePath, "claude"))
	environment = replaceEnvironment(environment, "AGENTENV_HOME", options.ProfileRoot)
	environment = privateHomeEnvironment(environment, filepath.Join(profilePath, "home"))
	for _, step := range steps {
		process := exec.CommandContext(ctx, step[0], step[1:]...)
		process.Dir = options.WorkingDir
		process.Env = environment
		process.Stdin = options.Stdin
		process.Stdout = options.Stdout
		process.Stderr = options.Stderr
		if err := process.Run(); err != nil {
			return fmt.Errorf("run %s: %w", strings.Join(step, " "), err)
		}
	}
	return nil
}

func proxyCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "proxy",
		Usage: "manage agent proxy URLs in the active profile",
		Commands: []*cli.Command{
			{
				Name:      "set",
				Usage:     "route an agent's API traffic through a proxy URL",
				ArgsUsage: "<codex|claude> <url>",
				Action: func(_ context.Context, command *cli.Command) error {
					tool := command.Args().Get(0)
					proxyURL := command.Args().Get(1)
					if tool == "" || proxyURL == "" {
						return cli.Exit("agent name and proxy URL are required", 2)
					}
					if _, ok := proxyEnvironmentVariables[tool]; !ok {
						return fmt.Errorf("unsupported agent %q", tool)
					}
					if err := validateProxyURL(proxyURL); err != nil {
						return err
					}
					profile, profilePath, err := activeProfilePath(options)
					if err != nil {
						return err
					}
					config, err := loadProfileConfig(profilePath)
					if err != nil {
						return err
					}
					if config.Proxy == nil {
						config.Proxy = map[string]string{}
					}
					config.Proxy[tool] = proxyURL
					if err := saveProfileConfig(profilePath, config); err != nil {
						return err
					}
					_, err = fmt.Fprintf(options.Stdout, "Set %s proxy %s for profile %s\n", tool, proxyURL, profile)
					return err
				},
			},
			{
				Name:      "unset",
				Usage:     "remove an agent's proxy URL",
				ArgsUsage: "<codex|claude>",
				Action: func(_ context.Context, command *cli.Command) error {
					tool := command.Args().First()
					if tool == "" {
						return cli.Exit("agent name is required", 2)
					}
					if _, ok := proxyEnvironmentVariables[tool]; !ok {
						return fmt.Errorf("unsupported agent %q", tool)
					}
					profile, profilePath, err := activeProfilePath(options)
					if err != nil {
						return err
					}
					config, err := loadProfileConfig(profilePath)
					if err != nil {
						return err
					}
					delete(config.Proxy, tool)
					if err := saveProfileConfig(profilePath, config); err != nil {
						return err
					}
					_, err = fmt.Fprintf(options.Stdout, "Removed %s proxy for profile %s\n", tool, profile)
					return err
				},
			},
			{
				Name:  "show",
				Usage: "print the proxy URLs configured for the active profile",
				Action: func(_ context.Context, _ *cli.Command) error {
					_, profilePath, err := activeProfilePath(options)
					if err != nil {
						return err
					}
					config, err := loadProfileConfig(profilePath)
					if err != nil {
						return err
					}
					for _, tool := range []string{"codex", "claude"} {
						if proxyURL := config.Proxy[tool]; proxyURL != "" {
							if _, err := fmt.Fprintf(options.Stdout, "%s %s\n", tool, proxyURL); err != nil {
								return err
							}
						}
					}
					return nil
				},
			},
		},
	}
}

func validateProxyURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid proxy URL %q: %w", raw, err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("invalid proxy URL %q: an absolute http or https URL is required", raw)
	}
	return nil
}

func activeProfilePath(options Options) (string, string, error) {
	profile, err := findActiveProfile(options.WorkingDir)
	if err != nil {
		return "", "", err
	}
	profilePath := filepath.Join(options.ProfileRoot, profile)
	if info, err := os.Stat(profilePath); err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("profile %q does not exist; create it with 'agentenv new %s'", profile, profile)
		}
		return "", "", fmt.Errorf("inspect profile %q: %w", profile, err)
	} else if !info.IsDir() {
		return "", "", fmt.Errorf("profile %q is not a directory", profile)
	}
	return profile, profilePath, nil
}

func loadProfileConfig(profilePath string) (profileConfig, error) {
	var config profileConfig
	contents, err := os.ReadFile(filepath.Join(profilePath, "config.json"))
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("read profile configuration: %w", err)
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		return config, fmt.Errorf("parse profile configuration: %w", err)
	}
	return config, nil
}

func saveProfileConfig(profilePath string, config profileConfig) error {
	contents, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(profilePath, "config.json"), append(contents, '\n'), 0o600); err != nil {
		return fmt.Errorf("write profile configuration: %w", err)
	}
	return nil
}

func codexPluginCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:            "codex-plugin",
		Usage:           "manage profile-local Codex plugins in the active profile",
		ArgsUsage:       "<add|list|remove|marketplace> [arguments...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, command *cli.Command) error {
			arguments := command.Args().Slice()
			if len(arguments) == 0 {
				return cli.Exit("plugin command is required", 2)
			}
			return runProfileProcess(ctx, "codex", "CODEX_HOME", "codex", append([]string{"plugin"}, arguments...), options)
		},
	}
}

func runProfileProcess(ctx context.Context, name, homeVariable, executable string, arguments []string, options Options) error {
	profile, err := findActiveProfile(options.WorkingDir)
	if err != nil {
		return err
	}

	profilePath := filepath.Join(options.ProfileRoot, profile)
	home := filepath.Join(profilePath, name)
	if info, err := os.Stat(home); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q does not exist; create it with 'agentenv new %s'", profile, profile)
		}
		return fmt.Errorf("inspect profile %q: %w", profile, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s home for profile %q is not a directory", name, profile)
	}
	if err := prepareProfileHome(profilePath); err != nil {
		return fmt.Errorf("prepare home for profile %q: %w", profile, err)
	}
	if err := adoptExistingCredentials(options.ProfileRoot); err != nil {
		return err
	}
	config, err := loadProfileConfig(profilePath)
	if err != nil {
		return fmt.Errorf("configure profile %q: %w", profile, err)
	}
	process := exec.CommandContext(ctx, executable, arguments...)
	process.Dir = options.WorkingDir
	process.Env = replaceEnvironment(os.Environ(), homeVariable, home)
	process.Env = replaceEnvironment(process.Env, "AGENTENV_HOME", options.ProfileRoot)
	if proxyURL := config.Proxy[name]; proxyURL != "" {
		process.Env = replaceEnvironment(process.Env, proxyEnvironmentVariables[name], proxyURL)
	}
	profileHome := filepath.Join(profilePath, "home")
	process.Env = privateHomeEnvironment(process.Env, profileHome)
	process.Stdin = options.Stdin
	process.Stdout = options.Stdout
	process.Stderr = options.Stderr
	processErr := process.Run()
	credentialErr := restoreSharedCredential(home, options.ProfileRoot, name)
	return errors.Join(processErr, credentialErr)
}

func prepareProfileHome(profilePath string) error {
	profileHome := filepath.Join(profilePath, "home")
	info, err := os.Lstat(profileHome)
	if os.IsNotExist(err) {
		if err := os.Mkdir(profileHome, 0o750); err != nil {
			return fmt.Errorf("create profile home: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect profile home: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("profile home is not a directory")
	}
	if err := removeLegacyHomePassthroughs(profileHome); err != nil {
		return err
	}
	aliases := []struct {
		name   string
		target string
	}{
		{name: ".codex", target: filepath.Join("..", "codex")},
		{name: ".claude", target: filepath.Join("..", "claude")},
		{name: ".claude.json", target: filepath.Join("..", "claude", ".claude.json")},
	}
	for _, alias := range aliases {
		link := filepath.Join(profileHome, alias.name)
		info, err := os.Lstat(link)
		if os.IsNotExist(err) {
			if err := os.Symlink(alias.target, link); err != nil {
				return fmt.Errorf("create %s alias: %w", alias.name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s alias: %w", alias.name, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("reserved home alias %s is not a symbolic link", alias.name)
		}
		target, err := os.Readlink(link)
		if err != nil {
			return fmt.Errorf("read %s alias: %w", alias.name, err)
		}
		if target != alias.target {
			return fmt.Errorf("reserved home alias %s has unexpected target %q", alias.name, target)
		}
	}

	agentsHome := filepath.Join(profileHome, ".agents")
	info, err = os.Lstat(agentsHome)
	if os.IsNotExist(err) {
		if err := os.Mkdir(agentsHome, 0o750); err != nil {
			return fmt.Errorf("create .agents directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect .agents directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("reserved home entry .agents is not a directory")
	}

	return nil
}

func removeLegacyHomePassthroughs(profileHome string) error {
	realHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate real home: %w", err)
	}
	entries, err := os.ReadDir(profileHome)
	if err != nil {
		return fmt.Errorf("list profile home: %w", err)
	}
	for _, entry := range entries {
		link := filepath.Join(profileHome, entry.Name())
		info, err := os.Lstat(link)
		if err != nil {
			return fmt.Errorf("inspect profile home entry %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolvedLink, err := filepath.EvalSymlinks(link)
		if err != nil {
			continue
		}
		resolvedRealEntry, err := filepath.EvalSymlinks(filepath.Join(realHome, entry.Name()))
		if err != nil {
			continue
		}
		if filepath.Clean(resolvedLink) != filepath.Clean(resolvedRealEntry) {
			continue
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove legacy home passthrough %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func privateHomeEnvironment(environment []string, profileHome string) []string {
	volume := filepath.VolumeName(profileHome)
	values := []struct {
		key   string
		value string
	}{
		{key: "HOME", value: profileHome},
		{key: "USERPROFILE", value: profileHome},
		{key: "HOMEDRIVE", value: volume},
		{key: "HOMEPATH", value: strings.TrimPrefix(profileHome, volume)},
		{key: "XDG_CONFIG_HOME", value: filepath.Join(profileHome, ".config")},
		{key: "XDG_CACHE_HOME", value: filepath.Join(profileHome, ".cache")},
		{key: "XDG_DATA_HOME", value: filepath.Join(profileHome, ".local", "share")},
		{key: "XDG_STATE_HOME", value: filepath.Join(profileHome, ".local", "state")},
		{key: "APPDATA", value: filepath.Join(profileHome, "AppData", "Roaming")},
		{key: "LOCALAPPDATA", value: filepath.Join(profileHome, "AppData", "Local")},
	}
	for _, variable := range values {
		environment = replaceEnvironment(environment, variable.key, variable.value)
	}
	return environment
}

func restoreSharedCredential(profileHome, profileRoot, tool string) error {
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

	profilePath := filepath.Join(profileHome, credential.profileFile)
	sharedDirectory := filepath.Join(profileRoot, "shared")
	sharedPath := filepath.Join(sharedDirectory, credential.sharedFile)
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
	if err := os.MkdirAll(sharedDirectory, 0o700); err != nil {
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
	target, err := filepath.Rel(profileHome, sharedPath)
	if err != nil {
		return fmt.Errorf("locate shared %s credentials: %w", tool, err)
	}
	if err := os.Symlink(target, profilePath); err != nil {
		return fmt.Errorf("restore shared %s credential link: %w", tool, err)
	}
	return nil
}

func findActiveProfile(start string) (string, error) {
	profile, found, err := findActiveProfileOptional(start)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no active profile; run 'agentenv use <name>'")
	}
	return profile, nil
}

func findActiveProfileOptional(start string) (string, bool, error) {
	directory := start
	for {
		contents, err := os.ReadFile(filepath.Join(directory, ".agentenv"))
		if err == nil {
			profile := strings.TrimSpace(string(contents))
			if profile == "" {
				return "", false, fmt.Errorf("profile selection %q is empty", filepath.Join(directory, ".agentenv"))
			}
			if !validProfileName(profile) {
				return "", false, fmt.Errorf("invalid profile name %q in %s", profile, filepath.Join(directory, ".agentenv"))
			}
			return profile, true, nil
		}
		if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("read profile selection: %w", err)
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false, nil
		}
		directory = parent
	}
}

func replaceEnvironment(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, variable := range environment {
		if !strings.HasPrefix(variable, prefix) {
			result = append(result, variable)
		}
	}
	return append(result, prefix+value)
}
