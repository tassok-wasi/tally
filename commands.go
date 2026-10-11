package main

import (
	"fmt"
	"maps"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ScanFlags are the options shared by the scanning commands. They live on the
// root command (and are therefore global); subcommands reference them.
type ScanFlags struct {
	Extensions   []string `short:"e" help:"File extensions to include (repeatable, e.g. -e go -e py). Default: all files."`
	Ignore       []string `short:"i" help:"Additional directory/file names to ignore."`
	Exclude      []string `short:"x" help:"Glob patterns to exclude (e.g. '*.min.js', 'vendor/**')."`
	Include      []string `short:"I" help:"Glob patterns to include (only matching files are counted)."`
	ExcludeTests bool     `help:"Skip test files and directories (test/, spec/, *_test.go, ...)."`
	Verbose      bool     `short:"v" help:"Verbose mode: show per-file line counts."`
	CountFiles   bool     `short:"c" help:"Only count files, not lines."`
	Breakdown    bool     `help:"Show code/comment/blank breakdown per key."`
	Dir          string   `short:"d" type:"path" help:"Directory to scan (default: current directory). Supports ~/."`
	MaxDepth     int      `short:"D" default:"-1" help:"Maximum directory depth to descend (-1 = unlimited)."`
	NoIgnore     bool     `help:"Disable the built-in default ignore list."`
	NoGitignore  bool     `help:"Do not read .gitignore/.ignore files."`
	Hidden       bool     `short:"H" help:"Include hidden files and directories."`
	Follow       bool     `help:"Follow symlinks."`
	Git          bool     `help:"Only count files tracked by git."`
	Since        string   `help:"Only count files modified since this date or duration (e.g. 2024-01-01, 24h, 7d)."`
	MinLines     int      `help:"Only count files with at least this many lines."`
	MaxLines     int      `help:"Only count files with at most this many lines (0 = unlimited)."`
	MaxFileSize  int64    `help:"Skip files larger than this many bytes (0 = unlimited)."`
	ByDir        bool     `help:"Group by directory instead of by extension."`
	Largest      int      `help:"Also list the N largest files by line count."`
	Sort         string   `default:"name" enum:"name,lines,files" help:"Sort order for the breakdown."`
	Top          int      `default:"0" help:"Show only the top N keys (0 = all)."`
	Format       string   `default:"table" enum:"table,json,csv,tsv,markdown,html" help:"Output format."`
	JSON         bool     `help:"Shorthand for --format=json."`
	CSV          bool     `help:"Shorthand for --format=csv."`
	Markdown     bool     `help:"Shorthand for --format=markdown."`
	SummaryOnly  bool     `help:"Print only the totals, without the breakdown."`
	NoColor      bool     `help:"Disable colored output."`
	Workers      int      `default:"0" help:"Number of concurrent workers (0 = number of CPUs)."`
}

func (f *ScanFlags) workers() int {
	if f.Workers > 0 {
		return f.Workers
	}
	return runtime.NumCPU()
}

func (f *ScanFlags) outputFormat() string {
	switch {
	case f.JSON:
		return "json"
	case f.CSV:
		return "csv"
	case f.Markdown:
		return "markdown"
	default:
		return f.Format
	}
}

func (f *ScanFlags) buildOptions(root string) (Options, error) {
	extensions := make(map[string]bool)
	allExts := len(f.Extensions) == 0
	for _, ext := range f.Extensions {
		ext = strings.ToLower(ext)
		if ext == "*" {
			allExts = true
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extensions[ext] = true
	}

	ignoreNames := make(map[string]bool)
	if !f.NoIgnore {
		maps.Copy(ignoreNames, DefaultIgnore)
	}
	for _, ig := range f.Ignore {
		ignoreNames[ig] = true
	}

	var since *time.Time
	if f.Since != "" {
		t, err := parseSince(f.Since)
		if err != nil {
			return Options{}, err
		}
		since = &t
	}

	return Options{
		Root:         root,
		Extensions:   extensions,
		AllExts:      allExts,
		IgnoreNames:  ignoreNames,
		Excludes:     compileExcludes(f.Exclude),
		Includes:     compileExcludes(f.Include),
		MaxDepth:     f.MaxDepth,
		Hidden:       f.Hidden,
		NoGitignore:  f.NoGitignore,
		Follow:       f.Follow,
		Git:          f.Git,
		Since:        since,
		MaxFileSize:  f.MaxFileSize,
		ExcludeTests: f.ExcludeTests,
	}, nil
}

func scanDir(f ScanFlags, root string) (map[string]*Stats, Stats, []FileEntry, error) {
	opts, err := f.buildOptions(root)
	if err != nil {
		return nil, Stats{}, nil, err
	}
	files, err := collectFiles(opts)
	if err != nil {
		return nil, Stats{}, nil, err
	}
	stats, total, entries := run(root, files, RunOptions{
		CountFiles: f.CountFiles,
		Verbose:    f.Verbose,
		Workers:    f.workers(),
		ByDir:      f.ByDir,
		MinLines:   f.MinLines,
		MaxLines:   f.MaxLines,
	})
	return stats, total, entries, nil
}

func sortKeys(keys []string, stats map[string]*Stats, order string) {
	switch order {
	case "lines":
		sort.Slice(keys, func(i, j int) bool {
			if stats[keys[i]].Lines != stats[keys[j]].Lines {
				return stats[keys[i]].Lines > stats[keys[j]].Lines
			}
			return keys[i] < keys[j]
		})
	case "files":
		sort.Slice(keys, func(i, j int) bool {
			if stats[keys[i]].Files != stats[keys[j]].Files {
				return stats[keys[i]].Files > stats[keys[j]].Files
			}
			return keys[i] < keys[j]
		})
	default:
		sort.Strings(keys)
	}
}

func buildRows(stats map[string]*Stats, order string, top int) []Row {
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sortKeys(keys, stats, order)
	if top > 0 && top < len(keys) {
		keys = keys[:top]
	}
	rows := make([]Row, len(keys))
	for i, k := range keys {
		rows[i] = Row{Key: k, Stats: *stats[k]}
	}
	return rows
}

func topFiles(entries []FileEntry, n int) []FileEntry {
	sorted := append([]FileEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Stats.Lines != sorted[j].Stats.Lines {
			return sorted[i].Stats.Lines > sorted[j].Stats.Lines
		}
		return sorted[i].Rel < sorted[j].Rel
	})
	if n < len(sorted) {
		sorted = sorted[:n]
	}
	return sorted
}

func keyLabel(byDir bool) string {
	if byDir {
		return "Directory"
	}
	return "Extension"
}

func runCount(f ScanFlags, root string) error {
	stats, total, entries, err := scanDir(f, root)
	if err != nil {
		return err
	}
	var largest []FileEntry
	if f.Largest > 0 {
		largest = topFiles(entries, f.Largest)
	}
	render(f.outputFormat(), buildRows(stats, f.Sort, f.Top), total, renderOptions{
		KeyLabel:    keyLabel(f.ByDir),
		CountFiles:  f.CountFiles,
		Breakdown:   f.Breakdown,
		SummaryOnly: f.SummaryOnly,
		Color:       colorEnabled(f.NoColor),
		Largest:     largest,
	})
	return nil
}

// CountCmd counts lines/files by extension, with an optional positional dir.
type CountCmd struct {
	RootDir string `arg:"" optional:"" type:"path" help:"Directory to scan (positional, default: current directory). Supports ~/."`

	scan *ScanFlags
}

func (c *CountCmd) Run() error {
	root := c.scan.Dir
	if root == "" {
		root = c.RootDir
	}
	if root == "" {
		root = "."
	}
	return runCount(*c.scan, root)
}

// DiffCmd compares two directories and reports per-key deltas.
type DiffCmd struct {
	From string `arg:"" type:"path" help:"Source directory."`
	To   string `arg:"" type:"path" help:"Target directory."`

	scan *ScanFlags
}

func (c *DiffCmd) Run() error {
	fromStats, fromTotal, _, err := scanDir(*c.scan, c.From)
	if err != nil {
		return err
	}
	toStats, toTotal, _, err := scanDir(*c.scan, c.To)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	var keys []string
	for k := range fromStats {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range toStats {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		di := delta(toStats[keys[i]], fromStats[keys[i]])
		dj := delta(toStats[keys[j]], fromStats[keys[j]])
		if abs(di) != abs(dj) {
			return abs(di) > abs(dj)
		}
		return keys[i] < keys[j]
	})
	if c.scan.Top > 0 && c.scan.Top < len(keys) {
		keys = keys[:c.scan.Top]
	}

	renderDiff(keys, fromStats, toStats, fromTotal, toTotal, colorEnabled(c.scan.NoColor))
	return nil
}

func delta(to, from *Stats) int {
	var t, f int
	if to != nil {
		t = to.Lines
	}
	if from != nil {
		f = from.Lines
	}
	return t - f
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func renderDiff(keys []string, fromStats, toStats map[string]*Stats, fromTotal, toTotal Stats, color bool) {
	rows := make([][]string, 0, len(keys)+1)
	for _, k := range keys {
		var f, t int
		if fromStats[k] != nil {
			f = fromStats[k].Lines
		}
		if toStats[k] != nil {
			t = toStats[k].Lines
		}
		rows = append(rows, []string{k, itoa(f), itoa(t), signed(t - f)})
	}
	rows = append(rows, []string{"TOTAL", itoa(fromTotal.Lines), itoa(toTotal.Lines), signed(toTotal.Lines - fromTotal.Lines)})

	head := []string{"Key", "From", "To", "Delta"}
	widths := make([]int, len(head))
	for i, h := range head {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	sep := strings.Repeat("-", totalWidth(widths))
	fmt.Println()
	fmt.Println(sep)
	fmt.Println(colorize(formatRow(head, widths), "1", color))
	fmt.Println(sep)
	for i, row := range rows {
		line := formatRow(row, widths)
		if i == len(rows)-1 {
			line = colorize(line, "1", color)
		} else {
			line = colorDelta(line, row[3], color)
		}
		fmt.Println(line)
	}
	fmt.Println(sep)
}

func signed(n int) string {
	if n > 0 {
		return "+" + itoa(n)
	}
	return itoa(n)
}

func colorDelta(line, delta string, color bool) string {
	if !color || delta == "" {
		return line
	}
	switch delta[0] {
	case '+':
		return colorize(line, "32", color)
	case '-':
		return colorize(line, "31", color)
	}
	return line
}

// CheckCmd enforces line/file budgets and exits non-zero when exceeded.
type CheckCmd struct {
	RootDir       string   `arg:"" optional:"" type:"path" help:"Directory to scan (positional, default: current directory)."`
	MaxTotalLines int      `help:"Fail if total lines exceed this (0 = disabled)."`
	MaxTotalFiles int      `help:"Fail if total files exceed this (0 = disabled)."`
	Budget        []string `help:"Per-extension line budget, e.g. '.go=1000' (repeatable)."`

	scan *ScanFlags
}

func (c *CheckCmd) Run() error {
	root := c.scan.Dir
	if root == "" {
		root = c.RootDir
	}
	if root == "" {
		root = "."
	}
	stats, total, _, err := scanDir(*c.scan, root)
	if err != nil {
		return err
	}

	failed := false
	if c.MaxTotalLines > 0 {
		if total.Lines > c.MaxTotalLines {
			fmt.Printf("FAIL  total lines %d > budget %d\n", total.Lines, c.MaxTotalLines)
			failed = true
		} else {
			fmt.Printf("ok    total lines %d <= budget %d\n", total.Lines, c.MaxTotalLines)
		}
	}
	if c.MaxTotalFiles > 0 {
		if total.Files > c.MaxTotalFiles {
			fmt.Printf("FAIL  total files %d > budget %d\n", total.Files, c.MaxTotalFiles)
			failed = true
		} else {
			fmt.Printf("ok    total files %d <= budget %d\n", total.Files, c.MaxTotalFiles)
		}
	}
	for _, b := range c.Budget {
		ext, limit, err := parseBudget(b)
		if err != nil {
			return err
		}
		got := 0
		if s := stats[ext]; s != nil {
			got = s.Lines
		}
		if got > limit {
			fmt.Printf("FAIL  %s lines %d > budget %d\n", ext, got, limit)
			failed = true
		} else {
			fmt.Printf("ok    %s lines %d <= budget %d\n", ext, got, limit)
		}
	}

	if failed {
		return fmt.Errorf("budget exceeded")
	}
	return nil
}

func parseBudget(s string) (string, int, error) {
	i := strings.LastIndex(s, "=")
	if i < 0 {
		return "", 0, fmt.Errorf("invalid budget %q (expected ext=lines)", s)
	}
	ext := strings.ToLower(strings.TrimSpace(s[:i]))
	if ext != "*" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	n, err := strconv.Atoi(strings.TrimSpace(s[i+1:]))
	if err != nil {
		return "", 0, fmt.Errorf("invalid budget %q: %w", s, err)
	}
	return ext, n, nil
}

// InitCmd writes a default config file.
type InitCmd struct {
	Force bool   `help:"Overwrite an existing config file."`
	Path  string `type:"path" default:"tally.json" help:"Config file path to write."`
}

const defaultConfig = `{
  "extensions": ["go", "py", "js", "ts"],
  "exclude": ["*.min.js", "vendor/**"],
  "sort": "lines",
  "top": 20
}
`

func (c *InitCmd) Run() error {
	if _, err := os.Stat(c.Path); err == nil && !c.Force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", c.Path)
	}
	if err := os.WriteFile(c.Path, []byte(defaultConfig), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", c.Path)
	return nil
}

// LanguagesCmd lists the comment syntaxes known to the tool.
type LanguagesCmd struct{}

func (c *LanguagesCmd) Run() error {
	exts := make([]string, 0, len(commentSyntaxes))
	for ext := range commentSyntaxes {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	for _, ext := range exts {
		syn := commentSyntaxes[ext]
		var parts []string
		if len(syn.line) > 0 {
			parts = append(parts, "line: "+strings.Join(syn.line, " "))
		}
		for _, b := range syn.block {
			parts = append(parts, "block: "+b[0]+" "+b[1])
		}
		fmt.Printf("%-12s %s\n", ext, strings.Join(parts, ", "))
	}
	return nil
}
