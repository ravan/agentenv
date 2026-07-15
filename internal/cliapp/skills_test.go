package cliapp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ravan/agentenv/internal/cliapp"
)

// fakeNpx simulates the skills installer inside the golden repository. An
// add writes one canonical skill plus the usual agent symlink and lock
// file; an update rewrites upstream content, either away from or on top of
// the line the tests curate, selected via FAKE_UPDATE_MODE.
const fakeNpx = `#!/bin/sh
set -e
command="$3"
if [ "$command" = "add" ]; then
	mkdir -p .agents/skills/demo .claude/skills
	cat > .agents/skills/demo/SKILL.md <<'EOF'
# Demo

Intro line.
Line A
Line B
Line C
upstream v1
EOF
	ln -sf ../../.agents/skills/demo .claude/skills/demo
	echo '{"version":1,"skills":{"demo":{"source":"acme/skills"}}}' > skills-lock.json
elif [ "$command" = "update" ]; then
	echo "$*" > update-args.log
	if [ "$FAKE_UPDATE_MODE" = "conflict" ]; then
		cat > .agents/skills/demo/SKILL.md <<'EOF'
# Demo

Intro line.
Line A
Line B
Line C
upstream v2
EOF
	else
		cat > .agents/skills/demo/SKILL.md <<'EOF'
# Demo

Updated intro.
Line A
Line B
Line C
upstream v1
EOF
	fi
fi
`

type skillsFixture struct {
	profileRoot string
	projectRoot string
	goldenDir   string
	binDir      string
	stdout      *bytes.Buffer
	stderr      *bytes.Buffer
	run         func(arguments ...string) error
}

