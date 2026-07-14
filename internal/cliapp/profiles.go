package cliapp

import (
	"context"
	"fmt"

	"github.com/ravan/agentenv/internal/profile"
	"github.com/urfave/cli/v3"
)

// requireProfileName extracts and validates the profile name argument.
func requireProfileName(command *cli.Command) (string, error) {
	name := command.Args().First()
	if name == "" {
		return "", cli.Exit("profile name is required", 2)
	}
	if !profile.ValidName(name) {
		return "", fmt.Errorf("invalid profile name %q", name)
	}
	return name, nil
}

func newCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "create a profile",
		ArgsUsage: "<name>",
		Action: func(_ context.Context, command *cli.Command) error {
			name, err := requireProfileName(command)
			if err != nil {
				return err
			}
			if err := options.store().Create(name); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Created profile %s\n", name)
			return err
		},
	}
}

func listCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list profiles",
		Action: func(_ context.Context, _ *cli.Command) error {
			names, err := options.store().List()
			if err != nil {
				return err
			}
			for _, name := range names {
				if _, err := fmt.Fprintln(options.Stdout, name); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func deleteCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "delete a profile",
		ArgsUsage: "<name>",
		Action: func(_ context.Context, command *cli.Command) error {
			name, err := requireProfileName(command)
			if err != nil {
				return err
			}
			if err := options.store().Delete(name); err != nil {
				return err
			}
			_, err = fmt.Fprintf(options.Stdout, "Deleted profile %s\n", name)
			return err
		},
	}
}

func useCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "use",
		Aliases:   []string{"activate"},
		Usage:     "select a profile for the current project",
		ArgsUsage: "<name>",
		Action: func(_ context.Context, command *cli.Command) error {
			name, err := requireProfileName(command)
			if err != nil {
				return err
			}
			if _, err := options.store().Ensure(name); err != nil {
				return err
			}
			if err := profile.WriteSelection(options.WorkingDir, name); err != nil {
				return fmt.Errorf("activate profile %q: %w", name, err)
			}
			_, err = fmt.Fprintf(options.Stdout, "Using profile %s\n", name)
			return err
		},
	}
}

func currentCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "current",
		Usage: "print the active profile",
		Action: func(_ context.Context, _ *cli.Command) error {
			active, err := profile.FindActive(options.WorkingDir)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(options.Stdout, active)
			return err
		},
	}
}
