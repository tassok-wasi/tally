# tally

A fast, dependency-light CLI that counts lines of code grouped by file
extension. It walks a directory tree, skips noise (VCS dirs, `node_modules`,
binaries, `.gitignore`d paths), and reports a per-extension breakdown — with an
optional code / comment / blank split. It can also diff two trees, enforce CI
budgets, and emit machine-readable output.

## Build & install

```sh
go build -o tally .
./install.sh          # copies the binary to ~/.local/bin (linux only)
```

## Usage

```
tally [<command>] [flags]
```

With no command, `tally` behaves like `count` in the current directory. `~/`
is expanded in directory paths.

## Commands

| Command | Description |
|---------|-------------|
| *(default)* / `count [<root-dir>]` | Count lines/files by extension. |
| `diff <from> <to>` | Compare two directories and show per-key line deltas. |
| `check [<root-dir>]` | Enforce line/file budgets; exits non-zero when exceeded. |
| `init` | Write a default config file. |
| `languages` | List supported comment syntaxes. |

## Global flags

These flags apply to every scanning command (`count`, `diff`, `check`).

| Flag | Default | Description |
|------|---------|-------------|
| `-e, --extensions` | all files | Extensions to include, repeatable (`-e go -e py`). Use `*` for all files. |
| `-i, --ignore` | – | Additional directory/file **names** to ignore (repeatable). |
| `-x, --exclude` | – | Glob patterns to exclude, repeatable (e.g. `*.min.js`, `vendor/**`). |
| `-I, --include` | – | Glob patterns to include; only matching files are counted. |
| `--exclude-tests` | `false` | Skip test files/dirs (`test/`, `spec/`, `*_test.go`, ...). |
| `-v, --verbose` | `false` | Print per-file line counts as files are processed. |
| `-c, --count-files` | `false` | Only count files, not lines. |
| `--breakdown` | `false` | Show code / comment / blank columns per key. |
| `-d, --dir` | `.` | Directory to scan. Supports `~/`. |
| `-D, --max-depth` | `-1` | Maximum directory depth to descend (`-1` = unlimited). |
| `--no-ignore` | `false` | Disable the built-in default ignore list. |
| `--no-gitignore` | `false` | Do not read `.gitignore` / `.ignore` files. |
| `-H, --hidden` | `false` | Include hidden files and directories. |
| `--follow` | `false` | Follow symlinks (with cycle detection). |
| `--git` | `false` | Only count files tracked by git. |
| `--since` | – | Only files modified since a date/duration (`2024-01-01`, `24h`, `7d`). |
| `--min-lines` | `0` | Only count files with at least this many lines. |
| `--max-lines` | `0` | Only count files with at most this many lines (`0` = unlimited). |
| `--max-file-size` | `0` | Skip files larger than this many bytes (`0` = unlimited). |
| `--by-dir` | `false` | Group by directory instead of by extension. |
| `--largest` | `0` | Also list the N largest files by line count. |
| `--sort` | `name` | Sort order: `name`, `lines`, or `files`. |
| `--top` | `0` | Show only the top N keys (`0` = all). |
| `--format` | `table` | Output format: `table`, `json`, `csv`, `tsv`, `markdown`, `html`. |
| `--json` / `--csv` / `--markdown` | `false` | Shorthands for `--format=...`. |
| `--summary-only` | `false` | Print only the totals, without the breakdown. |
| `--no-color` | `false` | Disable colored output (also honors `NO_COLOR`). |
| `--workers` | `0` | Concurrent workers (`0` = number of CPUs). |
| `--version` | – | Print version and exit. |
| `-h, --help` | – | Show help. |

### `check` flags

| Flag | Description |
|------|-------------|
| `--max-total-lines` | Fail if total lines exceed this (`0` = disabled). |
| `--max-total-files` | Fail if total files exceed this (`0` = disabled). |
| `--budget` | Per-extension line budget, e.g. `--budget '.go=1000'` (repeatable). |

### `init` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Overwrite an existing config file. |
| `--path` | `tally.json` | Config file path to write. |

## Features

- **Extension filtering** — count only the extensions you care about, or all
  files by default.
- **Binary detection** — files containing a NUL byte in the first 8 KB are
  skipped, so `-e '*'` doesn't count compiled binaries as text.
- **`.gitignore` / `.ignore` support** — read at every directory level (nested),
  supporting negation (`!`), anchoring, `**`, `*`, `?`, and character classes.
