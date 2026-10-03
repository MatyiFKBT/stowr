// Package repo locates the stow package repository (typically a "dotfiles"
// checkout) and answers whether a path has already been linked into it.
package repo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// EnvVar overrides repository discovery when set to a directory.
const EnvVar = "STOWR_DOTFILES"

// ErrNotFound reports that no repository could be located.
var ErrNotFound = errors.New("no dotfiles repository found (tried " + EnvVar + ", $XDG_CONFIG_HOME/dotfiles, ~/dotfiles, ~/.dotfiles; pass --dotfiles)")

// Root is a resolved stow repository.
type Root struct {
	// Path is the absolute, symlink-resolved repository root.
	Path string
}

// Candidates returns the discovery order for the repository root.
func Candidates(configHome, home string) []string {
	var out []string
	if v := os.Getenv(EnvVar); v != "" {
		out = append(out, v)
	}
	if configHome != "" {
		out = append(out, filepath.Join(configHome, "dotfiles"))
	}
	if home != "" {
		out = append(out, filepath.Join(home, "dotfiles"), filepath.Join(home, ".dotfiles"))
	}
	return out
}

// Find returns the repository root, preferring an explicit path and then the
// entries of Candidates.
func Find(explicit, configHome, home string) (Root, error) {
	if explicit != "" {
		path, err := resolve(explicit)
		if err != nil {
			return Root{}, err
		}
		return Root{Path: path}, nil
	}
	for _, candidate := range Candidates(configHome, home) {
		if path, err := resolve(candidate); err == nil {
			return Root{Path: path}, nil
		}
	}
	return Root{}, ErrNotFound
}

func resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New(abs + " is not a directory")
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

// Within reports whether path resolves inside the repository and returns the
// resolved location.
func (r Root) Within(path string) (string, bool) {
	if r.Path == "" {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	return resolved, isWithin(r.Path, resolved)
}

// isWithin reports whether path lies inside root. It compares path components,
// so a sibling such as /home/u/dotfiles-extra is not inside /home/u/dotfiles.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// PayloadPath is the repository location a package must hold so that name links
// back into the config directory.
func (r Root) PayloadPath(pkg, name string) string {
	return filepath.Join(r.Path, pkg, ".config", name)
}

// PayloadExists reports whether the package already holds that payload.
func (r Root) PayloadExists(pkg, name string) bool {
	info, err := os.Stat(r.PayloadPath(pkg, name))
	return err == nil && info.IsDir()
}

// PackageExists reports whether the package directory exists.
func (r Root) PackageExists(pkg string) bool {
	info, err := os.Stat(filepath.Join(r.Path, pkg))
	return err == nil && info.IsDir()
}
