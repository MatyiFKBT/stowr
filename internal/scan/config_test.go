package scan

import "testing"

func TestIsConfigFile(t *testing.T) {
	tests := map[string]bool{
		"alacritty.toml": true,
		"config.yml":     true,
		"foot.ini":       true,
		"kitty.conf":     true,
		"qalc.cfg":       true,
		"settings.json":  true,
		"hyprland.jsonc": true,
		"init.lua":       true,
		"config":         true,
		"README.md":      false,
		"notes.txt":      false,
		"pinentry":       false,
	}
	for name, want := range tests {
		if got := IsConfigFile(name); got != want {
			t.Errorf("IsConfigFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsBackup(t *testing.T) {
	tests := map[string]bool{
		"bindings.conf.bak.1782297671":                         true,
		"bindings.conf.backup.20251101_121729":                 true,
		"autostart.conf~":                                      true,
		"conf.default.json":                                    true,
		"walker.omarchy-upgrade-to-quattro.20260825205748.bak": true,
		"hyprland.conf":                                        false,
		"alacritty.toml":                                       false,
		"bindings":                                             false,
		"oldscroll.conf":                                       false,
	}
	for name, want := range tests {
		if got := IsBackup(name); got != want {
			t.Errorf("IsBackup(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsGenerated(t *testing.T) {
	tests := map[string]bool{
		"uv-receipt.json":         true,
		"telemetry.json":          true,
		"languagepacks.json":      true,
		"metrics.json":            true,
		"state.json":              true,
		".window-state.json":      true,
		".last_update_check.json": true,
		"config.json":             false,
		"alacritty.toml":          false,
	}
	for name, want := range tests {
		if got := IsGenerated(name, nil); got != want {
			t.Errorf("IsGenerated(%q) = %v, want %v", name, got, want)
		}
	}
	if !IsGenerated("custom-cache.json", []string{"custom-cache.json"}) {
		t.Error("IsGenerated must honour user-supplied deny names")
	}
}

func TestIsSecretish(t *testing.T) {
	for _, name := range []string{"auth.yml", "credentials.json", ".env", ".netrc", "credentials"} {
		if !IsSecretish(name) {
			t.Errorf("IsSecretish(%q) = false, want true", name)
		}
	}
	if IsSecretish("config.yml") {
		t.Error("IsSecretish(config.yml) = true, want false")
	}
}
