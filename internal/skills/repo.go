// Package skills manages the golden skills repository: a git repository in
// the profile store's shared area whose upstream branch only ever receives
// pristine installer output and whose curated branch carries the user's
// edits as commits rebased on top. Profiles link individual skills out of
// the curated working tree.
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// UpstreamBranch tracks pristine `npx skills` output.
	UpstreamBranch = "upstream"
	// CuratedBranch is the checked-out branch profiles link into: the
	// upstream state plus the user's curation commits.
	CuratedBranch = "main"
)

// Repo drives git and the skills installer inside the golden repository.
type Repo struct {
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// RepoDir returns the golden repository's location under the profile store
// root, next to the shared credential files.
func RepoDir(profileRoot string) string {
	return filepath.Join(profileRoot, "shared", "skills")
}

// CanonicalDir returns the directory holding the single canonical copy of
// every installed skill. The installer writes it when asked to install for
// all agents, following the agentskills.io project layout.
func (r Repo) CanonicalDir() string {
	return filepath.Join(r.Dir, ".agents", "skills")
}

// Status describes the golden repository for display.
type Status struct {
	Initialized      bool
	Branch           string
	RebaseInProgress bool
	DirtyFiles       []string
	CuratedCommits   []string
	Skills           []string
}

// Add installs a skill package on the upstream branch and rebases the
// curated branch on top. Arguments are passed through to `skills add`, so
// installer options like `-s <skill>` work unchanged.
func (r Repo) Add(ctx context.Context, arguments []string) error {
	if err := r.ensure(ctx); err != nil {
		return err
	}
	if err := r.requireReady(ctx); err != nil {
		return err
	}
	installer := append([]string{"-y", "skills@latest", "add"}, arguments...)
	installer = append(installer, "-a", "*", "-y")
	return r.applyUpstream(ctx, installer, "skills add "+strings.Join(arguments, " "))
}

// Update blindly updates the installed skills on the upstream branch and
// rebases the curated branch, replaying the user's edits on the new
// upstream state. Arguments are passed through to `skills update`.
func (r Repo) Update(ctx context.Context, arguments []string) error {
	if err := r.ensure(ctx); err != nil {
		return err
	}
	if err := r.requireReady(ctx); err != nil {
		return err
	}
	installer := append([]string{"-y", "skills@latest", "update"}, arguments...)
	installer = append(installer, "-y")
	return r.applyUpstream(ctx, installer, "skills update "+strings.Join(arguments, " "))
}

// Save commits the user's working-tree edits onto the curated branch and
// reports whether there was anything to save.
func (r Repo) Save(ctx context.Context, message string) (bool, error) {
	if err := r.ensure(ctx); err != nil {
		return false, err
	}
	if r.rebaseInProgress() {
		return false, r.rebaseError()
	}
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return false, err
	}
	if branch != CuratedBranch {
		return false, fmt.Errorf("the skills repository has %q checked out; switch it back to %q first", branch, CuratedBranch)
	}
	dirty, err := r.dirtyFiles(ctx)
	if err != nil {
		return false, err
	}
	if len(dirty) == 0 {
		return false, nil
	}
	if err := r.git(ctx, "add", "--all"); err != nil {
		return false, err
	}
	if err := r.git(ctx, "commit", "--quiet", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// Status inspects the golden repository without changing it.
func (r Repo) Status(ctx context.Context) (Status, error) {
	var status Status
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); os.IsNotExist(err) {
		return status, nil
	} else if err != nil {
		return status, fmt.Errorf("inspect skills repository: %w", err)
	}
	status.Initialized = true
	status.RebaseInProgress = r.rebaseInProgress()
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return status, err
	}
	status.Branch = branch
	status.DirtyFiles, err = r.dirtyFiles(ctx)
	if err != nil {
		return status, err
	}
	log, err := r.gitOutput(ctx, "log", "--format=%s", UpstreamBranch+".."+CuratedBranch)
	if err != nil {
		return status, err
	}
	if log != "" {
		status.CuratedCommits = strings.Split(log, "\n")
	}
	status.Skills, err = r.Skills()
	return status, err
}

