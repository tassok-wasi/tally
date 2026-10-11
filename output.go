package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
)

// Row is one line of the breakdown.
type Row struct {
	Key   string
	Stats Stats
}

type renderOptions struct {
	KeyLabel    string
	CountFiles  bool
	Breakdown   bool
	SummaryOnly bool
	Color       bool
	Largest     []FileEntry
}

func itoa(n int) string { return strconv.Itoa(n) }

func render(format string, rows []Row, total Stats, opts renderOptions) {
	switch format {
	case "json":
		renderJSON(rows, total, opts)
	case "csv":
		renderDelimited(rows, total, opts, ',')
	case "tsv":
		renderDelimited(rows, total, opts, '\t')
	case "markdown":
		renderMarkdown(rows, total, opts)
	case "html":
		renderHTML(rows, total, opts)
	default:
		renderTable(rows, total, opts)
	}
}

func headers(opts renderOptions) []string {
	h := []string{opts.KeyLabel, "Files"}
	if !opts.CountFiles {
		h = append(h, "Lines")
		if opts.Breakdown {
			h = append(h, "Code", "Comment", "Blank")
		}
	}
	return h
}

func cells(key string, s Stats, opts renderOptions) []string {
	c := []string{key, itoa(s.Files)}
	if !opts.CountFiles {
		c = append(c, itoa(s.Lines))
		if opts.Breakdown {
			c = append(c, itoa(s.Code), itoa(s.Comment), itoa(s.Blank))
		}
	}
	return c
}

func renderTable(rows []Row, total Stats, opts renderOptions) {
	if opts.SummaryOnly {
		fmt.Println()
		fmt.Println(colorize("TOTAL", "1", opts.Color) + "  " + strings.Join(cells("", total, opts)[1:], "  "))
		return
	}

	head := headers(opts)
	data := make([][]string, 0, len(rows)+1)
	for _, r := range rows {
		data = append(data, cells(r.Key, r.Stats, opts))
	}
	totalRow := cells("TOTAL", total, opts)

	widths := make([]int, len(head))
	for i, h := range head {
		widths[i] = len(h)
	}
	for _, row := range append(data, totalRow) {
		for i, c := range row {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	sep := strings.Repeat("-", totalWidth(widths))

	fmt.Println()
	fmt.Println(sep)
	fmt.Println(colorize(formatRow(head, widths), "1", opts.Color))
	fmt.Println(sep)
	for _, row := range data {
		fmt.Println(formatRow(row, widths))
	}
	fmt.Println(sep)
	fmt.Println(colorize(formatRow(totalRow, widths), "1", opts.Color))

	if len(opts.Largest) > 0 {
		renderLargest(opts)
	}
}

func renderLargest(opts renderOptions) {
	fmt.Println()
	fmt.Println(colorize(" Largest Files", "1", opts.Color))
	fmt.Println("----------------------------------------")
	for _, e := range opts.Largest {
		fmt.Printf("%8d  %s\n", e.Stats.Lines, e.Rel)
	}
}

func totalWidth(widths []int) int {
	w := 0
	for _, x := range widths {
		w += x
	}
	return w + 2*(len(widths)-1)
}

func formatRow(cells []string, widths []int) string {
	var b strings.Builder
	for i, c := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		pad := widths[i] - len(c)
		if i == 0 {
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", pad))
		} else {
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(c)
		}
	}
	return b.String()
}

type jsonRow struct {
	Key string `json:"key"`
	Stats
}

type jsonFile struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

func renderJSON(rows []Row, total Stats, opts renderOptions) {
	out := struct {
		Key     string     `json:"key"`
		Rows    []jsonRow  `json:"rows"`
		Total   Stats      `json:"total"`
		Largest []jsonFile `json:"largest,omitempty"`
	}{Key: opts.KeyLabel, Total: total}
	for _, r := range rows {
		out.Rows = append(out.Rows, jsonRow{Key: r.Key, Stats: r.Stats})
	}
	for _, e := range opts.Largest {
		out.Largest = append(out.Largest, jsonFile{Path: e.Rel, Lines: e.Stats.Lines})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func renderDelimited(rows []Row, total Stats, opts renderOptions, comma rune) {
	w := csv.NewWriter(os.Stdout)
	w.Comma = comma
	_ = w.Write(headers(opts))
	for _, r := range rows {
		_ = w.Write(cells(r.Key, r.Stats, opts))
	}
	_ = w.Write(cells("TOTAL", total, opts))
	w.Flush()
}

func renderMarkdown(rows []Row, total Stats, opts renderOptions) {
	head := headers(opts)
	fmt.Println("| " + strings.Join(head, " | ") + " |")
	sep := make([]string, len(head))
	for i := range sep {
		sep[i] = "---"
	}
	fmt.Println("| " + strings.Join(sep, " | ") + " |")
	for _, r := range rows {
		fmt.Println("| " + strings.Join(cells(r.Key, r.Stats, opts), " | ") + " |")
	}
	fmt.Println("| " + strings.Join(cells("TOTAL", total, opts), " | ") + " |")
}

func renderHTML(rows []Row, total Stats, opts renderOptions) {
	head := headers(opts)
	fmt.Println("<table>")
	fmt.Println("  <thead><tr>")
	for _, h := range head {
		fmt.Printf("    <th>%s</th>\n", html.EscapeString(h))
	}
	fmt.Println("  </tr></thead>")
	fmt.Println("  <tbody>")
	for _, r := range rows {
		renderHTMLRow(cells(r.Key, r.Stats, opts))
	}
	renderHTMLRow(cells("TOTAL", total, opts))
	fmt.Println("  </tbody>")
	fmt.Println("</table>")
}

func renderHTMLRow(cells []string) {
	fmt.Println("    <tr>")
	for _, c := range cells {
		fmt.Printf("      <td>%s</td>\n", html.EscapeString(c))
	}
	fmt.Println("    </tr>")
}

func colorize(s, code string, enabled bool) string {
	if !enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func colorEnabled(noColor bool) bool {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
