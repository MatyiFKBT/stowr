// Command stowr recommends XDG config directories that are ready to be managed
// with GNU stow, and can move them into the repository for you.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/MatyiFKBT/stowr/internal/apply"
	"github.com/MatyiFKBT/stowr/internal/repo"
	"github.com/MatyiFKBT/stowr/internal/report"
	"github.com/MatyiFKBT/stowr/internal/scan"
	"github.com/MatyiFKBT/stowr/internal/xdg"
)

// version is stamped at release time with -ldflags "-X main.version=...".
// When it is empty, the version recorded by `go install <module>@<version>` in
// the build info is used instead.
var version = ""

// buildVersion reports the version to display.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "devel"
}

// stringList collects a repeatable, comma-separated flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "stowr: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("stowr", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var (
		configDirFlag  = flags.String("config-dir", "", "config directory to scan (default $XDG_CONFIG_HOME or ~/.config)")
		dotfilesFlag   = flags.String("dotfiles", "", "stow repository (default $STOWR_DOTFILES, $XDG_CONFIG_HOME/dotfiles, ~/dotfiles, ~/.dotfiles)")
		targetFlag     = flags.String("target", "", "stow target directory (default the config directory's parent)")
		backupFlag     = flags.String("backup-dir", "", "where --apply stores backups (default $XDG_STATE_HOME/stowr/backups)")
		minFiles       = flags.Int("min-files", 1, "minimum number of config files for a candidate")
		maxFiles       = flags.Int("max-files", 2, "maximum number of config files for a candidate")
		maxSubdirs     = flags.Int("max-subdirs", 3, "maximum number of subdirectories for a candidate (-1 = unlimited)")
		depth          = flags.Int("depth", 1, "subdirectory levels inspected inside each entry (-1 = unlimited)")
		includeBackups = flags.Bool("include-backups", false, "include backup, disabled and template entries")
		includeFiles   = flags.Bool("include-files", false, "also consider plain files in the config directory")
		asJSON         = flags.Bool("json", false, "emit JSON")
		noColor        = flags.Bool("no-color", false, "disable ANSI colour")
		verbose        = flags.Bool("verbose", false, "list skipped entries and extra detail")
		doApply        = flags.Bool("apply", false, "move candidates into the repository and link them (backed up first)")
		assumeYes      = flags.Bool("yes", false, "skip the confirmation prompt used by --apply")
		showVersion    = flags.Bool("version", false, "print the version and exit")
		deny           stringList
	)
	flags.Var(&deny, "deny", "additional generated file names to ignore (repeatable, comma separated)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintf(stdout, "stowr %s\n", buildVersion())
		return nil
	}
	if *minFiles < 0 || *maxFiles < *minFiles {
		return fmt.Errorf("invalid thresholds: min-files=%d max-files=%d", *minFiles, *maxFiles)
	}
	if *depth < -1 {
		return fmt.Errorf("depth must be >= -1, got %d", *depth)
	}
	if *maxSubdirs < -1 {
		return fmt.Errorf("max-subdirs must be >= -1, got %d", *maxSubdirs)
	}
	if *doApply && *asJSON {
		return errors.New("--apply cannot be combined with --json")
	}

	home := xdg.Home()
	configDir := *configDirFlag
	if configDir == "" {
		configDir = xdg.ConfigHome()
	}
	if configDir == "" {
		return errors.New("cannot determine the config directory; pass --config-dir")
	}
	configDir, err := filepath.Abs(configDir)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(configDir); err == nil {
		configDir = resolved
	}

	root, err := repo.Find(*dotfilesFlag, xdg.ConfigHome(), home)
	if err != nil {
		return err
	}
	if insideRepo(root.Path, configDir) {
		return fmt.Errorf("repository %s contains the config directory %s; refusing to scan", root.Path, configDir)
	}

	target := *targetFlag
	if target == "" {
		target = filepath.Dir(configDir)
	}
	if target, err = filepath.Abs(target); err != nil {
		return err
	}

	backupRoot := *backupFlag
	if backupRoot == "" {
		stateHome := xdg.StateHome()
		if stateHome == "" && *doApply {
			return errors.New("cannot determine $XDG_STATE_HOME for backups; pass --backup-dir")
		}
		backupRoot = filepath.Join(stateHome, "stowr", "backups")
	}

	scanOptions := scan.Options{
		ConfigDir:      configDir,
		Repo:           root,
		MinFiles:       *minFiles,
		MaxFiles:       *maxFiles,
		MaxSubdirs:     *maxSubdirs,
		Depth:          *depth,
		IncludeBackups: *includeBackups,
		IncludeFiles:   *includeFiles,
		Deny:           deny,
	}
	entries, err := scan.Scan(scanOptions)
	if err != nil {
		return err
	}
	if entries, err = selectEntries(entries, flags.Args(), configDir); err != nil {
		return err
	}

	reportOptions := report.Options{
		RepoPath:  root.Path,
		ConfigDir: configDir,
		Target:    target,
		Color:     !*noColor && isTerminal(stdout),
		Verbose:   *verbose,
	}
	if *asJSON {
		return report.WriteJSON(stdout, entries, reportOptions)
	}
	if err := report.Write(stdout, entries, reportOptions); err != nil {
		return err
	}
	if !*doApply {
		return nil
	}
	return applyAll(entries, apply.Options{
		Repo:       root,
		ConfigDir:  configDir,
		Target:     target,
		BackupRoot: backupRoot,
	}, *assumeYes, stdin, stdout)
}

