package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestClassifyContent(t *testing.T) {
	tests := []struct {
		name    string
		ext     string
		content string
		want    FileStats
	}{
		{
			name:    "go mixed",
			ext:     ".go",
			content: "package main\n\n// comment\nfunc main() {\n\t/* block\n\t   comment */\n\tprintln(\"hi\")\n}\n",
			want:    FileStats{Lines: 8, Code: 4, Comment: 3, Blank: 1},
		},
		{
			name:    "python hash comments",
			ext:     ".py",
			content: "# comment\nx = 1  # inline\n\n",
			want:    FileStats{Lines: 3, Code: 1, Comment: 1, Blank: 1},
		},
		{
			name:    "no trailing newline",
			ext:     ".go",
			content: "a\nb",
			want:    FileStats{Lines: 2, Code: 2},
		},
		{
			name:    "html block comment",
			ext:     ".html",
			content: "<!-- c -->\n<p>hi</p>\n",
			want:    FileStats{Lines: 2, Code: 1, Comment: 1},
		},
		{
			name:    "code before block comment",
			ext:     ".go",
			content: "x := 1 /* c */\n",
			want:    FileStats{Lines: 1, Code: 1},
		},
		{
			name:    "unknown extension is all code",
			ext:     ".xyz",
			content: "# not a comment\ncode\n",
			want:    FileStats{Lines: 2, Code: 2},
		},
		{
			name:    "empty file",
			ext:     ".go",
			content: "",
			want:    FileStats{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyContent(tt.content, syntaxFor(tt.ext))
			if got != tt.want {
				t.Errorf("classifyContent() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIgnoreMatcher(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		{"*.log", "a.log", false, true},
		{"*.log", "sub/a.log", false, true},
		{"*.log", "a.txt", false, false},
		{"build/", "build", true, true},
		{"build/", "build", false, false},
		{"vendor/**", "vendor/a/b.go", false, true},
		{"vendor/**", "other/vendor/a.go", false, false},
		{"/root.txt", "root.txt", false, true},
		{"/root.txt", "sub/root.txt", false, false},
		{"**/temp", "a/b/temp", true, true},
		{"**/temp", "temp", true, true},
	}
	for _, tt := range tests {
		m := newIgnoreMatcher([]string{tt.pattern})
		if got := m.Match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("pattern %q path %q isDir %v: got %v, want %v", tt.pattern, tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestIgnoreMatcherNegation(t *testing.T) {
	m := newIgnoreMatcher([]string{"*.log", "!keep.log"})
	if !m.Match("a.log", false) {
		t.Error("a.log should be ignored")
	}
	if m.Match("keep.log", false) {
		t.Error("keep.log should be re-included")
	}
}

func TestGlobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.min.js", "a/b/x.min.js", true},
		{"*.min.js", "a/b/x.js", false},
		{"vendor/**", "other/vendor/a.go", true},
		{"/vendor/**", "other/vendor/a.go", false},
		{"/vendor/**", "vendor/a.go", true},
	}
	for _, tt := range tests {
		g := compileExcludes([]string{tt.pattern})[0]
		if got := g.match(tt.path); got != tt.want {
			t.Errorf("pattern %q path %q: got %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestAnalyzeFileBinary(t *testing.T) {
	dir := t.TempDir()

	bin := filepath.Join(dir, "b.dat")
	if err := os.WriteFile(bin, []byte("abc\x00def\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := analyzeFile(bin, commentSyntax{})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Binary {
		t.Error("expected binary file to be detected")
	}

	txt := filepath.Join(dir, "t.txt")
	if err := os.WriteFile(txt, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = analyzeFile(txt, commentSyntax{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Binary {
		t.Error("expected text file not to be binary")
	}
	if st.Lines != 2 {
		t.Errorf("lines = %d, want 2", st.Lines)
	}
}

func TestCollectFiles(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n")
	write("main_test.go", "package main\n")
	write("app.min.js", "x\n")
	write("vendor/lib.go", "package vendor\n")
	write("node_modules/x.js", "x\n")
	write(".hidden.go", "package hidden\n")
	write("sub/deep/file.go", "package deep\n")
	write(".gitignore", "vendor/\n*.min.js\n")

	rel := func(files []string) []string {
		out := make([]string, len(files))
		for i, f := range files {
			r, err := filepath.Rel(root, f)
			if err != nil {
				t.Fatal(err)
			}
			out[i] = filepath.ToSlash(r)
		}
		sort.Strings(out)
		return out
	}

	base := Options{
		Root:        root,
		Extensions:  map[string]bool{".go": true},
		IgnoreNames: DefaultIgnore,
		MaxDepth:    -1,
	}

	tests := []struct {
		name string
		mut  func(*Options)
		want []string
	}{
		{"default", func(o *Options) {}, []string{"main.go", "main_test.go", "sub/deep/file.go"}},
		{"hidden", func(o *Options) { o.Hidden = true }, []string{".hidden.go", "main.go", "main_test.go", "sub/deep/file.go"}},
		{"no gitignore", func(o *Options) { o.NoGitignore = true }, []string{"main.go", "main_test.go", "sub/deep/file.go", "vendor/lib.go"}},
		{"max depth 1", func(o *Options) { o.MaxDepth = 1 }, []string{"main.go", "main_test.go"}},
		{"exclude go", func(o *Options) { o.Excludes = compileExcludes([]string{"*.go"}) }, []string{}},
		{"include sub", func(o *Options) { o.Includes = compileExcludes([]string{"sub/**"}) }, []string{"sub/deep/file.go"}},
		{"exclude tests", func(o *Options) { o.ExcludeTests = true }, []string{"main.go", "sub/deep/file.go"}},
		{"all files", func(o *Options) { o.AllExts = true }, []string{"main.go", "main_test.go", "sub/deep/file.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := base
			tt.mut(&opts)
			files, err := collectFiles(opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := rel(files); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunAggregates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\n// c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := collectFiles(Options{
		Root:        root,
		Extensions:  map[string]bool{".go": true},
		IgnoreNames: DefaultIgnore,
		MaxDepth:    -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	stats, total, entries := run(root, files, RunOptions{Workers: 2})
	if total.Files != 2 {
		t.Errorf("total files = %d, want 2", total.Files)
	}
	if total.Lines != 4 {
		t.Errorf("total lines = %d, want 4", total.Lines)
	}
	if total.Comment != 1 {
		t.Errorf("total comments = %d, want 1", total.Comment)
	}
	if s := stats[".go"]; s == nil || s.Files != 2 {
		t.Errorf(".go stats = %+v, want 2 files", s)
	}
	if len(entries) != 2 {
		t.Errorf("entries = %d, want 2", len(entries))
	}
}

func TestRunMinMaxLines(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "small.go"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "big.go"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := collectFiles(Options{
		Root:        root,
		Extensions:  map[string]bool{".go": true},
		IgnoreNames: DefaultIgnore,
		MaxDepth:    -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, total, _ := run(root, files, RunOptions{Workers: 1, MinLines: 2})
	if total.Files != 1 {
		t.Errorf("min-lines: files = %d, want 1", total.Files)
	}
	_, total, _ = run(root, files, RunOptions{Workers: 1, MaxLines: 2})
	if total.Files != 1 {
		t.Errorf("max-lines: files = %d, want 1", total.Files)
	}
}

func TestRunByDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := collectFiles(Options{
		Root:        root,
		Extensions:  map[string]bool{".go": true},
		IgnoreNames: DefaultIgnore,
		MaxDepth:    -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, _, _ := run(root, files, RunOptions{Workers: 1, ByDir: true})
	if stats["pkg"] == nil {
		t.Errorf("expected a 'pkg' group, got %v", stats)
	}
	if stats["./"] == nil {
		t.Errorf("expected a './' group, got %v", stats)
	}
}

func TestTopFiles(t *testing.T) {
	entries := []FileEntry{
		{Rel: "a", Stats: FileStats{Lines: 5}},
		{Rel: "b", Stats: FileStats{Lines: 20}},
		{Rel: "c", Stats: FileStats{Lines: 10}},
	}
	got := topFiles(entries, 2)
	if len(got) != 2 || got[0].Rel != "b" || got[1].Rel != "c" {
		t.Errorf("topFiles = %+v, want [b c]", got)
	}
}

func TestSortKeys(t *testing.T) {
	stats := map[string]*Stats{
		".go": {Files: 2, Lines: 10},
		".py": {Files: 5, Lines: 3},
		".md": {Files: 1, Lines: 100},
	}
	tests := []struct {
		order string
		want  []string
	}{
		{"name", []string{".go", ".md", ".py"}},
		{"lines", []string{".md", ".go", ".py"}},
		{"files", []string{".py", ".go", ".md"}},
	}
	for _, tt := range tests {
		keys := []string{".go", ".py", ".md"}
		sortKeys(keys, stats, tt.order)
		if !reflect.DeepEqual(keys, tt.want) {
			t.Errorf("sortKeys(%q) = %v, want %v", tt.order, keys, tt.want)
		}
	}
}

func TestParseSince(t *testing.T) {
	if _, err := parseSince("2024-01-01"); err != nil {
		t.Errorf("date: %v", err)
	}
	if _, err := parseSince("24h"); err != nil {
		t.Errorf("duration: %v", err)
	}
	if _, err := parseSince("7d"); err != nil {
		t.Errorf("days: %v", err)
	}
	if _, err := parseSince("nonsense"); err == nil {
		t.Error("expected error for invalid value")
	}
}

func TestParseBudget(t *testing.T) {
	ext, n, err := parseBudget(".go=1000")
	if err != nil || ext != ".go" || n != 1000 {
		t.Errorf("parseBudget = %q %d %v", ext, n, err)
	}
	ext, _, err = parseBudget("py=5")
	if err != nil || ext != ".py" {
		t.Errorf("parseBudget = %q %v", ext, err)
	}
	if _, _, err := parseBudget("bad"); err == nil {
		t.Error("expected error for invalid budget")
	}
}

func TestIsTestPath(t *testing.T) {
	tests := []struct {
		rel, name string
		want      bool
	}{
		{"main.go", "main.go", false},
		{"main_test.go", "main_test.go", true},
		{"test/foo.go", "foo.go", true},
		{"spec/foo.js", "foo.js", true},
		{"foo.spec.ts", "foo.spec.ts", true},
		{"test_foo.py", "test_foo.py", true},
	}
	for _, tt := range tests {
		if got := isTestPath(tt.rel, tt.name); got != tt.want {
			t.Errorf("isTestPath(%q, %q) = %v, want %v", tt.rel, tt.name, got, tt.want)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestRenderMarkdown(t *testing.T) {
	rows := []Row{{Key: ".go", Stats: Stats{Files: 2, Lines: 10}}}
	out := captureStdout(t, func() {
		renderMarkdown(rows, Stats{Files: 2, Lines: 10}, renderOptions{KeyLabel: "Extension"})
	})
	if !strings.Contains(out, "| Extension | Files | Lines |") {
		t.Errorf("markdown header missing:\n%s", out)
	}
	if !strings.Contains(out, "| .go | 2 | 10 |") {
		t.Errorf("markdown row missing:\n%s", out)
	}
}

func TestRenderJSON(t *testing.T) {
	rows := []Row{{Key: ".go", Stats: Stats{Files: 2, Lines: 10}}}
	out := captureStdout(t, func() {
		renderJSON(rows, Stats{Files: 2, Lines: 10}, renderOptions{KeyLabel: "Extension"})
	})
	if !strings.Contains(out, `".go"`) || !strings.Contains(out, `"lines": 10`) {
		t.Errorf("json output unexpected:\n%s", out)
	}
}

func TestCheckCmd(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pass := CheckCmd{MaxTotalLines: 100, scan: &ScanFlags{Dir: root, Extensions: []string{"go"}, MaxDepth: -1}}
	captureStdout(t, func() {
		if err := pass.Run(); err != nil {
			t.Errorf("expected pass, got %v", err)
		}
	})

	fail := CheckCmd{Budget: []string{".go=0"}, scan: &ScanFlags{Dir: root, Extensions: []string{"go"}, MaxDepth: -1}}
	captureStdout(t, func() {
		if err := fail.Run(); err == nil {
			t.Error("expected budget failure")
		}
	})
}

func TestDiffCmd(t *testing.T) {
	from := t.TempDir()
	to := t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "a.go"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(to, "a.go"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := DiffCmd{From: from, To: to, scan: &ScanFlags{Extensions: []string{"go"}, MaxDepth: -1}}
	out := captureStdout(t, func() {
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "+2") {
		t.Errorf("expected +2 delta in output:\n%s", out)
	}
}

func TestLanguagesCmd(t *testing.T) {
	out := captureStdout(t, func() {
		if err := (&LanguagesCmd{}).Run(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, ".go") {
		t.Errorf("languages output missing .go:\n%s", out)
	}
}
