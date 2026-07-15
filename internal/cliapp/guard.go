package cliapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ravan/agentenv/internal/profile"
	"github.com/urfave/cli/v3"
)

// guardDisplayNames maps a supported agent name to the label used in the
// hook messages shown to the user.
var guardDisplayNames = map[string]string{
	"codex":  "Codex",
	"claude": "Claude Code",
}

// guardCommand validates, from inside an agent's startup hook, that the
// agent was launched through agentenv when the project selects a profile.
func guardCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "guard",
		Usage:     "validate a profiled agent startup hook",
		ArgsUsage: "<codex|claude>",
		Hidden:    true,
		Action: func(_ context.Context, command *cli.Command) error {
			name := command.Args().First()
			agent, ok := profile.AgentByName(name)
			if !ok {
				return fmt.Errorf("unsupported agent %q", name)
			}
			display := guardDisplayNames[agent.Name]
			var input struct {
				WorkingDir string `json:"cwd"`
			}
			if err := json.NewDecoder(options.Stdin).Decode(&input); err != nil {
				return fmt.Errorf("read %s SessionStart hook input: %w", display, err)
			}
			active, found, err := profile.FindActiveOptional(input.WorkingDir)
			if err != nil {
				return err
			}
			if !found {
				return json.NewEncoder(options.Stdout).Encode(struct {
					Continue bool `json:"continue"`
				}{Continue: true})
			}
			expectedHome := filepath.Join(options.ProfileRoot, active, agent.Name)
			requiredHome := fmt.Sprintf("%s=%s", agent.HomeVariable, expectedHome)
			if agent.ClearHomeVariable {
				// The composed home routes the agent's default location
				// into expectedHome; the variable itself must stay unset.
				expectedHome = ""
				requiredHome = fmt.Sprintf("an unset %s", agent.HomeVariable)
			}
			actualHome := os.Getenv(agent.HomeVariable)
			expectedPrivateHome := filepath.Join(options.ProfileRoot, active, "home")
			actualPrivateHome := os.Getenv("HOME")
			result := struct {
				Continue      bool   `json:"continue"`
				StopReason    string `json:"stopReason,omitempty"`
				SystemMessage string `json:"systemMessage,omitempty"`
			}{Continue: actualHome == expectedHome && actualPrivateHome == expectedPrivateHome}
			if !result.Continue {
				result.StopReason = fmt.Sprintf("project selects profile %q and requires %s and HOME=%s; got %s=%q and HOME=%q", active, requiredHome, expectedPrivateHome, agent.HomeVariable, actualHome, actualPrivateHome)
				result.SystemMessage = fmt.Sprintf("%s was launched outside agentenv profile %q. Exit and relaunch with `agentenv %s`.", display, active, agent.Name)
			}
			if err := json.NewEncoder(options.Stdout).Encode(result); err != nil {
				return fmt.Errorf("write %s SessionStart hook result: %w", display, err)
			}
			return nil
		},
	}
}
