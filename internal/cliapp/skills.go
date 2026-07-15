package cliapp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ravan/agentenv/internal/profile"
	"github.com/ravan/agentenv/internal/skills"
	"github.com/urfave/cli/v3"
)

// goldenRepo returns the shared golden skills repository the CLI operates
// on, with its git and installer output wired to the user's terminal.
func goldenRepo(options Options) skills.Repo {
	return skills.Repo{
		Dir:    skills.RepoDir(options.ProfileRoot),
		Stdin:  options.Stdin,
		Stdout: options.Stdout,
		Stderr: options.Stderr,
	}
}

func skillsCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "skills",
		Usage: "manage curated agent skills shared by all profiles",
		Commands: []*cli.Command{
			skillsAddCommand(options),
			skillsUpdateCommand(options),
			skillsSaveCommand(options),
			skillsToggleCommand("enable", options),
			skillsToggleCommand("disable", options),
			skillsListCommand(options),
			skillsStatusCommand(options),
		},
	}
}

func skillsAddCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:            "add",
		Usage:           "install a skill package into the shared skills repository",
		ArgsUsage:       "<owner/repo> [installer options]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, command *cli.Command) error {
			arguments := command.Args().Slice()
			if len(arguments) == 0 {
				return cli.Exit("skill package source is required", 2)
			}
			if err := goldenRepo(options).Add(ctx, arguments); err != nil {
				return err
			}
			style := newStyler(options.Stdout)
			_, err := fmt.Fprintln(options.Stdout, style.ok("Added "+style.bold(arguments[0])+
				" to the skills repository; enable skills per profile with 'agentenv skills enable <name>'"))
			return err
		},
	}
}

func skillsUpdateCommand(options Options) *cli.Command {
	return &cli.Command{
		Name: "update",
		Usage: "update enabled skills and replay curated edits on top " +
			"(use --all for every installed skill, or name specific skills)",
		ArgsUsage:       "[--all] [skills...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, command *cli.Command) error {
			var names, flags []string
			full := false
			for _, argument := range command.Args().Slice() {
				switch {
				case argument == "--all":
					full = true
				case strings.HasPrefix(argument, "-"):
					flags = append(flags, argument) // installer flag, forwarded as-is
				default:
					names = append(names, argument) // explicit skill name
				}
			}

			switch {
			case len(names) > 0 && full:
				return cli.Exit("cannot combine --all with explicit skill names", 2)
			case len(names) > 0:
				// Explicit named update.
				return runUpdate(ctx, options, append(flags, names...))
			case full:
				// Full update: forward flags only so the installer updates everything.
				return runUpdate(ctx, options, flags)
			default:
				// Default: update only the skills enabled across all profiles.
				enabled, err := enabledSkillNames(options)
				if err != nil {
					return err
				}
				if len(enabled) == 0 {
					// Return here rather than fall through: an empty argument list
					// makes the installer update every skill, the slow path we avoid.
					style := newStyler(options.Stdout)
					_, err := fmt.Fprintln(options.Stdout, style.dim(
						"no enabled skills to update; enable skills with 'agentenv skills enable <name>' "+
							"or run 'agentenv skills update --all'"))
					return err
				}
				return runUpdate(ctx, options, append(flags, enabled...))
			}
		},
	}
}

// runUpdate applies one skills update in the golden repository and reports it.
func runUpdate(ctx context.Context, options Options, arguments []string) error {
	if err := goldenRepo(options).Update(ctx, arguments); err != nil {
		return err
	}
	style := newStyler(options.Stdout)
	_, err := fmt.Fprintln(options.Stdout, style.ok("Updated skills and replayed curated edits"))
	return err
}

// enabledSkillNames returns the sorted union of skills enabled across every
// profile, restricted to skills actually installed in the golden store so no
// uninstalled name is ever handed to the installer.
func enabledSkillNames(options Options) ([]string, error) {
	installed, err := goldenRepo(options).Skills()
	if err != nil {
		return nil, err
	}
	installedSet := map[string]bool{}
	for _, name := range installed {
		installedSet[name] = true
	}

	store := options.store()
	profiles, err := store.List()
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, name := range profiles {
		config, err := profile.LoadConfig(store.Path(name))
		if err != nil {
			return nil, err
		}
		for _, skill := range config.Skills {
			if installedSet[skill] {
				enabled[skill] = true
			}
		}
	}
	names := make([]string, 0, len(enabled))
	for name := range enabled {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func skillsSaveCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "save",
		Usage: "commit skill edits as a curated change that survives updates",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   "commit message for the curated change",
				Value:   "curate skills",
			},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			saved, err := goldenRepo(options).Save(ctx, command.String("message"))
			if err != nil {
				return err
			}
			style := newStyler(options.Stdout)
			if !saved {
				_, err := fmt.Fprintln(options.Stdout, style.dim("Nothing to save; the skills repository is clean"))
				return err
			}
			_, err = fmt.Fprintln(options.Stdout, style.ok("Saved curated skill edits"))
			return err
		},
	}
}

