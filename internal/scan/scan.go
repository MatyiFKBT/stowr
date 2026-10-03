// Package scan classifies the children of an XDG config directory by how
// suitable each one is for management with GNU stow.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MatyiFKBT/stowr/internal/repo"
)

// Status classifies an entry.
type Status int

const (
	// Candidate is not linked yet and holds a stow-able number of config files.
	Candidate Status = iota
	// AlreadyStowed is a symlink resolving inside the repository.
	AlreadyStowed
	// PartiallyStowed is a real directory holding symlinks into the repository.
	PartiallyStowed
	// LinkedElsewhere is a symlink resolving outside the repository.
	LinkedElsewhere
	// NoConfigFiles holds fewer config files than requested.
	NoConfigFiles
	// TooManyConfigFiles holds more config files than requested.
	TooManyConfigFiles
	// TooManySubdirs holds more subdirectories than requested, which marks a
	// data or cache tree rather than a configuration directory.
	TooManySubdirs
)

// String implements fmt.Stringer.
func (s Status) String() string {
	switch s {
	case Candidate:
		return "candidate"
	case AlreadyStowed:
		return "already-stowed"
	case PartiallyStowed:
		return "partially-stowed"
	case LinkedElsewhere:
		return "linked-elsewhere"
	case NoConfigFiles:
		return "no-config"
	case TooManyConfigFiles:
		return "too-many-config"
	case TooManySubdirs:
		return "too-many-subdirs"
	default:
		return "unknown"
	}
}

// Kind distinguishes directories from plain files.
type Kind int

const (
	// KindDir is a directory in the config home.
	KindDir Kind = iota
	// KindFile is a plain file in the config home.
	KindFile
)

// String implements fmt.Stringer.
func (k Kind) String() string {
	if k == KindFile {
		return "file"
	}
	return "dir"
}

// Entry is one child of the config directory with its classification.
type Entry struct {
	Name           string
	Package        string // stow package name that would hold this entry
	Path           string // absolute path of the entry
	Kind           Kind
	Status         Status
	ConfigFiles    []string // paths relative to Path
	Secrets        []string // subset of ConfigFiles that look like credentials
	LinkedFiles    []string // paths relative to Path already linked into the repo
	ForeignLinks   int      // symlinks pointing outside the repository
	Subdirs        []string // immediate subdirectory names
	ResolvedTarget string   // symlink destination, when linked
	PackagePath    string   // repository location the package must hold
	PackageExists  bool     // the package payload already exists in the repository
}

// VerboseHint describes details that matter only when links or subdirectories
// are involved. It returns "" when there is nothing extra to say.
func (e Entry) VerboseHint() string {
	var parts []string
	if len(e.Subdirs) > 0 {
		parts = append(parts, "subdirs: "+strings.Join(e.Subdirs, ", "))
	}
	if e.ForeignLinks > 0 {
		parts = append(parts, fmt.Sprintf("%d symlink(s) pointing outside the repository", e.ForeignLinks))
	}
	return strings.Join(parts, "; ")
}

// Options controls a scan.
type Options struct {
	// ConfigDir is the directory whose children are classified.
	ConfigDir string
	// Repo is the stow repository used to detect existing links.
	Repo repo.Root
	// MinFiles and MaxFiles bound the number of config files a candidate holds.
	MinFiles int
	MaxFiles int
	// MaxSubdirs bounds the number of immediate subdirectories a candidate
	// may hold; zero allows none and a negative value disables the check.
	MaxSubdirs int
	// Depth is how many levels below an app directory are inspected;
	// -1 means unlimited.
	Depth int
	// IncludeBackups keeps backup, disabled and template entries.
	IncludeBackups bool
	// IncludeFiles also considers plain files in ConfigDir.
	IncludeFiles bool
	// Deny lists additional generated file names.
	Deny []string
}

