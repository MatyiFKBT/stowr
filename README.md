# stowr

[![CI](https://github.com/MatyiFKBT/stowr/actions/workflows/ci.yml/badge.svg)](https://github.com/MatyiFKBT/stowr/actions/workflows/ci.yml)

Recommends directories in `$XDG_CONFIG_HOME` that are ready to be managed with
[GNU stow](https://www.gnu.org/software/stow/): small app config directories
holding one or two config files that are not linked into your dotfiles
repository yet.

```
$ stowr
repo:   /home/you/dotfiles
config: /home/you/.config
target: /home/you

CANDIDATES (4)
  alacritty                  alacritty.toml
                             stow -t /home/you alacritty
  cliamp                     config.toml
                             mkdir -p /home/you/dotfiles/cliamp/.config
                             mv /home/you/.config/cliamp /home/you/dotfiles/cliamp/.config/cliamp
                             stow -t /home/you cliamp
  gh                         config.yml, hosts.yml
                             stow -t /home/you gh
  immich                     auth.yml
                             WARNING contains auth.yml; stowing publishes it

PARTIALLY STOWED (1)
  hypr                       1 linked file(s)

ALREADY STOWED (1)
  yazi
```

## Install

With Go 1.22 or newer:

```sh
go install github.com/MatyiFKBT/stowr@latest
```

That writes `stowr` to `$GOBIN` (or `$GOPATH/bin`) and reports the module
version in `stowr --version`. Note that `go get` no longer installs binaries —
`go install pkg@version` is the supported form since Go 1.17 — and that Go
module paths are case sensitive, so the capitalised `MatyiFKBT` is required
even though GitHub itself would resolve it either way.

Prebuilt binaries for `linux`, `darwin` and `windows` are attached to every
[release](https://github.com/MatyiFKBT/stowr/releases). From a checkout:

```sh
go build -o ~/.local/bin/stowr .
```

## Usage

```
stowr [flags] [entry ...]
```

Passing entry names limits the report (and `--apply`) to those entries; unknown
names are reported as errors.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--config-dir` | `$XDG_CONFIG_HOME`, else `~/.config` | directory to scan |
| `--dotfiles` | see below | stow repository root |
| `--target` | parent of the config directory | argument passed to `stow -t` |
| `--backup-dir` | `$XDG_STATE_HOME/stowr/backups` | where `--apply` stores its backups |
| `--min-files` | `1` | fewest config files a candidate may hold |
| `--max-files` | `2` | most config files a candidate may hold |
| `--max-subdirs` | `3` | most immediate subdirectories a candidate may hold (`-1` disables the check) |
| `--depth` | `1` | levels inspected inside each entry; `-1` recurses fully |
| `--include-backups` | off | keep `.bak`, `.backup`, `.omarchy-*`, `.default` style entries |
| `--include-files` | off | also consider plain files in the config directory |
| `--deny` | — | extra generated file names to ignore (repeatable, comma separated) |
| `--json` | off | machine-readable output |
| `--verbose` | off | list skipped entries |
| `--no-color` | off | disable ANSI colour |
| `--apply` | off | move candidates into the repository and link them |
| `--yes` | off | skip the `--apply` confirmation prompt |

`--apply` refuses to run unattended: without `--yes` it needs a confirmation
from a terminal, and an unanswered prompt (piped or closed stdin) is an error,
not a silent skip.

The repository is located via `--dotfiles`, then `$STOWR_DOTFILES`, then
`$XDG_CONFIG_HOME/dotfiles`, `~/dotfiles` and `~/.dotfiles`.

## What counts as a config file

Extensions `toml`, `yaml`, `yml`, `ini`, `conf`, `cfg`, `json`, `jsonc`, `lua`,
`rc`, `env`, plus an extensionless file named `config`.

Not counted:

- directories holding more than `--max-subdirs` immediate subdirectories
  (default 3): a cache or data tree with one stray `data.json` — `Bitwarden`,
  `Signal`, browser profiles — is not a config directory
- backups and templates — `.bak`, `.backup`, `.orig`, `.old`, `.tmp`, `.swp`,
  `~`, `omarchy-upgrade`, `.omarchy-`, `.disabled`, `.example`, `.sample`,
  `.default`
- runtime state that applications rewrite — `metrics.json`, `state.json`,
  `telemetry.json`, `ephemeral.json`, `languagepacks.json`, `*.receipt.json`,
  `*-state.json`, `.last*`

Files that look like credentials (`auth.yml`, `credentials*`, `token*`,
`secrets.*`, `.netrc`, `.env`) still count, but are flagged with a warning: the
repository is usually a git checkout, so stowing them publishes secrets.

## Applying

`--apply` never moves anything without a backup:

1. the whole entry is copied to
   `$XDG_STATE_HOME/stowr/backups/<timestamp>-<name>/<name>/`
2. the entry is moved to `<repo>/<name>/.config/<name>`
3. the entry is linked back, using `stow -t <target> <name>` when stow is
   installed and a relative symlink otherwise

Every applied entry prints its restore command. A failure after the move leaves
both the payload and the backup on disk and says so.

The backup is a plain recursive copy: file modes and symlinks are preserved,
sockets and devices are skipped. Moving across filesystems fails with a clear
error, since `stow` itself cannot span devices either.

## Releases

Every push to `main` (including merges) cuts a release automatically:

- the version is the next patch after the highest stable `vX.Y.Z` tag
  (`v0.1.0` when there is none), so merging after `v0.1.1` releases `v0.1.2`
- put `[skip release]` in the commit *subject* to publish nothing, for example
  for a docs-only merge (a mention in the body does not count)
- pushing a `vX.Y.Z` tag releases that exact version instead
- `workflow_dispatch` takes an explicit version

Each release builds and tests five targets (`linux`/`darwin` × `amd64`/`arm64`,
`windows/amd64`) with the version stamped in via
`-ldflags "-X main.version=..."`, and attaches the binaries to a GitHub Release
with generated notes. An in-flight release is never cancelled, and releases run
one at a time so two merges cannot pick the same version.

Not done: bumping minor or major automatically from commit messages
(conventional-commit driven versioning) — a patch bump is deterministic and
predictable.

## Testing

```sh
go test ./...
```
