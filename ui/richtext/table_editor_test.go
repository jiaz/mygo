package richtext

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// shots is where the tests leave their pictures, when set.
var shots = os.Getenv("TABLE_SHOTS")

func shoot(t *testing.T, tt *ui.Tester, name string) {
	if shots == "" {
		return
	}
	f, err := os.Create(filepath.Join(shots, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

const scheduleMD = "## Schedule\n\nBefore the table.\n\n" +
	"| Time | Talk | Notes |\n| ---: | --- | --- |\n" +
	"| 9:00 | Welcome & coffee | Badges at the **front desk**<br>Wi-Fi on the back |\n" +
	"| 9:30 | Keynote | Slides ![slides.pdf](app:file/slides.pdf) |\n\n" +
	"![Map](app:embed/map)\n\nAfter the table."

// sized gives the attachments sizes, as an app would: an embed a box as
// wide as the text, a file a chip.
func sized(t Text) Text {
	var paras []Paragraph
	for _, p := range t.Paragraphs() {
		for k, r := range p.Runs {
			if at := r.Attributes.Attachment; at != nil {
				a := *at
				if strings.HasPrefix(a.URL, "app:embed/") {
					a.Width, a.Height = 4000, 56
				} else {
					a.Width, a.Height = 92, 20
				}
				p.Runs[k].Attributes.Attachment = &a
			}
		}
		paras = append(paras, p)
	}
	return TextOf(paras...)
}

// rowCells returns the cells of each row, as the text says them.
func rowCells(t Text) [][]string {
	var out [][]string
	for _, p := range t.Paragraphs() {
		if p.Style.Row != NoRow {
			out = append(out, strings.Split(plainOf(p), "\t"))
		}
	}
	return out
}

func TestATableIsEditedAsCells(t *testing.T) {
	doc := NewDocument(sized(ParseMarkdown(scheduleMD)))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc).Fill() }, 900, 560)
	tt.Frame()
	shoot(t, tt, "table-light")

	// The rows' boxes, to click in their cells.
	var boxes []ParagraphBox
	for _, b := range doc.VisibleParagraphs() {
		if p := seqAt(doc.Text().seq(), b.Index); p.style.Row != NoRow {
			boxes = append(boxes, b)
		}
	}
	if len(boxes) != 3 {
		t.Fatalf("rows in view: %d, want 3", len(boxes))
	}
	// A click in the first body row's middle cell, right of its text:
	// the caret at the end of "Welcome & coffee".
	at := doc.Text().paraStart(boxes[1].Index) + len([]rune("9:00\tWelcome & coffee"))
	doc.SetSelection(at, at)
	tt.Frame()
	r, _ := doc.CaretRect()
	doc.SetSelection(0, 0)
	tt.Frame()
	tt.ClickAt(r.X+16, r.Y+r.H/2)
	tt.Type("!")
	if got := rowCells(doc.Text())[1][1]; got != "Welcome & coffee!" {
		t.Fatalf("typed in the cell clicked: %q", got)
	}

	// Tab: the end of the next cell.
	tt.Key(0, ui.KeyTab)
	tt.Type(" (ask)")
	if got := rowCells(doc.Text())[1][2]; !strings.HasSuffix(got, "Wi-Fi on the back (ask)") {
		t.Fatalf("Tab to the next cell: %q", got)
	}
	// Shift-Tab twice: the first cell.
	tt.Key(ui.Shift, ui.KeyTab)
	tt.Key(ui.Shift, ui.KeyTab)
	tt.Type("~")
	if got := rowCells(doc.Text())[1][0]; got != "9:00~" {
		t.Fatalf("Shift-Tab back: %q", got)
	}

	// Backspace at a cell's start and Delete at its end do nothing: the
	// cells never merge.
	before := doc.Text().Markdown()
	ps := doc.Text().paraStart(boxes[1].Index)
	start := ps + len([]rune("9:00~")) + 1 // after the first tab
	doc.SetSelection(start, start)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if got := doc.Text().Markdown(); got != before {
		t.Fatalf("Backspace at a cell's start changed the table:\n%s", got)
	}
	if a, z := doc.Selection(); a != start || z != start {
		t.Fatalf("Backspace at a cell's start moved the caret to %d-%d, from %d", a, z, start)
	}
	doc.SetSelection(start-1, start-1)
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	if got := doc.Text().Markdown(); got != before {
		t.Fatalf("Delete at a cell's end changed the table:\n%s", got)
	}

	// Down from the header's first cell: the first body row's.
	hs := doc.Text().paraStart(boxes[0].Index)
	doc.SetSelection(hs+2, hs+2)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Type("^")
	if got := rowCells(doc.Text())[1][0]; !strings.Contains(got, "^") {
		t.Fatalf("Down from the header: %q", got)
	}

	// A selection across cells, deleted: their text goes, the cells stay.
	rs := doc.Text().paraStart(boxes[2].Index)
	doc.SetSelection(rs+2, rs+len([]rune("9:30\tKey")))
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	cells := rowCells(doc.Text())
	if len(cells[2]) != 3 || cells[2][0] != "9:" || cells[2][1] != "note" {
		t.Fatalf("deleted across cells: %q", cells[2])
	}

	// Tab in the last cell adds a row; Enter another; Enter on an empty
	// last row leaves the table.
	end := doc.Text().paraStart(boxes[2].Index) + seqAt(doc.Text().seq(), boxes[2].Index).n
	doc.SetSelection(end, end)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Type("10:30")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	tt.Type("Notes under the table")
	cells = rowCells(doc.Text())
	if len(cells) != 4 || cells[3][0] != "10:30" || len(cells[3]) != 3 {
		t.Fatalf("rows after Tab and Enter: %q", cells)
	}

	// An input method composing in a cell.
	doc.SetSelection(doc.Text().paraStart(boxes[2].Index)+2, doc.Text().paraStart(boxes[2].Index)+2)
	tt.Frame()
	tt.Compose("会", 1)
	tt.Frame()
	shoot(t, tt, "table-composing")
	tt.Compose("", 0)
	tt.Type("会议")
	if got := rowCells(doc.Text())[2][0]; got != "9:会议" {
		t.Fatalf("composed in a cell: %q", got)
	}
	tt.Frame()
	shoot(t, tt, "table-edited")
	md := doc.Text().Markdown()
	t.Logf("Markdown after the edits:\n%s", md)
	if !strings.Contains(md, "| 10:30 |  |  |\n\nNotes under the table") {
		t.Fatalf("written:\n%s", md)
	}

	// Undo takes each step back, to the table read.
	for doc.CanUndo() {
		doc.Undo()
	}
	if got := doc.Text().Markdown(); got != sized(ParseMarkdown(scheduleMD)).Markdown() {
		t.Fatalf("undone:\n%s", got)
	}
	tt.SetDark(true)
	tt.Frame()
	shoot(t, tt, "table-dark")
}