func skillsToggleCommand(action string, options Options) *cli.Command {
	return &cli.Command{
		Name:      action,
		Usage:     action + " shared skills in the active profile",
		ArgsUsage: "<skill> [skills...]",
		Action: func(_ context.Context, command *cli.Command) error {
			names := command.Args().Slice()
			if len(names) == 0 {
				return cli.Exit("skill name is required", 2)
			}
			repo := goldenRepo(options)
			if action == "enable" {
				available, err := repo.Skills()
				if err != nil {
					return err
				}
				installed := map[string]bool{}
				for _, name := range available {
					installed[name] = true
				}
				for _, name := range names {
					if !installed[name] {
						return fmt.Errorf("unknown skill %q; run 'agentenv skills list' to see installed skills", name)
					}
				}
			}
			active, profilePath, err := activeProfilePath(options)
			if err != nil {
				return err
			}
			config, err := profile.LoadConfig(profilePath)
			if err != nil {
				return err
			}
			enabled := map[string]bool{}
			for _, name := range config.Skills {
				enabled[name] = true
			}
			for _, name := range names {
				if action == "enable" {
					enabled[name] = true
				} else {
					delete(enabled, name)
				}
			}
			config.Skills = nil
			for name := range enabled {
				config.Skills = append(config.Skills, name)
			}
			sort.Strings(config.Skills)
			if err := profile.SaveConfig(profilePath, config); err != nil {
				return err
			}
			missing, err := profile.SyncSkills(profilePath, repo.CanonicalDir(), config.Skills)
			if err != nil {
				return fmt.Errorf("sync skills for profile %q: %w", active, err)
			}
			warnMissingSkills(options, active, missing)
			style := newStyler(options.Stdout)
			message := "Enabled " + style.bold(strings.Join(names, ", ")) + " for profile " + style.bold(active)
			if action == "disable" {
				message = "Disabled " + style.bold(strings.Join(names, ", ")) + " for profile " + style.bold(active)
			}
			_, err = fmt.Fprintln(options.Stdout, style.ok(message))
			return err
		},
	}
}

func skillsListCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list shared skills by upstream package and whether the active profile enables them",
		Action: func(_ context.Context, _ *cli.Command) error {
			repo := goldenRepo(options)
			names, err := repo.Skills()
			if err != nil {
				return err
			}
			style := newStyler(options.Stdout)
			if len(names) == 0 {
				_, err := fmt.Fprintln(options.Stdout, style.dim("no skills installed; run 'agentenv skills add <owner/repo>'"))
				return err
			}
			sources, err := repo.Sources()
			if err != nil {
				return err
			}
			enabled := map[string]bool{}
			if active, found, err := profile.FindActiveOptional(options.WorkingDir); err == nil && found {
				config, err := profile.LoadConfig(options.store().Path(active))
				if err != nil {
					return err
				}
				for _, name := range config.Skills {
					enabled[name] = true
				}
			}
			const unknownSource = "(unknown source)"
			groups := map[string][]string{}
			for _, name := range names {
				source := sources[name]
				if source == "" {
					source = unknownSource
				}
				groups[source] = append(groups[source], name)
			}
			ordered := make([]string, 0, len(groups))
			for source := range groups {
				if source != unknownSource {
					ordered = append(ordered, source)
				}
			}
			sort.Strings(ordered)
			if _, ok := groups[unknownSource]; ok {
				ordered = append(ordered, unknownSource)
			}
			for _, source := range ordered {
				if _, err := fmt.Fprintln(options.Stdout, style.bold(source)); err != nil {
					return err
				}
				sort.Strings(groups[source])
				for _, name := range groups[source] {
					marker := style.dim("○")
					if enabled[name] {
						marker = style.green("●")
					}
					if _, err := fmt.Fprintf(options.Stdout, "  %s %s\n", marker, name); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

func skillsStatusCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "summarize the shared skills repository",
		Action: func(ctx context.Context, _ *cli.Command) error {
			repo := goldenRepo(options)
			status, err := repo.Status(ctx)
			if err != nil {
				return err
			}
			style := newStyler(options.Stdout)
			if !status.Initialized {
				_, err := fmt.Fprintln(options.Stdout, style.dim("no skills repository yet; run 'agentenv skills add <owner/repo>'"))
				return err
			}
			var output strings.Builder
			output.WriteString(style.cyan(iconSkills) + " " + style.bold(repo.Dir) + "\n")
			output.WriteString("  branch    " + status.Branch + "\n")
			output.WriteString(fmt.Sprintf("  skills    %d installed\n", len(status.Skills)))
			if len(status.CuratedCommits) > 0 {
				output.WriteString("  curated   " + fmt.Sprintf("%d change(s)\n", len(status.CuratedCommits)))
				for _, subject := range status.CuratedCommits {
					output.WriteString("    " + style.dim("• "+subject) + "\n")
				}
			}
			if status.RebaseInProgress {
				output.WriteString("  " + style.bold("rebase in progress") +
					"; resolve conflicts with your git tooling, then run 'git rebase --continue'\n")
			}
			if len(status.DirtyFiles) > 0 {
				output.WriteString(fmt.Sprintf("  unsaved   %d file(s); run 'agentenv skills save'\n", len(status.DirtyFiles)))
				for _, file := range status.DirtyFiles {
					output.WriteString("    " + style.dim(file) + "\n")
				}
			}
			_, err = fmt.Fprint(options.Stdout, output.String())
			return err
		},
	}
}

// warnMissingSkills reports enabled skills that are missing from the shared
// repository, e.g. after an upstream update removed them.
func warnMissingSkills(options Options, active string, missing []string) {
	for _, name := range missing {
		fmt.Fprintf(options.Stderr, "warning: skill %q enabled in profile %q is missing from the skills repository\n", name, active)
	}
}