func newSkillsFixture(t *testing.T) *skillsFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &skillsFixture{
		profileRoot: filepath.Join(root, "profiles"),
		projectRoot: filepath.Join(root, "project"),
		goldenDir:   filepath.Join(root, "profiles", "shared", "skills"),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
	}
	hostHome := filepath.Join(root, "host-home")
	binDir := filepath.Join(root, "bin")
	for _, path := range []string{hostHome, fixture.projectRoot, binDir} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	gitConfig := "[user]\n\tname = Test\n\temail = test@example.com\n"
	if err := os.WriteFile(filepath.Join(hostHome, ".gitconfig"), []byte(gitConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "npx"), []byte(fakeNpx), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(hostHome, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(hostHome, ".gitconfig"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := cliapp.New(cliapp.Options{
		ProfileRoot: fixture.profileRoot,
		WorkingDir:  fixture.projectRoot,
		Stdout:      fixture.stdout,
		Stderr:      fixture.stderr,
	})
	fixture.run = func(arguments ...string) error {
		return command.Run(context.Background(), append([]string{"agentenv"}, arguments...))
	}
	fixture.binDir = binDir
	return fixture
}

func (f *skillsFixture) skillFile() string {
	return filepath.Join(f.goldenDir, ".agents", "skills", "demo", "SKILL.md")
}

func (f *skillsFixture) readSkill(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(f.skillFile())
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func (f *skillsFixture) writeSkill(t *testing.T, contents string) {
	t.Helper()
	if err := os.WriteFile(f.skillFile(), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// updateArgsPath is where the fake npx records the arguments of the last
// `skills update` invocation, so tests can assert which skills were updated.
func (f *skillsFixture) updateArgsPath() string {
	return filepath.Join(f.goldenDir, "update-args.log")
}

// installSkill writes an extra canonical skill into the golden store and
// commits it onto the curated branch so the working tree stays clean.
func (f *skillsFixture) installSkill(t *testing.T, name string) {
	t.Helper()
	dir := filepath.Join(f.goldenDir, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.run("skills", "save", "-m", "install "+name); err != nil {
		t.Fatalf("skills save %s: %v", name, err)
	}
}

func TestSkillsAddInstallsUpstreamIntoTheGoldenRepository(t *testing.T) {
	fixture := newSkillsFixture(t)

	if err := fixture.run("skills", "add", "acme/skills"); err != nil {
		t.Fatalf("skills add: %v", err)
	}

	if !strings.Contains(fixture.readSkill(t), "upstream v1") {
		t.Fatalf("canonical skill file missing upstream content")
	}
	head, err := os.ReadFile(filepath.Join(fixture.goldenDir, ".git", "HEAD"))
	if err != nil || strings.TrimSpace(string(head)) != "ref: refs/heads/main" {
		t.Fatalf("HEAD = %q, %v; want the curated branch checked out", head, err)
	}
	if !strings.Contains(fixture.stdout.String(), "Added acme/skills") {
		t.Fatalf("stdout = %q, want add confirmation", fixture.stdout.String())
	}

	fixture.stdout.Reset()
	if err := fixture.run("skills", "list"); err != nil {
		t.Fatalf("skills list: %v", err)
	}
	if got := fixture.stdout.String(); got != "acme/skills\n  ○ demo\n" {
		t.Fatalf("skills list = %q, want disabled demo entry under its source", got)
	}
}

func TestSkillsEnableLinksTheSkillIntoTheActiveProfile(t *testing.T) {
	fixture := newSkillsFixture(t)
	for _, arguments := range [][]string{
		{"new", "dev"}, {"use", "dev"}, {"skills", "add", "acme/skills"},
	} {
		if err := fixture.run(arguments...); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
	}

	if err := fixture.run("skills", "enable", "demo"); err != nil {
		t.Fatalf("skills enable: %v", err)
	}

	canonical, err := filepath.EvalSymlinks(filepath.Join(fixture.goldenDir, ".agents", "skills", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(fixture.profileRoot, "dev")
	links := []string{
		filepath.Join(profilePath, "claude", "skills", "demo"),
		filepath.Join(profilePath, "home", ".agents", "skills", "demo"),
	}
	for _, link := range links {
		resolved, err := filepath.EvalSymlinks(link)
		if err != nil || resolved != canonical {
			t.Fatalf("skill link %s resolves to %q, %v; want %q", link, resolved, err, canonical)
		}
	}
	config, err := os.ReadFile(filepath.Join(profilePath, "config.json"))
	if err != nil || !strings.Contains(string(config), `"demo"`) {
		t.Fatalf("config = %q, %v; want demo enabled", config, err)
	}

	fixture.stdout.Reset()
	if err := fixture.run("skills", "list"); err != nil {
		t.Fatalf("skills list: %v", err)
	}
	if got := fixture.stdout.String(); got != "acme/skills\n  ● demo\n" {
		t.Fatalf("skills list = %q, want enabled demo entry under its source", got)
	}

	if err := fixture.run("skills", "disable", "demo"); err != nil {
		t.Fatalf("skills disable: %v", err)
	}
	for _, link := range links {
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Fatalf("skill link %s still present after disable: %v", link, err)
		}
	}
}

func TestSkillsEnableRejectsAnUnknownSkill(t *testing.T) {
	fixture := newSkillsFixture(t)
	for _, arguments := range [][]string{
		{"new", "dev"}, {"use", "dev"}, {"skills", "add", "acme/skills"},
	} {
		if err := fixture.run(arguments...); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
	}

	err := fixture.run("skills", "enable", "mystery")
	if err == nil || !strings.Contains(err.Error(), `unknown skill "mystery"`) {
		t.Fatalf("enable mystery = %v, want unknown skill error", err)
	}
}

func TestSkillsUpdateReplaysCuratedEditsOnTheNewUpstream(t *testing.T) {
	fixture := newSkillsFixture(t)
	if err := fixture.run("skills", "add", "acme/skills"); err != nil {
		t.Fatalf("skills add: %v", err)
	}
	fixture.writeSkill(t, fixture.readSkill(t)+"curated note\n")
	if err := fixture.run("skills", "save", "-m", "add curated note"); err != nil {
		t.Fatalf("skills save: %v", err)
	}

	t.Setenv("FAKE_UPDATE_MODE", "clean")
	if err := fixture.run("skills", "update", "--all"); err != nil {
		t.Fatalf("skills update: %v", err)
	}

	contents := fixture.readSkill(t)
	if !strings.Contains(contents, "Updated intro.") || !strings.Contains(contents, "curated note") {
		t.Fatalf("skill after update = %q, want upstream change and curated note", contents)
	}
}

func TestSkillsUpdateLeavesAConflictedRebaseToTheUser(t *testing.T) {
	fixture := newSkillsFixture(t)
	if err := fixture.run("skills", "add", "acme/skills"); err != nil {
		t.Fatalf("skills add: %v", err)
	}
	fixture.writeSkill(t, strings.Replace(fixture.readSkill(t), "upstream v1", "locally curated", 1))
	if err := fixture.run("skills", "save", "-m", "curate the version line"); err != nil {
		t.Fatalf("skills save: %v", err)
	}

	t.Setenv("FAKE_UPDATE_MODE", "conflict")
	err := fixture.run("skills", "update", "--all")
	if err == nil || !strings.Contains(err.Error(), "git rebase --continue") {
		t.Fatalf("skills update = %v, want rebase guidance", err)
	}

	fixture.stdout.Reset()
	if err := fixture.run("skills", "status"); err != nil {
		t.Fatalf("skills status: %v", err)
	}
	if !strings.Contains(fixture.stdout.String(), "rebase in progress") {
		t.Fatalf("skills status = %q, want rebase warning", fixture.stdout.String())
	}

	err = fixture.run("skills", "update", "--all")
	if err == nil || !strings.Contains(err.Error(), "rebase is in progress") {
		t.Fatalf("second update = %v, want rebase-in-progress refusal", err)
	}
}

func TestSkillsUpdateDefaultsToEnabledSkills(t *testing.T) {
	fixture := newSkillsFixture(t)
	for _, arguments := range [][]string{
		{"new", "dev"}, {"use", "dev"}, {"skills", "add", "acme/skills"},
	} {
		if err := fixture.run(arguments...); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
	}
	// A second installed-but-not-enabled skill must be left out of the update.
	fixture.installSkill(t, "other")
	if err := fixture.run("skills", "enable", "demo"); err != nil {
		t.Fatalf("skills enable: %v", err)
	}

	if err := fixture.run("skills", "update"); err != nil {
		t.Fatalf("skills update: %v", err)
	}

	logged, err := os.ReadFile(fixture.updateArgsPath())
	if err != nil {
		t.Fatalf("read update args: %v", err)
	}
	if !strings.Contains(string(logged), "demo") {
		t.Fatalf("update args = %q, want the enabled skill \"demo\"", logged)
	}
	if strings.Contains(string(logged), "other") {
		t.Fatalf("update args = %q, want the disabled skill \"other\" excluded", logged)
	}
}

func TestSkillsUpdateAllUpdatesEverything(t *testing.T) {
	fixture := newSkillsFixture(t)
	if err := fixture.run("skills", "add", "acme/skills"); err != nil {
		t.Fatalf("skills add: %v", err)
	}

	if err := fixture.run("skills", "update", "--all"); err != nil {
		t.Fatalf("skills update --all: %v", err)
	}

	logged, err := os.ReadFile(fixture.updateArgsPath())
	if err != nil {
		t.Fatalf("read update args: %v", err)
	}
	if strings.Contains(string(logged), "--all") {
		t.Fatalf("update args = %q, want the --all sentinel stripped", logged)
	}
	if strings.Contains(string(logged), "demo") {
		t.Fatalf("update args = %q, want no skill name forwarded for a full update", logged)
	}
}

func TestSkillsUpdateWithNoEnabledSkillsDoesNothing(t *testing.T) {
	fixture := newSkillsFixture(t)
	if err := fixture.run("skills", "add", "acme/skills"); err != nil {
		t.Fatalf("skills add: %v", err)
	}
	fixture.stdout.Reset()

	if err := fixture.run("skills", "update"); err != nil {
		t.Fatalf("skills update: %v", err)
	}

	if !strings.Contains(fixture.stdout.String(), "no enabled skills") {
		t.Fatalf("stdout = %q, want the no-enabled-skills notice", fixture.stdout.String())
	}
	if _, err := os.Stat(fixture.updateArgsPath()); !os.IsNotExist(err) {
		t.Fatalf("update-args.log exists (%v); want the installer never invoked", err)
	}
}

func TestSkillsSaveReportsACleanRepository(t *testing.T) {
	fixture := newSkillsFixture(t)
	if err := fixture.run("skills", "add", "acme/skills"); err != nil {
		t.Fatalf("skills add: %v", err)
	}
	fixture.stdout.Reset()

	if err := fixture.run("skills", "save"); err != nil {
		t.Fatalf("skills save: %v", err)
	}
	if !strings.Contains(fixture.stdout.String(), "Nothing to save") {
		t.Fatalf("stdout = %q, want nothing-to-save notice", fixture.stdout.String())
	}
}

func TestAgentLaunchReconcilesSkillLinks(t *testing.T) {
	fixture := newSkillsFixture(t)
	fakeClaude := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(fixture.binDir, "claude"), []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"new", "dev"}, {"use", "dev"}, {"skills", "add", "acme/skills"}, {"skills", "enable", "demo"},
	} {
		if err := fixture.run(arguments...); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
	}
	skillsDir := filepath.Join(fixture.profileRoot, "dev", "claude", "skills")
	if err := os.Remove(filepath.Join(skillsDir, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fixture.goldenDir, ".agents", "skills", "ghost"), filepath.Join(skillsDir, "ghost")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "mine"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fixture.projectRoot, filepath.Join(skillsDir, "theirs")); err != nil {
		t.Fatal(err)
	}

	if err := fixture.run("claude"); err != nil {
		t.Fatalf("launch claude: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(skillsDir, "demo")); err != nil {
		t.Fatalf("enabled skill link was not recreated: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "ghost")); !os.IsNotExist(err) {
		t.Fatalf("stale golden-store link survived the launch: %v", err)
	}
	for _, name := range []string{"mine", "theirs"} {
		if _, err := os.Lstat(filepath.Join(skillsDir, name)); err != nil {
			t.Fatalf("user-owned entry %q was removed: %v", name, err)
		}
	}
}
