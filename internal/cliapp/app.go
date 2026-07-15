// Package cliapp assembles the agentenv command-line interface on top of
// the profile store in internal/profile.
package cliapp

import (
	"io"
	"os"
	"path/filepath"

	"github.com/ravan/agentenv/internal/profile"
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

// store returns the profile store the CLI operates on.
func (o Options) store() profile.Store {
	return profile.Store{Root: o.ProfileRoot}
}

// New constructs the agentenv command.
func New(options Options) *cli.Command {
	options = withDefaults(options)

	commands := []*cli.Command{
		newCommand(options),
		listCommand(options),
		deleteCommand(options),
		useCommand(options),
		currentCommand(options),
		guardCommand(options),
		runCommand(options),
		integrationCommand("enable", options),
		integrationCommand("disable", options),
		proxyCommand(options),
		skillsCommand(options),
	}
	for _, agent := range profile.Agents {
		commands = append(commands, agentCommand(agent, options))
	}

	return &cli.Command{
		Name:     "agentenv",
		Usage:    "manage isolated profiles for AI coding agents",
		Commands: commands,
	}
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
