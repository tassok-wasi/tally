package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultIgnore is the built-in list of directory/file names skipped by default.
var DefaultIgnore = map[string]bool{
	"venv":             true,
	".venv":            true,
	"__pycache__":      true,
	".git":             true,
	".svn":             true,
	"node_modules":     true,
	"bower_components": true,
	".tox":             true,
	".mypy_cache":      true,
	".pytest_cache":    true,
	".idea":            true,
	".vscode":          true,
	".vs":              true,
	".eggs":            true,
	"build":            true,
	"dist":             true,
	".cache":           true,
	".github":          true,
}

type ignoreRule struct {
	negate  bool
	dirOnly bool
	re      *regexp.Regexp
}

// ignoreMatcher evaluates a set of gitignore-style patterns against paths
// relative to the directory the patterns were loaded from.
type ignoreMatcher struct {
	rules []ignoreRule
}

func newIgnoreMatcher(patterns []string) *ignoreMatcher {
	m := &ignoreMatcher{}
	for _, raw := range patterns {
		p := strings.TrimRight(raw, " ")
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(p, "!") {
			negate = true
			p = p[1:]
		}
		dirOnly := false
		if strings.HasSuffix(p, "/") {
			dirOnly = true
			p = strings.TrimSuffix(p, "/")
		}
		anchored := false
		if strings.HasPrefix(p, "/") {
			anchored = true
			p = p[1:]
		} else if strings.Contains(p, "/") {
			anchored = true
		}
		if p == "" {
			continue
		}
		m.rules = append(m.rules, ignoreRule{
			negate:  negate,
			dirOnly: dirOnly,
			re:      compileGlobToRegex(p, anchored),
		})
	}
	return m
}

// Match reports whether relPath is ignored. Later rules win, and a negated
// rule re-includes a previously ignored path.
func (m *ignoreMatcher) Match(relPath string, isDir bool) bool {
	if m == nil || len(m.rules) == 0 {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(relPath) {
			ignored = !r.negate
		}
	}
	return ignored
}

// compileGlobToRegex translates a gitignore-style glob into a regular
// expression. When anchored is false the pattern may match at any depth.
func compileGlobToRegex(pattern string, anchored bool) *regexp.Regexp {
	var b strings.Builder
	if anchored {
		b.WriteString("^")
	} else {
		b.WriteString("(?:^|/)")
	}
	i := 0
	for i < len(pattern) {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i += 2
		default:
			c := pattern[i]
			switch c {
			case '*':
				b.WriteString("[^/]*")
				i++
			case '?':
				b.WriteString("[^/]")
				i++
			case '[':
				j := i + 1
				if j < len(pattern) && (pattern[j] == '!' || pattern[j] == '^') {
					j++
				}
				if j < len(pattern) && pattern[j] == ']' {
					j++
				}
				for j < len(pattern) && pattern[j] != ']' {
					j++
				}
				if j < len(pattern) {
					class := pattern[i : j+1]
					if strings.HasPrefix(class, "[!") {
						class = "[^" + class[2:]
					}
					b.WriteString(class)
					i = j + 1
				} else {
					b.WriteString(regexp.QuoteMeta("["))
					i++
				}
			case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\':
				b.WriteByte('\\')
				b.WriteByte(c)
				i++
			default:
				b.WriteByte(c)
				i++
			}
		}
	}
	b.WriteString("(?:/.*)?$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		return regexp.MustCompile(regexp.QuoteMeta(pattern))
	}
	return re
}

// loadGitignore reads .gitignore and .ignore from dir, returning nil when
// neither exists or contains any usable pattern.
func loadGitignore(dir string) *ignoreMatcher {
	var patterns []string
	for _, name := range []string{".gitignore", ".ignore"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			patterns = append(patterns, sc.Text())
		}
		if err := sc.Err(); err != nil {
			f.Close()
			continue
		}
		f.Close()
	}
	if len(patterns) == 0 {
		return nil
	}
	return newIgnoreMatcher(patterns)
}

// globPattern is a compiled --exclude pattern.
type globPattern struct {
	re       *regexp.Regexp
	basename bool
}

// compileExcludes compiles --exclude patterns. A pattern is matched anywhere
// in the tree unless it starts with "/", in which case it is anchored to the
// scan root. Patterns without a slash match against the base name.
func compileExcludes(patterns []string) []globPattern {
	var out []globPattern
	for _, p := range patterns {
		if p == "" {
			continue
		}
		anchored := strings.HasPrefix(p, "/")
		if anchored {
			p = p[1:]
		}
		basename := !anchored && !strings.Contains(p, "/")
		out = append(out, globPattern{
			re:       compileGlobToRegex(p, anchored || basename),
			basename: basename,
		})
	}
	return out
}

func (g globPattern) match(relPath string) bool {
	if g.basename {
		return g.re.MatchString(filepath.Base(relPath))
	}
	return g.re.MatchString(filepath.ToSlash(relPath))
}
