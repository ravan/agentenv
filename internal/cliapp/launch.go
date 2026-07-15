package cliapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ravan/agentenv/internal/profile"
	"github.com/urfave/cli/v3"
)

func agentCommand(agent profile.Agent, options Options) *cli.Command {
	return &cli.Command{
		Name:            agent.Name,
		Usage:           "launch " + agent.Name + " with the active profile",
		ArgsUsage:       "[arguments...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, command *cli.Command) error {
			return runProfileProcess(ctx, agent, agent.Name, command.Args().Slice(), options)
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
			agent, ok := profile.AgentByName(arguments[0])
			if !ok {
				return fmt.Errorf("unsupported agent %q", arguments[0])
			}
			arguments = arguments[1:]
			if len(arguments) > 0 && arguments[0] == "--" {
				arguments = arguments[1:]
			}
			if len(arguments) == 0 {
				return cli.Exit("wrapper command is required", 2)
			}
			return runProfileProcess(ctx, agent, arguments[0], arguments[1:], options)
		},
	}
}

// runProfileProcess executes a command inside the active profile: the
// agent's home variable, the composed private home, and any configured proxy
// endpoint all point into the profile. After the process exits, refreshed
// credentials are folded back into the shared store.
func runProfileProcess(ctx context.Context, agent profile.Agent, executable string, arguments []string, options Options) error {
	store := options.store()
	active, err := profile.FindActive(options.WorkingDir)
	if err != nil {
		return err
	}

	home, err := store.AgentHome(active, agent.Name)
	if err != nil {
		return err
	}
	profilePath := store.Path(active)
	if err := profile.PrepareHome(profilePath); err != nil {
		return fmt.Errorf("prepare home for profile %q: %w", active, err)
	}
	if err := store.AdoptExistingCredentials(); err != nil {
		return err
	}
	if err := store.EnsureSharedCredentialLinks(profilePath); err != nil {
		return err
	}
	if err := profile.SeedClaudeOnboarding(filepath.Join(profilePath, "claude")); err != nil {
		return err
	}
	config, err := profile.LoadConfig(profilePath)
	if err != nil {
		return fmt.Errorf("configure profile %q: %w", active, err)
	}
	missingSkills, err := profile.SyncSkills(profilePath, goldenRepo(options).CanonicalDir(), config.Skills)
	if err != nil {
		return fmt.Errorf("sync skills for profile %q: %w", active, err)
	}
	warnMissingSkills(options, active, missingSkills)
	process := exec.CommandContext(ctx, executable, arguments...)
	process.Dir = options.WorkingDir
	process.Env = agent.HomeEnvironment(os.Environ(), home)
	process.Env = profile.ReplaceEnvironment(process.Env, "AGENTENV_HOME", options.ProfileRoot)
	if proxyURL := config.Proxy[agent.Name]; proxyURL != "" {
		process.Env = profile.ReplaceEnvironment(process.Env, agent.ProxyVariable, proxyURL)
	}
	process.Env = profile.PrivateHomeEnvironment(process.Env, filepath.Join(profilePath, "home"))
	process.Stdin = options.Stdin
	process.Stdout = options.Stdout
	process.Stderr = options.Stderr
	processErr := process.Run()
	credentialErr := store.RestoreSharedCredential(home, agent.Name)
	return errors.Join(processErr, credentialErr)
}
