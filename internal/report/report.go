// Package report renders scan results for humans and machines.
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/MatyiFKBT/stowr/internal/scan"
)

// Options controls rendering.
type Options struct {
	RepoPath  string
	ConfigDir string
	Target    string // stow target directory, usually the config directory's parent
	Color     bool
	Verbose   bool
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
)

func (o Options) paint(code, text string) string {
	if !o.Color || text == "" {
		return text
	}
	return code + text + ansiReset
}

// Commands returns the shell commands that would stow entry.
func Commands(entry scan.Entry, o Options) []string {
	stow := fmt.Sprintf("stow -t %s %s", o.Target, entry.Package)
	switch entry.Status {
	case scan.Candidate:
		if entry.PackageExists {
			return []string{stow}
		}
		return []string{
			fmt.Sprintf("mkdir -p %s", filepath.Dir(entry.PackagePath)),
			fmt.Sprintf("mv %s %s", entry.Path, entry.PackagePath),
			stow,
		}
	case scan.PartiallyStowed, scan.AlreadyStowed:
		return []string{fmt.Sprintf("stow -R -t %s %s", o.Target, entry.Package)}
	default:
		return nil
	}
}

// Write renders entries as a human-readable report.
func Write(w io.Writer, entries []scan.Entry, o Options) error {
	out := bufio.NewWriter(w)
	defer out.Flush()

	fmt.Fprintf(out, "%s %s\n", o.paint(ansiDim, "repo:  "), o.RepoPath)
	fmt.Fprintf(out, "%s %s\n", o.paint(ansiDim, "config:"), o.ConfigDir)
	fmt.Fprintf(out, "%s %s\n", o.paint(ansiDim, "target:"), o.Target)

	sections := []struct {
		title  string
		code   string
		status scan.Status
	}{
		{"CANDIDATES", ansiGreen, scan.Candidate},
		{"PARTIALLY STOWED", ansiYellow, scan.PartiallyStowed},
		{"ALREADY STOWED", ansiCyan, scan.AlreadyStowed},
		{"LINKED ELSEWHERE", ansiRed, scan.LinkedElsewhere},
	}
	for _, section := range sections {
		rows := withStatus(entries, section.status)
		if len(rows) == 0 {
			continue
		}
		heading := fmt.Sprintf("%s (%d)", section.title, len(rows))
		fmt.Fprintf(out, "\n%s\n", o.paint(ansiBold+section.code, heading))
		for _, entry := range rows {
			o.writeEntry(out, entry)
		}
	}

	if o.Verbose {
		for _, section := range []struct {
			title  string
			status scan.Status
		}{
			{"NO CONFIG FILES", scan.NoConfigFiles},
			{"TOO MANY CONFIG FILES", scan.TooManyConfigFiles},
			{"TOO MANY SUBDIRECTORIES", scan.TooManySubdirs},
		} {
			rows := withStatus(entries, section.status)
			if len(rows) == 0 {
				continue
			}
			heading := fmt.Sprintf("%s (%d)", section.title, len(rows))
			fmt.Fprintf(out, "\n%s\n", o.paint(ansiBold+ansiDim, heading))
			for _, entry := range rows {
				fmt.Fprintf(out, "  %s\n", entry.Name)
			}
		}
	}

	summary := scan.Summarize(entries)
	fmt.Fprintf(out, "\n%d stow-able, %d partially stowed, %d already stowed, %d linked elsewhere, %d skipped\n",
		summary.Candidates, summary.PartiallyStowed, summary.AlreadyStowed,
		summary.LinkedElsewhere, summary.NoConfigFiles+summary.TooManyConfigFiles+summary.TooManySubdirs)
	return nil
}

