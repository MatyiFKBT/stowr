package scan

import (
	"path/filepath"
	"strings"
)

// configExts lists the file extensions treated as configuration.
var configExts = map[string]bool{
	".toml":  true,
	".yaml":  true,
	".yml":   true,
	".ini":   true,
	".conf":  true,
	".cfg":   true,
	".json":  true,
	".jsonc": true,
	".lua":   true,
	".rc":    true,
	".env":   true,
}

// bareConfigNames lists extensionless names treated as configuration.
var bareConfigNames = map[string]bool{
	"config": true,
}

// generatedNames are files applications rewrite at runtime. Counting them would
// recommend directories that hold no user-authored configuration.
var generatedNames = map[string]bool{
	"metrics.json":       true,
	"state.json":         true,
	"telemetry.json":     true,
	"ephemeral.json":     true,
	"languagepacks.json": true,
}

// generatedSuffixes match runtime state regardless of the application prefix.
var generatedSuffixes = []string{"receipt.json", "window-state.json", "-state.json"}

// generatedPrefixes match runtime state written into dotfiles in $HOME.
var generatedPrefixes = []string{".last"}

// secretNames are configuration files that commonly hold credentials. Stowing
// them publishes secrets to the repository, so they are reported rather than
// silently accepted.
var secretNames = map[string]bool{
	"auth.yml":         true,
	"auth.yaml":        true,
	"auth.json":        true,
	"credentials":      true,
	"credentials.json": true,
	"credentials.yml":  true,
	"credentials.yaml": true,
	"token":            true,
	"token.json":       true,
	"tokens.json":      true,
	"secrets.yml":      true,
	"secrets.yaml":     true,
	"secrets.json":     true,
	"secrets.toml":     true,
	".netrc":           true,
	".env":             true,
}

// backupMarkers mark files and directories that are not the live configuration.
var backupMarkers = []string{
	".bak", ".backup", ".orig", ".old", ".tmp", ".swp",
	"omarchy-upgrade", ".omarchy-",
	".disabled", ".example", ".sample", ".default",
}

// IsConfigFile reports whether name looks like an application configuration
// file.
func IsConfigFile(name string) bool {
	lower := strings.ToLower(name)
	if bareConfigNames[lower] {
		return true
	}
	return configExts[filepath.Ext(lower)]
}

// IsBackup reports whether name is a backup, template or disabled variant of a
// configuration file or directory.
func IsBackup(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "~") {
		return true
	}
	for _, marker := range backupMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// IsGenerated reports whether name is runtime state rather than configuration.
// extra lists additional file names supplied by the user.
func IsGenerated(name string, extra []string) bool {
	lower := strings.ToLower(name)
	if generatedNames[lower] {
		return true
	}
	for _, name := range extra {
		if strings.ToLower(strings.TrimSpace(name)) == lower {
			return true
		}
	}
	for _, suffix := range generatedSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	for _, prefix := range generatedPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// IsSecretish reports whether name commonly holds credentials.
func IsSecretish(name string) bool {
	lower := strings.ToLower(name)
	if secretNames[lower] {
		return true
	}
	return strings.HasPrefix(lower, "credentials")
}
