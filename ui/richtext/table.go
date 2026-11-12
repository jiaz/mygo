package richtext

import (
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// TableRow is what row of a table a paragraph is (ParagraphStyle.Row).
type TableRow uint8

const (
	NoRow TableRow = iota
	// HeaderRow is a table's first row, which starts it.
	HeaderRow
	// BodyRow is a row after it.
	BodyRow
)

const (
	// CellSeparator parts a row's cells.
	CellSeparator = '\t'
	// CellLineBreak breaks a cell's line, as <br> in Markdown.
	CellLineBreak = ' '
)

// columnsOf returns a table's alignments as ParagraphStyle.Columns.
func columnsOf(aligns []parser.Align) string {
	var b strings.Builder
	for _, a := range aligns {
		switch a {
		case parser.AlignLeft:
			b.WriteByte('l')
		case parser.AlignCenter:
			b.WriteByte('c')
		case parser.AlignRight:
			b.WriteByte('r')
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// splitCells returns a row's runs as its cells' runs, at the tabs.
func splitCells(runs []Run) [][]Run {
	cells := [][]Run{nil}
	for _, r := range runs {
		for {
			part, rest, cut := strings.Cut(r.Text, string(CellSeparator))
			if part != "" {
				cells[len(cells)-1] = append(cells[len(cells)-1], Run{Text: part, Attributes: r.Attributes})
			}
			if !cut {
				break
			}
			cells = append(cells, nil)
			r.Text = rest
		}
	}
	return cells
}

// cellText returns s as a cell's text: no tabs, its lines broken by line
// separators.
func cellText(s string) string {
	return strings.NewReplacer("\t", " ", "\r\n", string(CellLineBreak), "\n", string(CellLineBreak)).Replace(s)
}

// tableEnd returns the paragraph after the table whose first row is
// paras[i].
func tableEnd(paras []Paragraph, i int) int {
	j := i + 1
	for j < len(paras) && paras[j].Style.Row == BodyRow && paras[j].Style.Quote == paras[i].Style.Quote {
		j++
	}
	return j
}

// writeMarkdownTable writes the rows as a GitHub table: the first as its
// header (a table has one in Markdown), the delimiter row with the
// columns' alignments, every row as many cells as the table has columns.
func writeMarkdownTable(b *strings.Builder, rows []Paragraph, quote string) {
	cols := rows[0].Style.Columns
	n := len(cols)
	cells := make([][][]Run, len(rows))
	for i, r := range rows {
		cells[i] = splitCells(r.Runs)
		n = max(n, len(cells[i]))
	}
	line := func(cs [][]Run) {
		b.WriteString(quote + "|")
		for k := range n {
			var c strings.Builder
			if k < len(cs) {
				writeMarkdownRuns(&c, cs[k])
			}
			s := strings.ReplaceAll(c.String(), "|", "\\|")
			s = strings.ReplaceAll(s, string(CellLineBreak), "<br>")
			b.WriteString(" " + s + " |")
		}
	}
	for i := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		line(cells[i])
		if i == 0 {
			b.WriteString("\n" + quote + "|")
			for k := range n {
				switch {
				case k < len(cols) && cols[k] == 'l':
					b.WriteString(" :--- |")
				case k < len(cols) && cols[k] == 'c':
					b.WriteString(" :---: |")
				case k < len(cols) && cols[k] == 'r':
					b.WriteString(" ---: |")
				default:
					b.WriteString(" --- |")
				}
			}
		}
	}
}
