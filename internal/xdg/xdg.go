// Package xdg resolves the XDG base directories used by stowr.
package xdg

import (
	"os"
	"path/filepath"
)

// ConfigHome returns $XDG_CONFIG_HOME, falling back to $HOME/.config. A
// relative value is ignored, as the XDG Base Directory specification requires.
func ConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	home := Home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}

// StateHome returns $XDG_STATE_HOME, falling back to $HOME/.local/state.
func StateHome() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" && filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	home := Home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state")
}

// Home returns the current user's home directory, or "" when it cannot be
// determined.
func Home() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
