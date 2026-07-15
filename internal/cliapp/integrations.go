package cliapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ravan/agentenv/internal/profile"
	"github.com/urfave/cli/v3"
)

// integrationSteps maps a helper tool and action to the tool's own installer
// invocations, one per agent home. The installers resolve every path through
// HOME (or the agent home variables), so running them inside the profile
// environment writes to the profile instead of the real global config.
// integrationTools lists the supported helper tools in display order.
var integrationTools = []string{"rtk", "tokensave"}

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
			active, profilePath, err := activeProfilePath(options)
			if err != nil {
				return err
			}
			if err := runIntegrationSteps(ctx, active, profilePath, steps, options); err != nil {
				return err
			}
			config, err := profile.LoadConfig(profilePath)
			if err != nil {
				return err
			}
			if config.Integrations == nil {
				config.Integrations = map[string]bool{}
			}
			config.Integrations[tool] = action == "enable"
			return profile.SaveConfig(profilePath, config)
		},
	}
}

func runIntegrationSteps(ctx context.Context, active, profilePath string, steps [][]string, options Options) error {
	store := options.store()
	environment := profile.ReplaceEnvironment(os.Environ(), "AGENTENV_HOME", options.ProfileRoot)
	for _, agent := range profile.Agents {
		home, err := store.AgentHome(active, agent.Name)
		if err != nil {
			return err
		}
		environment = agent.HomeEnvironment(environment, home)
	}
	if err := profile.PrepareHome(profilePath); err != nil {
		return fmt.Errorf("prepare home for profile %q: %w", active, err)
	}
	environment = profile.PrivateHomeEnvironment(environment, filepath.Join(profilePath, "home"))
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