// Scan classifies every child of opts.ConfigDir. Results keep the sorted order
// of the directory listing.
func Scan(opts Options) ([]Entry, error) {
	children, err := os.ReadDir(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(children))
	for _, child := range children {
		name := child.Name()
		if IsBackup(name) && !opts.IncludeBackups {
			continue
		}
		path := filepath.Join(opts.ConfigDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			continue // raced away or unreadable; not worth reporting
		}
		entry := Entry{Name: name, Package: name, Path: path}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			entry.Kind = KindDir
			if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
				entry.Kind = KindFile
			}
			entry.Package = packageName(name, entry.Kind)
			if target, err := filepath.EvalSymlinks(path); err == nil {
				entry.ResolvedTarget = target
			}
			if _, ok := opts.Repo.Within(path); ok {
				entry.Status = AlreadyStowed
			} else {
				entry.Status = LinkedElsewhere
			}
		case info.IsDir():
			entry.Kind = KindDir
			classifyDir(&entry, opts)
		default:
			if !opts.IncludeFiles || !IsConfigFile(name) || IsGenerated(name, opts.Deny) {
				continue
			}
			entry.Kind = KindFile
			entry.Package = packageName(name, KindFile)
			entry.Status = Candidate
			entry.ConfigFiles = []string{name}
			if IsSecretish(name) {
				entry.Secrets = []string{name}
			}
		}
		entry.PackagePath = opts.Repo.PayloadPath(entry.Package, entry.Name)
		entry.PackageExists = opts.Repo.PayloadExists(entry.Package, entry.Name)
		entries = append(entries, entry)
	}
	return entries, nil
}

// classifyDir fills in the status and file lists of a real directory.
func classifyDir(entry *Entry, opts Options) {
	linked := 0
	_ = filepath.WalkDir(entry.Path, func(path string, dirent fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: report what is reachable
		}
		if path == entry.Path {
			return nil
		}
		rel, relErr := filepath.Rel(entry.Path, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if opts.Depth >= 0 && depth > opts.Depth {
			if dirent.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if dirent.IsDir() {
			if depth == 1 {
				entry.Subdirs = append(entry.Subdirs, dirent.Name())
			}
			if opts.Depth >= 0 && depth >= opts.Depth {
				return filepath.SkipDir // nothing below can be within the depth limit
			}
			return nil
		}
		if dirent.Type()&os.ModeSymlink != 0 {
			if _, ok := opts.Repo.Within(path); ok {
				linked++
				entry.LinkedFiles = append(entry.LinkedFiles, rel)
			} else {
				entry.ForeignLinks++
			}
			return nil // never descend through links
		}
		if !dirent.Type().IsRegular() {
			return nil
		}
		name := dirent.Name()
		if IsBackup(name) && !opts.IncludeBackups {
			return nil
		}
		if IsGenerated(name, opts.Deny) || !IsConfigFile(name) {
			return nil
		}
		entry.ConfigFiles = append(entry.ConfigFiles, rel)
		if IsSecretish(name) {
			entry.Secrets = append(entry.Secrets, rel)
		}
		return nil
	})
	switch {
	case linked > 0:
		entry.Status = PartiallyStowed
	case opts.MaxSubdirs >= 0 && len(entry.Subdirs) > opts.MaxSubdirs:
		entry.Status = TooManySubdirs
	case len(entry.ConfigFiles) < opts.MinFiles:
		entry.Status = NoConfigFiles
	case len(entry.ConfigFiles) > opts.MaxFiles:
		entry.Status = TooManyConfigFiles
	default:
		entry.Status = Candidate
	}
	sort.Strings(entry.ConfigFiles)
	sort.Strings(entry.LinkedFiles)
	sort.Strings(entry.Secrets)
	sort.Strings(entry.Subdirs)
}

// packageName derives the stow package name for an entry. Plain files in the
// config home lose their extension, so starship.toml becomes the starship
// package; directory names are used verbatim.
func packageName(name string, kind Kind) string {
	if kind == KindFile {
		if ext := filepath.Ext(name); ext != "" {
			return strings.TrimSuffix(name, ext)
		}
	}
	return name
}
