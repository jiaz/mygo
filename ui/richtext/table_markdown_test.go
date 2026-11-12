package richtext

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// gfmHTML renders Markdown as GitHub's Markdown would, through MyGo's own
// GFM parser: what a reader of the Markdown sees.
func gfmHTML(s string) string {
	d := parser.New(parser.GFM | parser.ExtFootnotes)
	d.Set(s)
	return d.HTML()
}

// tableCases are GitHub tables, each with what is lost when written back,
// if anything.
var tableCases = []struct {
	name, md string
	// same is false where the Markdown written back renders otherwise
	// than md, on purpose: why in lost.
	lost string
}{
	{name: "basic", md: "| Time | Talk | Room |\n| --- | --- | --- |\n| 9:00 | Welcome & coffee | Hall |\n| 9:30 | Keynote | Main stage |"},
	{name: "alignment", md: "| Left | Center | Right | None |\n| :--- | :---: | ---: | --- |\n| a | b | c | d |"},
	{name: "inline formats", md: "| Formats |\n| --- |\n| **bold** *italic* ~~struck~~ `code` <u>under</u> [a link](https://example.com) |"},
	{name: "escaped pipe and pipe in code", md: "| Pipes |\n| --- |\n| a \\| b and `x \\| y` |"},
	{name: "empty and short rows", md: "| a | b | c |\n| --- | --- | --- |\n| 1 |\n|  | 2 |  |"},
	{name: "row longer than the header (GitHub drops the extra cell too)", md: "| a | b |\n| --- | --- |\n| 1 | 2 | 3 |"},
	{name: "line break in a cell", md: "| Points |\n| --- |\n| Confirm the pain<br>Agree the next step |"},
	{name: "no outer pipes", md: "a | b\n--|--\n1 | 2"},
	{name: "in a quote", md: "> | a | b |\n> | --- | --- |\n> | 1 | 2 |"},
	{name: "two tables", md: "| a |\n| --- |\n| 1 |\n\n| b |\n| --- |\n| 2 |"},
	{name: "between paragraphs", md: "Before.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\nAfter."},
	{name: "header only", md: "| a | b |\n| --- | --- |"},
	{name: "images and an app's own URLs in cells", md: "| Image | Embed |\n| --- | --- |\n| ![chart](https://example.com/chart.png) | ![Board](app:embed/board) |"},
	{name: "escapes, entities, CJK, emoji", md: "| Text |\n| --- |\n| \\*not bold\\* &amp; 会议 🎉 # not a heading |"},
	{name: "image with a link around it", md: "| Shot |\n| --- |\n| [![chart](https://example.com/c.png)](https://example.com) |", lost: "the link around the image, which the editor drops outside tables too"},
	{name: "in a list item", md: "- Notes\n\n    | a | b |\n    | --- | --- |\n    | 1 | 2 |"},
	{name: "right under a paragraph", md: "The plan:\n| a | b |\n| --- | --- |\n| 1 | 2 |"},
	{name: "HTML marks in a cell", md: "| a |\n| --- |\n| <b>bold</b> <s>gone</s> |", lost: "nothing seen: <b> and <s> come back as ** and ~~ (<strong>, <del>)"},
	{name: "inline HTML other than marks", md: "| a |\n| --- |\n| <span>kept text</span> |", lost: "the <span> tag, which the editor never keeps"},
}

// TestTablesReadAndWriteAsGitHubs reads each table, writes it back and
// reads that again: the text is the same both times, the Markdown written
// is stable, and it renders as the table read did.
func TestTablesReadAndWriteAsGitHubs(t *testing.T) {
	for _, c := range tableCases {
		t.Run(c.name, func(t *testing.T) {
			t1 := ParseMarkdown(c.md)
			md2 := t1.Markdown()
			t2 := ParseMarkdown(md2)
			if dumpRuns(t1) != dumpRuns(t2) {
				t.Errorf("read back otherwise:\n%s\nwritten:\n%s", dumpText(t1), md2)
			}
			if md3 := t2.Markdown(); md3 != md2 {
				t.Errorf("not stable:\n%s\nthen:\n%s", md2, md3)
			}
			rows := 0
			for _, p := range t1.Paragraphs() {
				if p.Style.Row != NoRow {
					rows++
				}
			}
			if rows == 0 {
				t.Errorf("no rows read from\n%s", c.md)
			}
			h1, h2 := gfmHTML(c.md), gfmHTML(md2)
			if (h1 == h2) != (c.lost == "") {
				t.Errorf("renders otherwise (lost %q):\n%s\nas\n%s\nwritten:\n%s", c.lost, h1, h2, md2)
			}
			t.Logf("written:\n%s", md2)
		})
	}
}

func dumpText(t Text) string {
	var b strings.Builder
	for _, p := range t.Paragraphs() {
		b.WriteString(strings.NewReplacer("\t", "⇥", string(CellLineBreak), "↵").Replace(plainOf(p)))
		b.WriteString("  ")
		b.WriteString(strings.TrimSpace(strings.ReplaceAll(fmtStyle(p.Style), "\n", " ")))
		b.WriteByte('\n')
	}
	return b.String()
}

func fmtStyle(s ParagraphStyle) string {
	out := ""
	switch s.Row {
	case HeaderRow:
		out += "header "
	case BodyRow:
		out += "row "
	}
	if s.Columns != "" {
		out += "cols=" + s.Columns + " "
	}
	if s.Quote {
		out += "quote "
	}
	return out
}

// dumpRuns writes a text's paragraphs, styles and runs, an attachment by
// its name and URL: two reads of one Markdown make two attachments.
func dumpRuns(t Text) string {
	var b strings.Builder
	for _, p := range t.Paragraphs() {
		fmt.Fprintf(&b, "%+v\n", p.Style)
		for _, r := range p.Runs {
			a := r.Attributes
			at := ""
			if a.Attachment != nil {
				at = a.Attachment.Name + " " + a.Attachment.URL
			}
			a.Attachment = nil
			fmt.Fprintf(&b, "  %q %+v %s\n", r.Text, a, at)
		}
	}
	return b.String()
}
