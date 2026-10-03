// Package apply moves a config directory into the stow repository and links it
// back into place.
package apply

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"stowr/internal/repo"
	"stowr/internal/scan"
)

// Options controls how a candidate is applied.
type Options struct {
	Repo      repo.Root
	ConfigDir string
	Target    string
	// BackupRoot is the directory that receives a copy of every applied entry.
	BackupRoot string
	// StowBin is the stow executable to use. When empty the link is created
	// natively instead of shelling out.
	StowBin string
	// Out receives the output of the stow process.
	Out io.Writer
	// Now is the clock used to name backups; nil means time.Now.
	Now func() time.Time
}

// Plan describes the changes an apply performs.
type Plan struct {
	Entry      scan.Entry
	PackageDir string // repository payload directory
	LinkPath   string // path the link is created at
	LinkTarget string // link destination, relative to LinkPath's directory
	BackupDir  string // directory holding the pre-apply copy
	UsesStow   bool
}

// Prepare validates that entry can be applied and returns the planned changes.
func Prepare(entry scan.Entry, opts Options) (Plan, error) {
	switch entry.Status {
	case scan.Candidate:
	case scan.PartiallyStowed:
		return Plan{}, fmt.Errorf("already partially stowed; run `stow -R -t %s %s`", opts.Target, entry.Package)
	default:
		return Plan{}, fmt.Errorf("status is %s", entry.Status)
	}
	if entry.Kind != scan.KindDir {
		return Plan{}, errors.New("only directories can be moved into the repository")
	}
	if opts.BackupRoot == "" {
		return Plan{}, errors.New("no backup directory configured; refusing to move anything")
	}

	packageDir := opts.Repo.PayloadPath(entry.Package, entry.Name)
	if _, err := os.Lstat(packageDir); err == nil {
		return Plan{}, fmt.Errorf("package payload already exists at %s", packageDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Plan{}, err
	}
	if _, err := os.Lstat(entry.Path); err != nil {
		return Plan{}, err
	}

	linkTarget, err := filepath.Rel(opts.ConfigDir, packageDir)
	if err != nil {
		return Plan{}, err
	}
	backupDir, err := reserveBackupDir(opts.BackupRoot, entry.Name, opts.clock())
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Entry:      entry,
		PackageDir: packageDir,
		LinkPath:   entry.Path,
		LinkTarget: linkTarget,
		BackupDir:  backupDir,
		UsesStow:   opts.StowBin != "",
	}, nil
}

// BackupPath is the location the pre-apply copy of the entry is written to.
func (p Plan) BackupPath() string {
	return filepath.Join(p.BackupDir, p.Entry.Name)
}

// RestoreCommand returns a shell command that puts the backup back in place.
func (p Plan) RestoreCommand() string {
	return fmt.Sprintf("rm -f %s && mv %s %s", p.LinkPath, p.BackupPath(), p.Entry.Path)
}

// Apply executes a plan. The directory is copied into the backup root first,
// then moved into the repository and linked back, either with stow or, when
// stow is unavailable, with a symlink.
func Apply(plan Plan, opts Options) error {
	if plan.BackupDir == "" {
		return errors.New("refusing to apply without a backup directory")
	}
	if err := os.MkdirAll(plan.BackupDir, 0o700); err != nil {
		return err
	}
	if err := copyTree(plan.Entry.Path, plan.BackupPath()); err != nil {
		return fmt.Errorf("backup %s: %w", plan.Entry.Name, err)
	}
	if err := os.MkdirAll(filepath.Dir(plan.PackageDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(plan.Entry.Path, plan.PackageDir); err != nil {
		return fmt.Errorf("move %s into repository (backup kept at %s): %w", plan.Entry.Name, plan.BackupPath(), err)
	}
	if plan.UsesStow {
		cmd := exec.Command(opts.StowBin, "-t", opts.Target, plan.Entry.Package)
		cmd.Dir = opts.Repo.Path
		cmd.Stdout = opts.Out
		cmd.Stderr = opts.Out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("stow: %w (payload is at %s; backup at %s)", err, plan.PackageDir, plan.BackupPath())
		}
		return nil
	}
	if err := os.Symlink(plan.LinkTarget, plan.LinkPath); err != nil {
		return fmt.Errorf("link %s: %w (payload is at %s; backup at %s)", plan.Entry.Name, err, plan.PackageDir, plan.BackupPath())
	}
	return nil
}

func (o Options) clock() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// reserveBackupDir creates a uniquely named backup directory for name.
func reserveBackupDir(root, name string, now time.Time) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create backup root: %w", err)
	}
	stamp := now.Format("20060102T150405")
	for attempt := 0; ; attempt++ {
		candidate := filepath.Join(root, fmt.Sprintf("%s-%s", stamp, name))
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", candidate, attempt)
		}
		if err := os.Mkdir(candidate, 0o700); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create backup directory: %w", err)
		}
	}
}

// copyTree copies src to dst, preserving file modes and recreating symlinks.
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()|0o700); err != nil {
			return err
		}
		children, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := copyTree(filepath.Join(src, child.Name()), filepath.Join(dst, child.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		return copyFile(src, dst, info.Mode().Perm())
	default:
		return nil // sockets, fifos and devices are not worth backing up
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