func (o Options) writeEntry(w io.Writer, entry scan.Entry) {
	fmt.Fprintf(w, "  %-26s %s\n", entry.Name, o.paint(ansiDim, describeFiles(entry)))

	if entry.Status == scan.AlreadyStowed || entry.Status == scan.LinkedElsewhere {
		fmt.Fprintf(w, "  %-26s %s\n", "", o.paint(ansiDim, "-> "+entry.ResolvedTarget))
	}
	if o.Verbose && len(entry.ConfigFiles) > 0 {
		fmt.Fprintf(w, "  %-26s %s\n", "", o.paint(ansiDim, "config: "+strings.Join(entry.ConfigFiles, ", ")))
	}
	if o.Verbose && len(entry.LinkedFiles) > 0 {
		fmt.Fprintf(w, "  %-26s %s\n", "", o.paint(ansiDim, "linked: "+strings.Join(entry.LinkedFiles, ", ")))
	}
	if len(entry.Secrets) > 0 {
		note := "contains " + strings.Join(entry.Secrets, ", ") + "; stowing publishes it"
		fmt.Fprintf(w, "  %-26s %s\n", "", o.paint(ansiRed, "WARNING "+note))
	}
	if hint := entry.VerboseHint(); hint != "" && o.Verbose {
		fmt.Fprintf(w, "  %-26s %s\n", "", o.paint(ansiDim, hint))
	}
	for _, command := range Commands(entry, o) {
		fmt.Fprintf(w, "  %-26s %s\n", "", o.paint(ansiGreen, command))
	}
}

// describeFiles summarizes the file list of an entry. Linked entries report
// counts instead, because a half-stowed application directory can hold dozens
// of files.
func describeFiles(entry scan.Entry) string {
	switch entry.Status {
	case scan.PartiallyStowed:
		return fmt.Sprintf("%d linked into the repository, %d local config file(s)",
			len(entry.LinkedFiles), len(entry.ConfigFiles))
	case scan.AlreadyStowed, scan.LinkedElsewhere:
		if entry.Kind == scan.KindFile {
			return "file"
		}
		return "whole directory"
	default:
		return strings.Join(entry.ConfigFiles, ", ")
	}
}

type jsonEntry struct {
	Name           string   `json:"name"`
	Package        string   `json:"package"`
	Path           string   `json:"path"`
	Kind           string   `json:"kind"`
	Status         string   `json:"status"`
	ConfigFiles    []string `json:"configFiles"`
	Secrets        []string `json:"secrets,omitempty"`
	LinkedFiles    []string `json:"linkedFiles,omitempty"`
	ForeignLinks   int      `json:"foreignLinks,omitempty"`
	Subdirs        []string `json:"subdirs,omitempty"`
	ResolvedTarget string   `json:"resolvedTarget,omitempty"`
	PackagePath    string   `json:"packagePath"`
	PackageExists  bool     `json:"packageExists"`
	Commands       []string `json:"commands,omitempty"`
}

type jsonDocument struct {
	Repo    string       `json:"repo"`
	Config  string       `json:"config"`
	Target  string       `json:"target"`
	Summary scan.Summary `json:"summary"`
	Entries []jsonEntry  `json:"entries"`
}

// WriteJSON renders entries as a machine-readable document.
func WriteJSON(w io.Writer, entries []scan.Entry, o Options) error {
	document := jsonDocument{
		Repo:    o.RepoPath,
		Config:  o.ConfigDir,
		Target:  o.Target,
		Summary: scan.Summarize(entries),
		Entries: make([]jsonEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		document.Entries = append(document.Entries, jsonEntry{
			Name:           entry.Name,
			Package:        entry.Package,
			Path:           entry.Path,
			Kind:           entry.Kind.String(),
			Status:         entry.Status.String(),
			ConfigFiles:    entry.ConfigFiles,
			Secrets:        entry.Secrets,
			LinkedFiles:    entry.LinkedFiles,
			ForeignLinks:   entry.ForeignLinks,
			Subdirs:        entry.Subdirs,
			ResolvedTarget: entry.ResolvedTarget,
			PackagePath:    entry.PackagePath,
			PackageExists:  entry.PackageExists,
			Commands:       Commands(entry, o),
		})
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func withStatus(entries []scan.Entry, status scan.Status) []scan.Entry {
	var out []scan.Entry
	for _, entry := range entries {
		if entry.Status == status {
			out = append(out, entry)
		}
	}
	return out
}
