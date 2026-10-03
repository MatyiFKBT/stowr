package scan

import (
	"os"
	"path/filepath"
	"testing"

	"stowr/internal/repo"
)

// tree builds a synthetic config directory and repository.
type tree struct {
	base      string
	configDir string
	repoDir   string
}

func newTree(t *testing.T) tree {
	t.Helper()
	base := t.TempDir()
	tr := tree{
		base:      base,
		configDir: filepath.Join(base, "config"),
		repoDir:   filepath.Join(base, "dotfiles"),
	}
	mustMkdir(t, tr.configDir)
	mustMkdir(t, tr.repoDir)

	// Plain candidates.
	mustWrite(t, filepath.Join(tr.configDir, "alacritty", "alacritty.toml"), "theme = 'x'\n")
	mustWrite(t, filepath.Join(tr.configDir, "gh", "config.yml"), "version: 1\n")
	mustWrite(t, filepath.Join(tr.configDir, "gh", "hosts.yml"), "github.com:\n")

	// Too many config files.
	for _, name := range []string{"a.conf", "b.conf", "c.conf"} {
		mustWrite(t, filepath.Join(tr.configDir, "busy", name), "x\n")
	}

	// No config files.
	mustWrite(t, filepath.Join(tr.configDir, "empty", "notes.txt"), "x\n")

	// Only backup files, and a backup-named directory.
	mustWrite(t, filepath.Join(tr.configDir, "archived", "config.toml.bak"), "x\n")
	mustWrite(t, filepath.Join(tr.configDir, "walker.omarchy-upgrade-to-quattro.20260825205748.bak", "config.toml"), "x\n")

	// Only runtime state.
	mustWrite(t, filepath.Join(tr.configDir, "generated", "uv-receipt.json"), "{}\n")
	mustWrite(t, filepath.Join(tr.configDir, "generated", ".last_update_check.json"), "{}\n")

	// Credentials lookalike.
	mustWrite(t, filepath.Join(tr.configDir, "immich", "auth.yml"), "token: secret\n")

	// Whole directory linked into the repository.
	mustMkdir(t, filepath.Join(tr.repoDir, "mypkg", ".config", "linked"))
	mustSymlink(t, filepath.Join(tr.repoDir, "mypkg", ".config", "linked"), filepath.Join(tr.configDir, "linked"))

	// Directory holding a mix of local and repository-linked files.
	mustWrite(t, filepath.Join(tr.configDir, "partial", "partial.conf"), "x\n")
	mustWrite(t, filepath.Join(tr.repoDir, "other", ".config", "partial", "fromrepo.conf"), "x\n")
	mustSymlink(t, filepath.Join(tr.repoDir, "other", ".config", "partial", "fromrepo.conf"), filepath.Join(tr.configDir, "partial", "fromrepo.conf"))

	// Directory holding a symlink that points outside the repository.
	mustMkdir(t, filepath.Join(tr.base, "outside"))
	mustWrite(t, filepath.Join(tr.base, "outside", "x.conf"), "x\n")
	mustSymlink(t, filepath.Join(tr.base, "outside"), filepath.Join(tr.configDir, "elsewhere"))

	// Config files below the first level.
	mustWrite(t, filepath.Join(tr.configDir, "deep", "nested", "config.toml"), "x\n")

	// A cache tree: one config file but many data subdirectories.
	mustWrite(t, filepath.Join(tr.configDir, "cache", "data.json"), "{}\n")
	for _, sub := range []string{"Cache", "Code Cache", "GPUCache", "IndexedDB"} {
		mustMkdir(t, filepath.Join(tr.configDir, "cache", sub))
	}

	// Plain file directly in the config directory.
	mustWrite(t, filepath.Join(tr.configDir, "starship.toml"), "x\n")
	return tr
}

func (tr tree) options() Options {
	return Options{
		ConfigDir:  tr.configDir,
		Repo:       repo.Root{Path: tr.repoDir},
		MinFiles:   1,
		MaxFiles:   2,
		MaxSubdirs: 3,
		Depth:      1,
	}
}

func (tr tree) scan(t *testing.T, options Options) map[string]Entry {
	t.Helper()
	entries, err := Scan(options)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byName := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	return byName
}

