package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ravan/agentenv/internal/profile"
)

func TestSyncSkillsReportsSkillsMissingFromTheStore(t *testing.T) {
	root := t.TempDir()
	profilePath := filepath.Join(root, "profile")
	canonical := filepath.Join(root, "golden", ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(canonical, "present"), 0o750); err != nil {
		t.Fatal(err)
	}

	missing, err := profile.SyncSkills(profilePath, canonical, []string{"present", "vanished"})
	if err != nil {
		t.Fatalf("sync skills: %v", err)
	}

	if len(missing) != 1 || missing[0] != "vanished" {
		t.Fatalf("missing = %v, want [vanished]", missing)
	}
	link := filepath.Join(profilePath, "claude", "skills", "present")
	if target, err := os.Readlink(link); err != nil || filepath.IsAbs(target) {
		t.Fatalf("present link = %q, %v; want a relative symlink", target, err)
	}
	if _, err := os.Lstat(filepath.Join(profilePath, "claude", "skills", "vanished")); !os.IsNotExist(err) {
		t.Fatalf("vanished skill got a link anyway: %v", err)
	}
}

func TestSyncSkillsRejectsUnsafeSkillNames(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "golden", ".agents", "skills")

	for _, name := range []string{"", ".", "..", "../evil", "nested/name"} {
		if _, err := profile.SyncSkills(filepath.Join(root, "profile"), canonical, []string{name}); err == nil {
			t.Fatalf("sync skills accepted unsafe name %q", name)
		}
	}
}
