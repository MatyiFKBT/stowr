package repo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootWithin(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "dotfiles")
	payload := filepath.Join(rootPath, "pkg", ".config", "app")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(base, "dotfiles-extra")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	root := Root{Path: rootPath}

	if _, ok := root.Within(payload); !ok {
		t.Error("payload must be within the repository")
	}
	if _, ok := root.Within(rootPath); !ok {
		t.Error("the repository root must be within itself")
	}
	for _, outside := range []string{sibling, base, t.TempDir()} {
		if _, ok := root.Within(outside); ok {
			t.Errorf("%s must not be within the repository", outside)
		}
	}
	if _, ok := (Root{}).Within(payload); ok {
		t.Error("an empty root must never match")
	}

	link := filepath.Join(base, "config-app")
	if err := os.Symlink(payload, link); err != nil {
		t.Fatal(err)
	}
	resolved, ok := root.Within(link)
	if !ok || resolved != payload {
		t.Errorf("Within(link) = %q, %v; want %q, true", resolved, ok, payload)
	}
}

func TestFind(t *testing.T) {
	t.Setenv(EnvVar, "")
	base := t.TempDir()
	configHome := filepath.Join(base, "config")
	home := filepath.Join(base, "home")
	want := filepath.Join(home, "dotfiles")
	for _, dir := range []string{configHome, want} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root, err := Find("", configHome, home)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if root.Path != want {
		t.Errorf("Find returned %q, want %q", root.Path, want)
	}

	explicit := filepath.Join(configHome, "dotfiles")
	if err := os.MkdirAll(explicit, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err = Find(explicit, configHome, home)
	if err != nil {
		t.Fatalf("Find explicit: %v", err)
	}
	if root.Path != explicit {
		t.Errorf("Find explicit returned %q, want %q", root.Path, explicit)
	}

	if _, err := Find("", t.TempDir(), filepath.Join(base, "nowhere")); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find with no repository = %v, want ErrNotFound", err)
	}
	if _, err := Find(filepath.Join(base, "missing"), configHome, home); err == nil {
		t.Error("Find with a missing explicit path must fail")
	}
}

func TestPaths(t *testing.T) {
	root := Root{Path: "/repo"}
	if got, want := root.PayloadPath("kitty", "kitty"), filepath.Join("/repo", "kitty", ".config", "kitty"); got != want {
		t.Errorf("PayloadPath = %q, want %q", got, want)
	}
	base := t.TempDir()
	root = Root{Path: base}
	if root.PayloadExists("kitty", "kitty") {
		t.Error("PayloadExists must be false before the payload is created")
	}
	if err := os.MkdirAll(root.PayloadPath("kitty", "kitty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !root.PayloadExists("kitty", "kitty") {
		t.Error("PayloadExists must be true after the payload is created")
	}
	if !root.PackageExists("kitty") || root.PackageExists("alacritty") {
		t.Error("PackageExists reported the wrong packages")
	}
}