- **Glob include/exclude** — `-x '*.min.js'`, `-I 'src/**'`. Patterns match
  anywhere unless prefixed with `/` (anchored to the scan root).
- **Code / comment / blank breakdown** — `--breakdown` classifies each line
  using per-language comment syntax.
- **Diff** — `diff` compares two trees and reports per-key line deltas.
- **CI budgets** — `check` fails the build when totals or per-extension budgets
  are exceeded.
- **Machine-readable output** — `--format json|csv|tsv|markdown|html`.
- **Sorting, truncation, largest files** — `--sort`, `--top`, `--largest`.
- **Filtering** — `--exclude-tests`, `--min-lines`, `--max-lines`,
  `--max-file-size`, `--since`, `--git`, `--by-dir`.
- **Depth, hidden, symlinks** — `--max-depth`, `--hidden`, `--follow`.
- **Concurrency** — files are analyzed in parallel (`--workers`).
- **Config file** — defaults loaded from JSON (see below).

### Default ignore list

Unless `--no-ignore` is passed, these directory/file names are skipped:

```
venv  .venv  __pycache__  .git  .svn  node_modules  bower_components
.tox  .mypy_cache  .pytest_cache  .idea  .vscode  .vs  .eggs
build  dist  .cache  .github
```

### Comment syntaxes

`--breakdown` recognizes line and block comments for common languages,
including: Go, C/C++/C#, Java, JavaScript/TypeScript, PHP, Swift, Kotlin, Scala,
Rust, Dart, Groovy, Proto, CSS/SCSS/LESS, SQL, shell, Python, Ruby, Perl, R,
YAML/TOML/INI, Lua, Vim script, Lisp/Clojure, Haskell, OCaml, Elixir, Erlang,
TeX, HTML/XML, Vue/Svelte, Fortran, Assembly, PowerShell, batch, Make/CMake, and
Terraform/Nix. Run `tally languages` for the full list. Extensions without a
known syntax treat every non-blank line as code.

## Config file

Defaults can be supplied via JSON. Keys are flag names (snake_case or
camelCase). Files are loaded from:

1. `tally.json` in the current directory
2. `~/.config/tally/config.json`

Command-line flags override config values. Generate a starter file with
`tally init`.

```json
{
  "extensions": ["go", "py"],
  "sort": "lines",
  "top": 10,
  "exclude": ["*.min.js", "vendor/**"]
}
```

## Examples

```sh
# All files in the current directory
tally

# Only Go and Python, sorted by line count, top 10
tally -e go -e py --sort lines --top 10

# Code/comment/blank breakdown, plus the 5 largest files
tally --breakdown --largest 5

# Count files only, excluding generated code
tally -c -x '*.pb.go' -x 'vendor/**'

# Compare two directories
tally diff ./before ./after -e go

# Fail CI if the Go codebase grows past 50k lines
tally check --budget '.go=50000'

# Machine-readable output
tally --json | jq '.rows'
```

## Output

Default table:

```
-----------------------
Extension  Files  Lines
-----------------------
.go            2      9
.md            1      3
-----------------------
TOTAL          3     12
```

With `--breakdown`:

```
----------------------------------------------------
Extension  Files  Lines  Code  Comment  Blank
----------------------------------------------------
.go            2      9     5        3      1
.md            1      3     2        0      1
----------------------------------------------------
TOTAL          3     12     7        3      2
```

With `diff`:

```
----------------------
Key    From  To  Delta
----------------------
.go       1   4     +3
TOTAL     1   4     +3
----------------------
```

## Known limitations

- **Comment detection is heuristic.** It does not parse string literals, so a
  comment token inside a multi-line string can be misclassified.
- **Unknown extensions** have no comment syntax, so all non-blank lines count as
  code (e.g. `.md`).
- **gitignore re-inclusion.** A file inside an already-ignored directory cannot
  be re-included with `!` — ignored directories are skipped during the walk.
- **`--top` and `TOTAL`.** With `--top N`, the `TOTAL` row still reflects the
  full scan, not just the displayed rows.
- **Binary heuristic.** Detection is based on a NUL byte in the first 8 KB;
  unusual text files containing NUL bytes would be skipped, and binaries without
  an early NUL byte would be counted.
- **`--git`** requires `git` on `PATH` and a git repository at the scan root.
- **`diff`** compares two directories; comparing git refs is not supported.
