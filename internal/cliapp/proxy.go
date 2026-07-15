package cliapp

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ravan/agentenv/internal/profile"
	"github.com/urfave/cli/v3"
)

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
					if _, ok := profile.AgentByName(tool); !ok {
						return fmt.Errorf("unsupported agent %q", tool)
					}
					if err := validateProxyURL(proxyURL); err != nil {
						return err
					}
					active, profilePath, err := activeProfilePath(options)
					if err != nil {
						return err
					}
					config, err := profile.LoadConfig(profilePath)
					if err != nil {
						return err
					}
					if config.Proxy == nil {
						config.Proxy = map[string]string{}
					}
					config.Proxy[tool] = proxyURL
					if err := profile.SaveConfig(profilePath, config); err != nil {
						return err
					}
					style := newStyler(options.Stdout)
					_, err = fmt.Fprintln(options.Stdout, style.ok(fmt.Sprintf(
						"Set %s proxy %s for profile %s", tool, style.cyan(proxyURL), style.bold(active))))
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
					if _, ok := profile.AgentByName(tool); !ok {
						return fmt.Errorf("unsupported agent %q", tool)
					}
					active, profilePath, err := activeProfilePath(options)
					if err != nil {
						return err
					}
					config, err := profile.LoadConfig(profilePath)
					if err != nil {
						return err
					}
					delete(config.Proxy, tool)
					if err := profile.SaveConfig(profilePath, config); err != nil {
						return err
					}
					style := newStyler(options.Stdout)
					_, err = fmt.Fprintln(options.Stdout, style.ok(fmt.Sprintf(
						"Removed %s proxy for profile %s", tool, style.bold(active))))
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
					config, err := profile.LoadConfig(profilePath)
					if err != nil {
						return err
					}
					for _, agent := range profile.Agents {
						if proxyURL := config.Proxy[agent.Name]; proxyURL != "" {
							if _, err := fmt.Fprintf(options.Stdout, "%s %s\n", agent.Name, proxyURL); err != nil {
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

// activeProfilePath resolves the project's active profile and verifies it
// exists in the store.
func activeProfilePath(options Options) (string, string, error) {
	active, err := profile.FindActive(options.WorkingDir)
	if err != nil {
		return "", "", err
	}
	profilePath, err := options.store().Ensure(active)
	if err != nil {
		return "", "", err
	}
	return active, profilePath, nil
}