// Skills lists the installed skill names in directory order. A missing
// repository or canonical directory means no skills.
func (r Repo) Skills() ([]string, error) {
	entries, err := os.ReadDir(r.CanonicalDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// Sources maps each installed skill to the upstream package it came from
// (e.g. "mattpocock/skills"), as recorded by the installer in
// skills-lock.json. A missing lock file means no known sources.
func (r Repo) Sources() (map[string]string, error) {
	contents, err := os.ReadFile(filepath.Join(r.Dir, "skills-lock.json"))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skills lock file: %w", err)
	}
	var lock struct {
		Skills map[string]struct {
			Source string `json:"source"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(contents, &lock); err != nil {
		return nil, fmt.Errorf("parse skills lock file: %w", err)
	}
	sources := make(map[string]string, len(lock.Skills))
	for name, entry := range lock.Skills {
		sources[name] = entry.Source
	}
	return sources, nil
}

// applyUpstream runs one installer invocation on the upstream branch,
// commits its output, and rebases the curated branch on top. A failed
// installer leaves the repository back on the curated branch; a failed
// rebase is left in place for the user to resolve with their git tooling.
func (r Repo) applyUpstream(ctx context.Context, installer []string, message string) error {
	if err := r.git(ctx, "switch", "--quiet", UpstreamBranch); err != nil {
		return err
	}
	if err := r.npx(ctx, installer...); err != nil {
		_ = r.git(ctx, "reset", "--hard", "--quiet")
		_ = r.git(ctx, "clean", "--force", "-d", "--quiet")
		_ = r.git(ctx, "switch", "--quiet", CuratedBranch)
		return err
	}
	if err := r.git(ctx, "add", "--all"); err != nil {
		return err
	}
	dirty, err := r.dirtyFiles(ctx)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		if err := r.git(ctx, "commit", "--quiet", "-m", message); err != nil {
			return err
		}
	}
	if err := r.git(ctx, "switch", "--quiet", CuratedBranch); err != nil {
		return err
	}
	if err := r.git(ctx, "rebase", UpstreamBranch); err != nil {
		return fmt.Errorf("replay curated edits onto the new upstream: %w\nresolve the conflicts in %s with your git tooling (lazygit, git), then run 'git rebase --continue'", err, r.Dir)
	}
	return nil
}

// ensure creates the golden repository on first use: an empty root commit
// shared by the curated and upstream branches, with the curated branch
// checked out.
func (r Repo) ensure(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect skills repository: %w", err)
	}
	if err := os.MkdirAll(r.Dir, 0o750); err != nil {
		return fmt.Errorf("create skills repository: %w", err)
	}
	if err := r.git(ctx, "init", "--quiet", "--initial-branch", CuratedBranch); err != nil {
		return err
	}
	if err := r.git(ctx, "commit", "--quiet", "--allow-empty", "-m", "initialize skills repository"); err != nil {
		return err
	}
	return r.git(ctx, "branch", UpstreamBranch)
}

// requireReady rejects branch-switching operations while a rebase is
// unresolved or the curated working tree has unsaved edits.
func (r Repo) requireReady(ctx context.Context) error {
	if r.rebaseInProgress() {
		return r.rebaseError()
	}
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	if branch != CuratedBranch {
		return fmt.Errorf("the skills repository has %q checked out; switch it back to %q first", branch, CuratedBranch)
	}
	dirty, err := r.dirtyFiles(ctx)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		return fmt.Errorf("the skills repository has unsaved edits; run 'agentenv skills save' first")
	}
	return nil
}

func (r Repo) rebaseInProgress() bool {
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(r.Dir, ".git", marker)); err == nil {
			return true
		}
	}
	return false
}

func (r Repo) rebaseError() error {
	return fmt.Errorf("a rebase is in progress in %s; resolve it with your git tooling, then run 'git rebase --continue'", r.Dir)
}

func (r Repo) currentBranch(ctx context.Context) (string, error) {
	return r.gitOutput(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

func (r Repo) dirtyFiles(ctx context.Context) ([]string, error) {
	output, err := r.gitOutput(ctx, "status", "--porcelain")
	if err != nil || output == "" {
		return nil, err
	}
	return strings.Split(output, "\n"), nil
}

// git runs a git command inside the repository with output wired to the
// caller's streams.
func (r Repo) git(ctx context.Context, arguments ...string) error {
	process := exec.CommandContext(ctx, "git", arguments...)
	process.Dir = r.Dir
	process.Stdin = r.Stdin
	process.Stdout = r.Stdout
	process.Stderr = r.Stderr
	if err := process.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
	}
	return nil
}

// gitOutput runs a git command and captures its trimmed standard output.
func (r Repo) gitOutput(ctx context.Context, arguments ...string) (string, error) {
	process := exec.CommandContext(ctx, "git", arguments...)
	process.Dir = r.Dir
	process.Stderr = r.Stderr
	output, err := process.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

// npx runs the skills installer inside the repository.
func (r Repo) npx(ctx context.Context, arguments ...string) error {
	process := exec.CommandContext(ctx, "npx", arguments...)
	process.Dir = r.Dir
	process.Stdin = r.Stdin
	process.Stdout = r.Stdout
	process.Stderr = r.Stderr
	if err := process.Run(); err != nil {
		return fmt.Errorf("npx %s: %w", strings.Join(arguments, " "), err)
	}
	return nil
}
