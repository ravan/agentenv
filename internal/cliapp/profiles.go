package cliapp

import (
	"context"
	"fmt"
	"io"
	"strings"

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
			style := newStyler(options.Stdout)
			_, err = fmt.Fprintln(options.Stdout, style.ok("Created profile "+style.bold(name)))
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
			style := newStyler(options.Stdout)
			_, err = fmt.Fprintln(options.Stdout, style.ok("Deleted profile "+style.bold(name)))
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
			style := newStyler(options.Stdout)
			_, err = fmt.Fprintln(options.Stdout, style.ok("Using profile "+style.bold(name)))
			return err
		},
	}
}

func currentCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "current",
		Usage: "summarize the active profile",
		Action: func(_ context.Context, _ *cli.Command) error {
			active, err := profile.FindActive(options.WorkingDir)
			if err != nil {
				return err
			}
			config, err := profile.LoadConfig(options.store().Path(active))
			if err != nil {
				return err
			}
			style := newStyler(options.Stdout)
			type row struct{ icon, label, value string }
			var rows []row
			for _, tool := range integrationTools {
				value := style.dim("○ disabled")
				if config.Integrations[tool] {
					value = style.green("● enabled")
				}
				rows = append(rows, row{integrationIcons[tool], tool, value})
			}
			for _, agent := range profile.Agents {
				value := style.dim("(not set)")
				if proxyURL := config.Proxy[agent.Name]; proxyURL != "" {
					value = style.cyan(proxyURL)
				}
				rows = append(rows, row{iconProxy, agent.Name + " proxy", value})
			}
			width := 0
			for _, r := range rows {
				width = max(width, len(r.label))
			}
			var output strings.Builder
			output.WriteString(style.cyan(iconProfile) + " " + style.bold(active) + "\n")
			for _, r := range rows {
				fmt.Fprintf(&output, "  %s %-*s  %s\n", style.dim(r.icon), width, r.label, r.value)
			}
			_, err = io.WriteString(options.Stdout, output.String())
			return err
		},
	}
}