// TestATableKeepsItsShape pastes into a table and deletes the empty
// paragraphs around it: the rows stay rows, as many cells as they were.
func TestATableKeepsItsShape(t *testing.T) {
	row := func(r TableRow, s string) Paragraph {
		return Paragraph{Style: ParagraphStyle{Row: r, Columns: "--"}, Runs: []Run{{Text: s}}}
	}
	empty := Paragraph{}
	doc := NewDocument(TextOf(empty, row(HeaderRow, "a\tb"), row(BodyRow, "1\t2"), empty))
	tt := ui.NewTester(func(c *ui.Context) { Editor(c, doc).Fill() }, 600, 300)
	tt.Frame()
	tt.Press(300, 150)
	tt.Release(300, 150)
	styles := func() (out []TableRow) {
		for _, p := range doc.Text().Paragraphs() {
			out = append(out, p.Style.Row)
		}
		return out
	}

	// Rich text pasted in a cell is its text: its paragraphs the cell's
	// lines, its tabs spaces, its formats kept.
	end := doc.Text().paraStart(2) + 1 // after "1"
	doc.SetSelection(end, end)
	tt.Frame()
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte("<p><b>x</b></p><p>y\tz</p>")), transfer.Bytes(transfer.Text, []byte("x\ny\tz")))))
	tt.Key(ui.Cmd, ui.KeyV)
	if got := rowCells(doc.Text())[1]; len(got) != 2 || got[0] != "1x\u2028y z" || got[1] != "2" {
		t.Fatalf("pasted in a cell: %q", got)
	}
	if a := doc.Text().AttributesAt(end); a.Weight != 700 {
		t.Fatalf("the paste's bold: %+v", a)
	}
	// At a row's start, the row keeps its style.
	start := doc.Text().paraStart(2)
	doc.SetSelection(start, start)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyV)
	if got := styles(); len(got) != 4 || got[2] != BodyRow {
		t.Fatalf("pasted at a row's start: %v", got)
	}

	// Backspace in the empty paragraph after the table takes it away.
	n := doc.Text().Len()
	doc.SetSelection(n, n)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if got := styles(); len(got) != 3 || got[1] != HeaderRow || got[2] != BodyRow {
		t.Fatalf("Backspace after the table: %v", got)
	}
	// Backspace at the header's start takes the empty paragraph above.
	doc.SetSelection(1, 1)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if got := styles(); len(got) != 2 || got[0] != HeaderRow || got[1] != BodyRow {
		t.Fatalf("Backspace at the header's start: %v", got)
	}
	if got := rowCells(doc.Text()); len(got[0]) != 2 || len(got[1]) != 2 {
		t.Fatalf("cells: %q", got)
	}
}