// applyAll moves every candidate into the repository, backing it up first.
func applyAll(entries []scan.Entry, options apply.Options, assumeYes bool, stdin io.Reader, stdout io.Writer) error {
	stowBin, _ := exec.LookPath("stow")
	options.StowBin = stowBin
	options.Out = stdout
	if stowBin == "" {
		fmt.Fprintln(stdout, "note: stow is not installed, creating symlinks directly")
	}
	if !assumeYes && !isTerminal(stdin) {
		return errors.New("refusing to apply interactively without a terminal; pass --yes")
	}

	applied, skipped := 0, 0
	for _, entry := range entries {
		switch entry.Status {
		case scan.Candidate:
		case scan.PartiallyStowed:
			fmt.Fprintf(stdout, "%s: already partially stowed; run `stow -R -t %s %s`\n", entry.Name, options.Target, entry.Package)
			skipped++
			continue
		default:
			continue
		}
		plan, err := apply.Prepare(entry, options)
		if err != nil {
			fmt.Fprintf(stdout, "%s: %v\n", entry.Name, err)
			skipped++
			continue
		}
		fmt.Fprintf(stdout, "\n%s\n  backup: %s\n  move:   %s -> %s\n  link:   %s -> %s\n",
			entry.Name, plan.BackupPath(), entry.Path, plan.PackageDir, entry.Path, plan.LinkTarget)
		// Prepare reserves the backup directory so the printed path is exact;
		// drop it again when the entry ends up not being applied. Only ever
		// removes the still-empty reservation, never a real backup.
		discard := func() {
			if entries, err := os.ReadDir(plan.BackupDir); err == nil && len(entries) == 0 {
				_ = os.Remove(plan.BackupDir)
			}
		}
		if !assumeYes {
			ok, err := confirm(stdin, stdout)
			if err != nil {
				discard()
				return err
			}
			if !ok {
				discard()
				fmt.Fprintf(stdout, "  skipped\n")
				skipped++
				continue
			}
		}
		if err := apply.Apply(plan, options); err != nil {
			discard()
			return fmt.Errorf("%s: %w", entry.Name, err)
		}
		fmt.Fprintf(stdout, "  applied; restore with: %s\n", plan.RestoreCommand())
		applied++
	}
	fmt.Fprintf(stdout, "\napplied %d, skipped %d\n", applied, skipped)
	return nil
}

// selectEntries filters entries by name, reporting names that do not exist.
func selectEntries(entries []scan.Entry, names []string, configDir string) ([]scan.Entry, error) {
	if len(names) == 0 {
		return entries, nil
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	found := make(map[string]bool, len(names))
	var selected []scan.Entry
	for _, entry := range entries {
		if wanted[entry.Name] {
			selected = append(selected, entry)
			found[entry.Name] = true
		}
	}
	var missing []string
	for _, name := range names {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no such entry in %s: %s", configDir, strings.Join(missing, ", "))
	}
	return selected, nil
}

// insideRepo reports whether path lies inside root.
func insideRepo(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// errNoAnswer reports that the confirmation prompt was never answered, which
// means the process is unattended.
var errNoAnswer = errors.New("no answer on stdin; pass --yes to apply without confirmation")

func confirm(stdin io.Reader, stdout io.Writer) (bool, error) {
	fmt.Fprint(stdout, "apply? [y/N] ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return false, errNoAnswer
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func isTerminal(rw any) bool {
	file, ok := rw.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