func TestScanClassifiesEntries(t *testing.T) {
	tr := newTree(t)
	entries := tr.scan(t, tr.options())

	tests := []struct {
		name        string
		status      Status
		files       []string
		packageName string
	}{
		{"alacritty", Candidate, []string{"alacritty.toml"}, "alacritty"},
		{"gh", Candidate, []string{"config.yml", "hosts.yml"}, "gh"},
		{"busy", TooManyConfigFiles, []string{"a.conf", "b.conf", "c.conf"}, "busy"},
		{"empty", NoConfigFiles, nil, "empty"},
		{"archived", NoConfigFiles, nil, "archived"},
		{"generated", NoConfigFiles, nil, "generated"},
		{"immich", Candidate, []string{"auth.yml"}, "immich"},
		{"cache", TooManySubdirs, []string{"data.json"}, "cache"},
		{"linked", AlreadyStowed, nil, "linked"},
		{"elsewhere", LinkedElsewhere, nil, "elsewhere"},
		{"deep", NoConfigFiles, nil, "deep"},
	}
	for _, test := range tests {
		entry, ok := entries[test.name]
		if !ok {
			t.Errorf("%s: missing from scan", test.name)
			continue
		}
		if entry.Status != test.status {
			t.Errorf("%s: status = %s, want %s", test.name, entry.Status, test.status)
		}
		if entry.Package != test.packageName {
			t.Errorf("%s: package = %q, want %q", test.name, entry.Package, test.packageName)
		}
		if len(test.files) != len(entry.ConfigFiles) {
			t.Errorf("%s: config files = %v, want %v", test.name, entry.ConfigFiles, test.files)
			continue
		}
		for i, file := range test.files {
			if entry.ConfigFiles[i] != file {
				t.Errorf("%s: config files = %v, want %v", test.name, entry.ConfigFiles, test.files)
				break
			}
		}
	}

	if got := entries["immich"].Secrets; len(got) != 1 || got[0] != "auth.yml" {
		t.Errorf("immich secrets = %v, want [auth.yml]", got)
	}
	partial := entries["partial"]
	if partial.Status != PartiallyStowed {
		t.Errorf("partial: status = %s, want %s", partial.Status, PartiallyStowed)
	}
	if len(partial.LinkedFiles) != 1 || partial.LinkedFiles[0] != "fromrepo.conf" {
		t.Errorf("partial: linked files = %v, want [fromrepo.conf]", partial.LinkedFiles)
	}
	if len(partial.ConfigFiles) != 1 || partial.ConfigFiles[0] != "partial.conf" {
		t.Errorf("partial: config files = %v, want [partial.conf]", partial.ConfigFiles)
	}
	if _, ok := entries["walker.omarchy-upgrade-to-quattro.20260825205748.bak"]; ok {
		t.Error("backup-named directory must be skipped")
	}
	if _, ok := entries["starship.toml"]; ok {
		t.Error("plain file must be skipped without IncludeFiles")
	}
	if entries["alacritty"].PackageExists {
		t.Error("alacritty payload does not exist yet")
	}
	mustMkdir(t, filepath.Join(tr.repoDir, "alacritty", ".config", "alacritty"))
	if !tr.scan(t, tr.options())["alacritty"].PackageExists {
		t.Error("PackageExists must report an existing payload")
	}
}

func TestScanHonoursDepth(t *testing.T) {
	tr := newTree(t)

	shallow := tr.options()
	shallow.Depth = 1
	if got := tr.scan(t, shallow)["deep"].Status; got != NoConfigFiles {
		t.Errorf("depth 1: deep status = %s, want %s", got, NoConfigFiles)
	}

	deep := tr.options()
	deep.Depth = 2
	entry := tr.scan(t, deep)["deep"]
	if entry.Status != Candidate {
		t.Fatalf("depth 2: deep status = %s, want %s", entry.Status, Candidate)
	}
	if len(entry.ConfigFiles) != 1 || entry.ConfigFiles[0] != "nested/config.toml" {
		t.Errorf("depth 2: deep config files = %v, want [nested/config.toml]", entry.ConfigFiles)
	}
}

func TestScanIncludeFlags(t *testing.T) {
	tr := newTree(t)

	options := tr.options()
	options.IncludeBackups = true
	options.IncludeFiles = true
	entries := tr.scan(t, options)

	if _, ok := entries["walker.omarchy-upgrade-to-quattro.20260825205748.bak"]; !ok {
		t.Error("IncludeBackups must keep backup-named directories")
	}
	file, ok := entries["starship.toml"]
	if !ok {
		t.Fatal("IncludeFiles must consider plain files")
	}
	if file.Kind != KindFile || file.Status != Candidate {
		t.Errorf("starship.toml: kind = %s status = %s, want file/candidate", file.Kind, file.Status)
	}
	if file.Package != "starship" {
		t.Errorf("starship.toml: package = %q, want %q", file.Package, "starship")
	}
}

func TestScanHonoursThresholds(t *testing.T) {
	tr := newTree(t)
	options := tr.options()
	options.MinFiles = 2
	options.MaxFiles = 2
	entries := tr.scan(t, options)

	if got := entries["gh"].Status; got != Candidate {
		t.Errorf("gh: status = %s, want %s", got, Candidate)
	}
	if got := entries["alacritty"].Status; got != NoConfigFiles {
		t.Errorf("alacritty: status = %s, want %s", got, NoConfigFiles)
	}
}

func TestScanHonoursDeny(t *testing.T) {
	tr := newTree(t)
	options := tr.options()
	options.Deny = []string{"alacritty.toml"}
	if got := tr.scan(t, options)["alacritty"].Status; got != NoConfigFiles {
		t.Errorf("alacritty: status = %s, want %s", got, NoConfigFiles)
	}
}

func TestScanHonoursSubdirBound(t *testing.T) {
	tr := newTree(t)

	unlimited := tr.options()
	unlimited.MaxSubdirs = -1
	entry := tr.scan(t, unlimited)["cache"]
	if entry.Status != Candidate {
		t.Fatalf("unlimited: cache status = %s, want %s", entry.Status, Candidate)
	}
	if len(entry.Subdirs) != 4 {
		t.Errorf("cache subdirs = %v, want 4 entries", entry.Subdirs)
	}

	strict := tr.options()
	strict.MaxSubdirs = 0
	if got := tr.scan(t, strict)["cache"].Status; got != TooManySubdirs {
		t.Errorf("MaxSubdirs 0: cache status = %s, want %s", got, TooManySubdirs)
	}
	if got := tr.scan(t, strict)["alacritty"].Status; got != Candidate {
		t.Errorf("MaxSubdirs 0: alacritty status = %s, want %s", got, Candidate)
	}
}

func TestSummarize(t *testing.T) {
	summary := Summarize([]Entry{
		{Status: Candidate},
		{Status: Candidate},
		{Status: PartiallyStowed},
		{Status: NoConfigFiles},
		{Status: TooManySubdirs},
	})
	if summary.Candidates != 2 || summary.PartiallyStowed != 1 || summary.NoConfigFiles != 1 ||
		summary.TooManySubdirs != 1 || summary.Total != 5 {
		t.Errorf("unexpected summary: %+v", summary)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}
