package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stowr/internal/repo"
	"stowr/internal/scan"
)

type fixture struct {
	configDir string
	repoDir   string
	backupDir string
	root      repo.Root
	entry     scan.Entry
	fixed     time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	configDir := filepath.Join(base, "config")
	repoDir := filepath.Join(base, "dotfiles")
	backupDir := filepath.Join(base, "state", "stowr", "backups")
	app := filepath.Join(configDir, "kitty")
	for _, dir := range []string{app, repoDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(app, "kitty.conf"), "font_size 12\n")
	write(filepath.Join(app, "themes", "dark.conf"), "background #000\n")
	if err := os.Symlink("kitty.conf", filepath.Join(app, "linked.conf")); err != nil {
		t.Fatal(err)
	}

	root := repo.Root{Path: repoDir}
	return fixture{
		configDir: configDir,
		repoDir:   repoDir,
		backupDir: backupDir,
		root:      root,
		fixed:     time.Date(2026, 10, 3, 18, 30, 0, 0, time.UTC),
		entry: scan.Entry{
			Name:    "kitty",
			Package: "kitty",
			Path:    app,
			Kind:    scan.KindDir,
			Status:  scan.Candidate,
		},
	}
}

func (f fixture) options() Options {
	return Options{
		Repo:       f.root,
		ConfigDir:  f.configDir,
		Target:     filepath.Dir(f.configDir),
		BackupRoot: f.backupDir,
		Now:        func() time.Time { return f.fixed },
	}
}

func TestApplyBacksUpThenMovesAndLinks(t *testing.T) {
	f := newFixture(t)
	plan, err := Prepare(f.entry, f.options())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	wantBackup := filepath.Join(f.backupDir, "20261003T183000-kitty")
	if plan.BackupDir != wantBackup {
		t.Errorf("backup dir = %q, want %q", plan.BackupDir, wantBackup)
	}
	if plan.LinkTarget != filepath.Join("..", "dotfiles", "kitty", ".config", "kitty") {
		t.Errorf("link target = %q", plan.LinkTarget)
	}

	if err := Apply(plan, f.options()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The entry moved into the repository.
	if _, err := os.Stat(filepath.Join(plan.PackageDir, "kitty.conf")); err != nil {
		t.Errorf("payload missing after apply: %v", err)
	}

	// The backup holds an untouched copy, including the nested file and the
	// relative symlink.
	for _, rel := range []string{"kitty.conf", filepath.Join("themes", "dark.conf")} {
		content, err := os.ReadFile(filepath.Join(plan.BackupPath(), rel))
		if err != nil {
			t.Errorf("backup %s: %v", rel, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("backup %s is empty", rel)
		}
	}
	linkTarget, err := os.Readlink(filepath.Join(plan.BackupPath(), "linked.conf"))
	if err != nil {
		t.Errorf("backup symlink: %v", err)
	} else if linkTarget != "kitty.conf" {
		t.Errorf("backup symlink target = %q, want %q", linkTarget, "kitty.conf")
	}
	if info, err := os.Stat(filepath.Join(plan.BackupPath(), "kitty.conf")); err != nil {
		t.Errorf("backup mode: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, want 0600", info.Mode().Perm())
	}

	// The original path is a symlink resolving back into the repository.
	link, err := os.Readlink(f.entry.Path)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if link != plan.LinkTarget {
		t.Errorf("link = %q, want %q", link, plan.LinkTarget)
	}
	resolved, err := filepath.EvalSymlinks(f.entry.Path)
	if err != nil {
		t.Fatalf("resolve link: %v", err)
	}
	if resolved != plan.PackageDir {
		t.Errorf("link resolves to %q, want %q", resolved, plan.PackageDir)
	}
	if content, err := os.ReadFile(filepath.Join(f.entry.Path, "kitty.conf")); err != nil || string(content) != "font_size 12\n" {
		t.Errorf("config unreadable through the link: %q %v", content, err)
	}

	if !strings.Contains(plan.RestoreCommand(), plan.BackupPath()) {
		t.Errorf("restore command %q does not mention %q", plan.RestoreCommand(), plan.BackupPath())
	}
}

func TestPrepareRejectsUnsafeEntries(t *testing.T) {
	f := newFixture(t)

	existing := f.entry
	existing.Status = scan.PartiallyStowed
	if _, err := Prepare(existing, f.options()); err == nil {
		t.Error("Prepare must reject partially stowed entries")
	}

	wrongKind := f.entry
	wrongKind.Kind = scan.KindFile
	if _, err := Prepare(wrongKind, f.options()); err == nil {
		t.Error("Prepare must reject plain files")
	}

	noBackup := f.options()
	noBackup.BackupRoot = ""
	if _, err := Prepare(f.entry, noBackup); err == nil {
		t.Error("Prepare must refuse to run without a backup directory")
	}

	if err := os.MkdirAll(filepath.Join(f.repoDir, "kitty", ".config", "kitty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(f.entry, f.options()); err == nil {
		t.Error("Prepare must reject an existing payload")
	}
}

func TestApplyWithoutBackupDirFails(t *testing.T) {
	f := newFixture(t)
	if err := Apply(Plan{Entry: f.entry, PackageDir: filepath.Join(f.repoDir, "kitty", ".config", "kitty")}, f.options()); err == nil {
		t.Error("Apply must refuse to run without a backup directory")
	}
}

func TestPrepareRejectsMissingSource(t *testing.T) {
	f := newFixture(t)
	missing := f.entry
	missing.Path = filepath.Join(f.configDir, "ghost")
	if _, err := Prepare(missing, f.options()); err == nil {
		t.Error("Prepare must reject a missing source directory")
	}
}
