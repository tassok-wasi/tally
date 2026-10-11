package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stats holds per-key (extension or directory) or total counts.
type Stats struct {
	Files   int `json:"files"`
	Lines   int `json:"lines"`
	Code    int `json:"code"`
	Comment int `json:"comment"`
	Blank   int `json:"blank"`
}

// FileStats holds the analysis of a single file.
type FileStats struct {
	Lines   int
	Code    int
	Comment int
	Blank   int
	Binary  bool
}

type commentSyntax struct {
	line  []string
	block [][2]string
}

// commentSyntaxes maps a lower-case file extension to its comment tokens.
var commentSyntaxes = map[string]commentSyntax{
	".go":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".c":      {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".h":      {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".cc":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".cpp":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".cxx":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".hpp":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".hxx":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".java":   {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".js":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".jsx":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".ts":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".tsx":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".mjs":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".cjs":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".cs":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".php":    {line: []string{"//", "#"}, block: [][2]string{{"/*", "*/"}}},
	".swift":  {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".kt":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".kts":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".scala":  {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".rs":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".dart":   {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".groovy": {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".gradle": {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".proto":  {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".zig":    {line: []string{"//"}},
	".v":      {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".css":    {block: [][2]string{{"/*", "*/"}}},
	".scss":   {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".less":   {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	".sql":    {line: []string{"--"}, block: [][2]string{{"/*", "*/"}}},
	".sh":     {line: []string{"#"}},
	".bash":   {line: []string{"#"}},
	".zsh":    {line: []string{"#"}},
	".fish":   {line: []string{"#"}},
	".py":     {line: []string{"#"}},
	".pyi":    {line: []string{"#"}},
	".rb":     {line: []string{"#"}},
	".pl":     {line: []string{"#"}},
	".pm":     {line: []string{"#"}},
	".r":      {line: []string{"#"}},
	".yaml":   {line: []string{"#"}},
	".yml":    {line: []string{"#"}},
	".toml":   {line: []string{"#"}},
	".ini":    {line: []string{";", "#"}},
	".cfg":    {line: []string{";", "#"}},
	".conf":   {line: []string{"#"}},
	".lua":    {line: []string{"--"}},
	".vim":    {line: []string{"\""}},
	".el":     {line: []string{";"}},
	".lisp":   {line: []string{";"}},
	".clj":    {line: []string{";"}},
	".hs":     {line: []string{"--"}, block: [][2]string{{"{-", "-}"}}},
	".ml":     {block: [][2]string{{"(*", "*)"}}},
	".ex":     {line: []string{"#"}},
	".exs":    {line: []string{"#"}},
	".erl":    {line: []string{"%"}},
	".hrl":    {line: []string{"%"}},
	".tex":    {line: []string{"%"}},
	".html":   {block: [][2]string{{"<!--", "-->"}}},
	".htm":    {block: [][2]string{{"<!--", "-->"}}},
	".xml":    {block: [][2]string{{"<!--", "-->"}}},
	".vue":    {line: []string{"//"}, block: [][2]string{{"/*", "*/"}, {"<!--", "-->"}}},
	".svelte": {line: []string{"//"}, block: [][2]string{{"/*", "*/"}, {"<!--", "-->"}}},
	".f":      {line: []string{"!"}},
	".f90":    {line: []string{"!"}},
	".for":    {line: []string{"!"}},
	".asm":    {line: []string{";"}},
	".ps1":    {line: []string{"#"}},
	".bat":    {line: []string{"::"}},
	".cmd":    {line: []string{"::"}},
	".mk":     {line: []string{"#"}},
	".make":   {line: []string{"#"}},
	".cmake":  {line: []string{"#"}},
	".tf":     {line: []string{"#"}, block: [][2]string{{"/*", "*/"}}},
	".tfvars": {line: []string{"#"}, block: [][2]string{{"/*", "*/"}}},
	".nix":    {line: []string{"#"}, block: [][2]string{{"/*", "*/"}}},
}

func syntaxFor(ext string) commentSyntax {
	return commentSyntaxes[ext]
}

// Options controls which files are collected.
type Options struct {
	Root         string
	Extensions   map[string]bool
	AllExts      bool
	IgnoreNames  map[string]bool
	Excludes     []globPattern
	Includes     []globPattern
	MaxDepth     int
	Hidden       bool
	NoGitignore  bool
	Follow       bool
	Git          bool
	Since        *time.Time
	MaxFileSize  int64
	ExcludeTests bool
}

type ignoreFrame struct {
	base    string
	matcher *ignoreMatcher
}

// collectFiles walks the tree and returns the sorted list of files to analyze.
func collectFiles(opts Options) ([]string, error) {
	var gitSet map[string]bool
	if opts.Git {
		set, err := gitTrackedFiles(opts.Root)
		if err != nil {
			return nil, fmt.Errorf("git: %w", err)
		}
		gitSet = set
	}

	var files []string
	visited := map[string]bool{}
	if abs, err := filepath.Abs(opts.Root); err == nil {
		visited[abs] = true
	}
	walkDir(opts.Root, opts, nil, &files, visited, gitSet)
	sort.Strings(files)
	return files, nil
}

func walkDir(dir string, opts Options, stack []ignoreFrame, files *[]string, visited, gitSet map[string]bool) {
	if !opts.NoGitignore {
		if m := loadGitignore(dir); m != nil {
			stack = append(stack, ignoreFrame{base: dir, matcher: m})
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		rel, err := filepath.Rel(opts.Root, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)

		if opts.MaxDepth >= 0 && strings.Count(rel, "/")+1 > opts.MaxDepth {
			continue
		}

		name := e.Name()
		if !opts.Hidden && strings.HasPrefix(name, ".") {
			continue
		}
		if opts.IgnoreNames[name] {
			continue
		}

		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			if !opts.Follow {
				continue
			}
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				continue
			}
			ti, err := os.Stat(target)
			if err != nil {
				continue
			}
			isDir = ti.IsDir()
			if isDir {
				abs, _ := filepath.Abs(target)
				if visited[abs] {
					continue
				}
				visited[abs] = true
			}
		}

		if ignoredByStack(stack, path, isDir) {
			continue
		}
		if matchAny(opts.Excludes, rel) {
			continue
		}
		if opts.ExcludeTests && isTestPath(rel, name) {
			continue
		}

		if isDir {
			walkDir(path, opts, stack, files, visited, gitSet)
			continue
		}

		if len(opts.Includes) > 0 && !matchAny(opts.Includes, rel) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !opts.AllExts && !opts.Extensions[ext] {
			continue
		}
		if gitSet != nil && !gitSet[rel] {
			continue
		}
		if opts.Since != nil || opts.MaxFileSize > 0 {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if opts.Since != nil && info.ModTime().Before(*opts.Since) {
				continue
			}
			if opts.MaxFileSize > 0 && info.Size() > opts.MaxFileSize {
				continue
			}
		}
		*files = append(*files, path)
	}
}

func ignoredByStack(stack []ignoreFrame, path string, isDir bool) bool {
	for _, f := range stack {
		r, err := filepath.Rel(f.base, path)
		if err != nil {
			continue
		}
		if f.matcher.Match(r, isDir) {
			return true
		}
	}
	return false
}

func matchAny(globs []globPattern, rel string) bool {
	for _, g := range globs {
		if g.match(rel) {
			return true
		}
	}
	return false
}

func isTestPath(rel, name string) bool {
	for _, seg := range strings.Split(rel, "/") {
		switch seg {
		case "test", "tests", "__tests__", "spec", "specs":
			return true
		}
	}
	switch {
	case strings.HasSuffix(name, "_test.go"),
		strings.HasSuffix(name, ".test.js"),
		strings.HasSuffix(name, ".test.ts"),
		strings.HasSuffix(name, ".test.jsx"),
		strings.HasSuffix(name, ".test.tsx"),
		strings.HasSuffix(name, ".spec.js"),
		strings.HasSuffix(name, ".spec.ts"),
		strings.HasPrefix(name, "test_"),
		strings.HasSuffix(name, "_test.py"):
		return true
	}
	return false
}

func gitTrackedFiles(root string) (map[string]bool, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			set[filepath.ToSlash(p)] = true
		}
	}
	return set, nil
}

type fileResult struct {
	rel   string
	ext   string
	stats FileStats
	err   error
}

// FileEntry is a single analyzed file, used for the largest-files report.
type FileEntry struct {
	Rel   string
	Key   string
	Stats FileStats
}

// RunOptions controls analysis and aggregation.
type RunOptions struct {
	CountFiles bool
	Verbose    bool
	Workers    int
	ByDir      bool
	MinLines   int
	MaxLines   int
}

// run analyzes files concurrently and aggregates the results.
func run(root string, files []string, opts RunOptions) (map[string]*Stats, Stats, []FileEntry) {
	if opts.Workers < 1 {
		opts.Workers = 1
	}

	jobs := make(chan string)
	results := make(chan fileResult)
	var wg sync.WaitGroup
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				ext := strings.ToLower(filepath.Ext(p))
				rel, _ := filepath.Rel(root, p)
				rel = filepath.ToSlash(rel)
				var st FileStats
				var err error
				if !opts.CountFiles {
					st, err = analyzeFile(p, syntaxFor(ext))
				}
				results <- fileResult{rel: rel, ext: ext, stats: st, err: err}
			}
		}()
	}
	go func() {
		for _, p := range files {
			jobs <- p
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var all []fileResult
	for r := range results {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].rel < all[j].rel })

	stats := make(map[string]*Stats)
	var total Stats
	var entries []FileEntry
	for _, r := range all {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", r.rel, r.err)
			continue
		}
		if r.stats.Binary {
			if opts.Verbose {
				fmt.Printf("%-8s %s (binary, skipped)\n", r.ext, r.rel)
			}
			continue
		}
		if !opts.CountFiles {
			if opts.MinLines > 0 && r.stats.Lines < opts.MinLines {
				continue
			}
			if opts.MaxLines > 0 && r.stats.Lines > opts.MaxLines {
				continue
			}
		}

		key := r.ext
		if opts.ByDir {
			key = filepath.ToSlash(filepath.Dir(r.rel))
			if key == "." {
				key = "./"
			}
		}

		s := stats[key]
		if s == nil {
			s = &Stats{}
			stats[key] = s
		}
		s.Files++
		s.Lines += r.stats.Lines
		s.Code += r.stats.Code
		s.Comment += r.stats.Comment
		s.Blank += r.stats.Blank
		total.Files++
		total.Lines += r.stats.Lines
		total.Code += r.stats.Code
		total.Comment += r.stats.Comment
		total.Blank += r.stats.Blank
		entries = append(entries, FileEntry{Rel: r.rel, Key: key, Stats: r.stats})

		if opts.Verbose {
			if opts.CountFiles {
				fmt.Printf("%-8s %s\n", r.ext, r.rel)
			} else {
				fmt.Printf("%7d lines  %-8s %s\n", r.stats.Lines, r.ext, r.rel)
			}
		}
	}
	return stats, total, entries
}

// analyzeFile reads a file, detects binary content, and classifies its lines.
func analyzeFile(path string, syn commentSyntax) (FileStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileStats{}, err
	}
	defer f.Close()

	head := make([]byte, 8000)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return FileStats{Binary: true}, nil
	}
	rest, err := io.ReadAll(f)
	if err != nil {
		return FileStats{}, err
	}
	return classifyContent(string(head)+string(rest), syn), nil
}

func classifyContent(content string, syn commentSyntax) FileStats {
	var st FileStats
	inBlock := false
	blockEnd := ""
	for _, line := range splitLines(content) {
		st.Lines++
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			st.Blank++
			continue
		}
		if classifyLine(line, syn, &inBlock, &blockEnd) {
			st.Code++
		} else {
			st.Comment++
		}
	}
	return st
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// classifyLine reports whether line contains code (true) or is entirely a
// comment (false), carrying block-comment state across lines.
func classifyLine(line string, syn commentSyntax, inBlock *bool, blockEnd *string) bool {
	s := line
	hasCode := false
	for {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return hasCode
		}
		if *inBlock {
			idx := strings.Index(s, *blockEnd)
			if idx < 0 {
				return hasCode
			}
			s = s[idx+len(*blockEnd):]
			*inBlock = false
			continue
		}
		if hasPrefixAny(s, syn.line) {
			return hasCode
		}
		bestIdx, bestStart, bestEnd := -1, "", ""
		for _, b := range syn.block {
			if idx := strings.Index(s, b[0]); idx >= 0 && (bestIdx == -1 || idx < bestIdx) {
				bestIdx, bestStart, bestEnd = idx, b[0], b[1]
			}
		}
		if bestIdx == -1 {
			return true
		}
		if bestIdx > 0 {
			hasCode = true
		}
		s = s[bestIdx+len(bestStart):]
		idx := strings.Index(s, bestEnd)
		if idx < 0 {
			*inBlock = true
			*blockEnd = bestEnd
			return hasCode
		}
		s = s[idx+len(bestEnd):]
	}
}

func hasPrefixAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// parseSince accepts an absolute date/time or a relative duration such as
// "24h" or "7d".
func parseSince(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if strings.HasSuffix(s, "d") {
		if days, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil {
			return time.Now().AddDate(0, 0, -days), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid --since value %q (use YYYY-MM-DD, RFC3339, or a duration like 24h/7d)", s)
}
