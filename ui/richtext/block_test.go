package richtext

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// card is an attachment shown as a block: a box h high, which counts its
// builds and binds its element to handle, in a block named name.
type card struct {
	name   string
	h      float32
	builds int
	handle ui.Handle
	at     *Attachment
	// inside builds what the card holds, if anything.
	inside func(c *ui.Context)
}

func newCard(name string, h float32) *card {
	cd := &card{name: name, h: h}
	cd.at = &Attachment{Name: name, Height: 24, Element: func(c *ui.Context, width float32) ui.Element {
		cd.builds++
		e := ui.Box(c).Height(cd.h).Background(ui.RGB(200, 200, 200)).Bind(&cd.handle)
		if cd.inside != nil {
			e.Children(func() { cd.inside(c) })
		}
		return e
	}}
	return cd
}

// blockPara returns a paragraph of the attachment alone.
func blockPara(at *Attachment) Paragraph {
	return Paragraph{Runs: []Run{{Text: string(ObjectReplacement), Attributes: Attributes{Attachment: at}}}}
}

// blockWorld is an editor of doc filling a window, which counts its
// builds and keeps its context, to read the cards' handles.
type blockWorld struct {
	tt     *ui.Tester
	doc    *Document
	c      *ui.Context
	builds int
}

func newBlockWorld(t *testing.T, doc *Document, w, h int, opts EditorOptions) *blockWorld {
	t.Helper()
	if opts.Font.Size == 0 {
		opts.Font.Size = 10
	}
	bw := &blockWorld{doc: doc}
	doc.SetSelection(0, 0)
	bw.tt = openTester(t, func(c *ui.Context) {
		bw.c = c
		bw.builds++
		EditorBase(c, doc, opts).Fill()
	}, w, h)
	bw.tt.Frame()
	return bw
}

// view returns the editor's view.
func (bw *blockWorld) view(t *testing.T) *view {
	t.Helper()
	v := bw.doc.shownIn.Value()
	if v == nil {
		t.Fatal("no editor shows the document")
	}
	return v
}

// box returns paragraph i as the editor painted it in the last frame.
func (bw *blockWorld) box(t *testing.T, i int) ParagraphBox {
	t.Helper()
	return boxOf(t, bw.view(t).boxes(nil), i)
}

// bounds returns the card's element as the last frame laid it out.
func (bw *blockWorld) bounds(cd *card) ui.Rect { return cd.handle.Bounds(bw.c) }

// frame builds one frame, and fails unless it was one.
func (bw *blockWorld) frame(t *testing.T, what string) {
	t.Helper()
	n := bw.builds
	bw.tt.Frame()
	if bw.builds != n+1 {
		t.Fatalf("%s: %d frames for one", what, bw.builds-n)
	}
}

// focus gives the editor the focus with a press on its first line, and
// puts the selection at a to z.
func (bw *blockWorld) focus(a, z int) {
	bw.tt.Press(2, 2)
	bw.tt.Release(2, 2)
	bw.doc.SetSelection(a, z)
	bw.tt.Frame()
}

// A block's element shows where its paragraph is, as wide as the text, and
// when it grows the text below it moves in the frame it grows in.
func TestBlockGrowsInItsFrame(t *testing.T) {
	cd := newCard("Card", 40)
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	check := func(what string) float32 {
		t.Helper()
		r, blk, after := bw.bounds(cd), bw.box(t, 1), bw.box(t, 2)
		if r.H != cd.h || blk.Height != cd.h || !near(r.Y, blk.Top) || r.W != 300 {
			t.Fatalf("%s: the element is at %v, its paragraph at %+v", what, r, blk)
		}
		if after.Top < r.Y+r.H {
			t.Fatalf("%s: the paragraph below starts at %v, the element ends at %v", what, after.Top, r.Y+r.H)
		}
		return after.Top - (r.Y + r.H)
	}
	gap := check("built")
	cd.h = 80
	bw.frame(t, "grown")
	if got := check("grown"); !near(got, gap) {
		t.Fatalf("grown, the paragraph below is %v below the element, was %v", got, gap)
	}
	cd.h = 30
	bw.frame(t, "shrunk")
	if got := check("shrunk"); !near(got, gap) {
		t.Fatalf("shrunk, the paragraph below is %v below the element, was %v", got, gap)
	}
	// Frames that change nothing ask for none more.
	bw.frame(t, "still")
	if bw.tt.FramePending() {
		t.Fatal("the frames do not settle")
	}
}

// A block's element paints where its paragraph is, partly scrolled out at
// the top too, and a block selected paints no selection of the editor's
// around it: the element shows it.
func TestBlockPaints(t *testing.T) {
	cd := newCard("Card", 60)
	paras := []Paragraph{para("Before"), blockPara(cd.at)}
	for i := range 100 {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	doc := NewDocument(TextOf(paras...))
	selection := ui.RGB(0, 0, 255)
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{Theme: &Theme{Selection: selection}})
	bw.focus(7, 8)
	doc.ScrollToParagraph(1, ui.ScrollStart)
	bw.tt.Frame()
	doc.ScrollBy(30)
	bw.tt.Frame()
	r := bw.bounds(cd)
	if r.Y >= 0 || r.Y+r.H <= 0 {
		t.Fatalf("the block is at %v", r)
	}
	img := bw.tt.Image()
	if got := img.RGBAAt(int(r.X+r.W/2), int(r.Y+r.H/2)); got.R != 200 || got.G != 200 || got.B != 200 {
		t.Fatalf("the element partly out of view paints %v", got)
	}
	if got := img.RGBAAt(int(r.X+r.W/2), int(r.Y+r.H)+1); got.B > 200 && got.R < 60 {
		t.Fatalf("below the selected block, the editor paints the selection: %v", got)
	}
}

// A block below one that grows moves with it in that frame, where its
// paragraph is painted.
func TestBlockBelowMovesInTheFrame(t *testing.T) {
	a, b := newCard("A", 40), newCard("B", 30)
	doc := NewDocument(TextOf(para("Before"), blockPara(a.at), para("Middle"), blockPara(b.at), para("After")))
	bw := newBlockWorld(t, doc, 300, 300, EditorOptions{})
	check := func(what string) {
		t.Helper()
		ra, rb := bw.bounds(a), bw.bounds(b)
		if pa, pb := bw.box(t, 1), bw.box(t, 3); !near(ra.Y, pa.Top) || !near(rb.Y, pb.Top) || pb.Height != b.h {
			t.Fatalf("%s: the elements are at %v and %v, their paragraphs at %+v and %+v", what, ra, rb, pa, pb)
		}
		if mid := bw.box(t, 2); mid.Top < ra.Y+ra.H || rb.Y < mid.Top+mid.Height {
			t.Fatalf("%s: the paragraph between them is at %+v, they are at %v and %v", what, mid, ra, rb)
		}
	}
	check("built")
	was := bw.bounds(b).Y
	a.h = 90
	bw.frame(t, "the first grown")
	check("the first grown")
	if got := bw.bounds(b).Y; !near(got, was+50) {
		t.Fatalf("the first grew by 50, the second went from %v to %v", was, got)
	}
}

// A block kept built out of view is measured there: when it grows above the
// view, nothing in view moves, and the text keeps its height.
func TestBlockKeptGrowsOutOfView(t *testing.T) {
	cd := newCard("Kept", 40)
	cd.at.KeepBuilt = true
	paras := []Paragraph{para("Top"), blockPara(cd.at)}
	for i := range 300 {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	doc := NewDocument(TextOf(paras...))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	doc.ScrollBy(2000)
	bw.tt.Frame()
	v := bw.view(t)
	if v.first <= 1 {
		t.Fatalf("the view starts at paragraph %d", v.first)
	}
	first := bw.box(t, v.first)
	cd.builds = 0
	cd.h = 100
	bw.frame(t, "grown out of view")
	if cd.builds == 0 {
		t.Fatal("the kept block was not built out of view")
	}
	if _, ok := bw.tt.Find("Kept"); ok {
		t.Fatal("the kept block above the view shows")
	}
	if got := bw.box(t, first.Index); got != first {
		t.Fatalf("the block grew out of view, and the first paragraph in view went from %+v to %+v", first, got)
	}
	if pv := v.paras[1]; pv.h-pv.space != 100 {
		t.Fatalf("the block's paragraph is %v high, its element 100", pv.h-pv.space)
	}
	// Scrolled back, it is where its paragraph is.
	doc.ScrollBy(-1e6)
	bw.tt.Frame()
	if r, blk := bw.bounds(cd), bw.box(t, 1); !near(r.Y, blk.Top) || blk.Height != 100 {
		t.Fatalf("scrolled back, the element is at %v, its paragraph at %+v", r, blk)
	}
}

// While the editor keeps the line being typed where it is, blocks above it
// grow up, away from it, in the frame they grow in: the text between them
// and the line stays, and what is above them moves up with them.
func TestBlockAboveTheCaretGrowsUp(t *testing.T) {
	a, b := newCard("A", 40), newCard("B", 40)
	var paras []Paragraph
	for i := range 30 {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	// 30 A, 31, 32 B, 33, 34 the caret's.
	paras = append(paras, blockPara(a.at), para("Between"), blockPara(b.at), para("Below"), para("Typing here"))
	for i := range 30 {
		paras = append(paras, para(fmt.Sprintf("After %d", i)))
	}
	doc := NewDocument(TextOf(paras...))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	typing := doc.Text().paraStart(34) + 3
	bw.focus(typing, typing)
	doc.ScrollToParagraph(29, ui.ScrollStart)
	bw.tt.Frame()
	v := bw.view(t)
	if !v.anchorText || v.anchor != 34 {
		t.Fatalf("the editor keeps paragraph %d, by its text %v", v.anchor, v.anchorText)
	}
	line, below, between := bw.box(t, 34), bw.box(t, 33), bw.box(t, 31)
	ra, rb := bw.bounds(a), bw.bounds(b)
	b.h = 70
	bw.frame(t, "the lower grown")
	if got := bw.box(t, 34); got != line {
		t.Fatalf("the block above it grew, and the caret's line went from %+v to %+v", line, got)
	}
	if got := bw.box(t, 33); got != below {
		t.Fatalf("the paragraph between the block and the line went from %+v to %+v", below, got)
	}
	if got := bw.bounds(b); !near(got.Y+got.H, rb.Y+rb.H) || !near(got.Y, rb.Y-30) || !near(got.Y, bw.box(t, 32).Top) {
		t.Fatalf("the block grew from %v to %v, its paragraph at %+v", rb, got, bw.box(t, 32))
	}
	if got := bw.box(t, 31); !near(got.Top, between.Top-30) {
		t.Fatalf("the paragraph above the block went from %+v to %+v", between, got)
	}
	if got := bw.bounds(a); !near(got.Y, ra.Y-30) || !near(got.Y, bw.box(t, 30).Top) {
		t.Fatalf("the block above it went from %v to %v, its paragraph at %+v", ra, got, bw.box(t, 30))
	}
	a.h = 20
	bw.frame(t, "the upper shrunk")
	if got := bw.box(t, 34); got != line {
		t.Fatalf("the upper block shrank, and the caret's line went from %+v to %+v", line, got)
	}
	if got := bw.bounds(a); !near(got.Y, bw.box(t, 30).Top) || got.H != 20 {
		t.Fatalf("the upper block is at %v, its paragraph at %+v", got, bw.box(t, 30))
	}
}

// An editor as tall as its text, in a scroll container, builds the blocks
// inside the painter's clip, and a screen around it, not the others.
func TestBlocksBuiltInTheClip(t *testing.T) {
	var cards []*card
	var paras []Paragraph
	for i := range 100 {
		cd := newCard(fmt.Sprint("Card ", i), 40)
		cards = append(cards, cd)
		paras = append(paras, blockPara(cd.at))
		for k := range 5 {
			paras = append(paras, para(fmt.Sprintf("Paragraph %d.%d", i, k)))
		}
	}
	doc := NewDocument(TextOf(paras...))
	var scroll ui.ScrollState
	var c0 *ui.Context
	tt := openTester(t, func(c *ui.Context) {
		c0 = c
		ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
			ed := EditorBase(c, doc, EditorOptions{NoScrollbar: true, Font: ui.Font{Size: 10}}).FillWidth()
			h, ok := doc.ContentHeight()
			if !ok {
				h = 100 * 100
			}
			ed.Height(h)
		})
	}, 300, 300)
	tt.Frame()
	tt.Frame()
	built := func(what string) (lo, hi int) {
		t.Helper()
		for _, cd := range cards {
			cd.builds = 0
		}
		tt.Frame()
		lo, hi = -1, -1
		for i, cd := range cards {
			if cd.builds == 0 {
				continue
			}
			if lo < 0 {
				lo = i
			}
			if hi >= 0 && hi != i-1 {
				t.Fatalf("%s: cards %d and %d built, not those between", what, hi, i)
			}
			hi = i
			// Within the clip and a screen above and below it.
			if r := cd.handle.Bounds(c0); r.Y+r.H < -300 || r.Y > 600 {
				t.Fatalf("%s: card %d is built at %v, out of the clip", what, i, r)
			}
		}
		if lo < 0 {
			t.Fatalf("%s: no card built", what)
		}
		return lo, hi
	}
	if lo, hi := built("at the top"); lo != 0 || hi > 20 {
		t.Fatalf("at the top, cards %d to %d built", lo, hi)
	}
	tt.Wheel(150, 150, 0, 5000)
	tt.Advance(2e9)
	if lo, hi := built("scrolled"); lo == 0 || hi-lo > 20 {
		t.Fatalf("scrolled, cards %d to %d built", lo, hi)
	}
	// Each one built is where its paragraph is.
	v := doc.shownIn.Value()
	for _, bb := range v.built {
		r := bb.b.box.Bounds(c0)
		er := v.rect
		_, y := v.origin(bb.p)
		if !near(r.Y, er.Y+float32(y-v.scroll)) {
			t.Fatalf("the block of paragraph %d is at %v, its paragraph at %v", bb.p, r.Y, er.Y+float32(y-v.scroll))
		}
	}
}

// A block's paragraph is as high as its attachment says until its element
// is laid out, and then as high as the element last was, built or not.
func TestBlockHeightOutOfView(t *testing.T) {
	cd := newCard("Card", 40)
	cd.at.Height = 120
	var paras []Paragraph
	for i := range 100 {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	paras = append(paras, blockPara(cd.at))
	doc := NewDocument(TextOf(paras...))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	height := func() float32 { pv := bw.view(t).paras[100]; return pv.h - pv.space }
	// Laid out to scroll to it, before a frame builds its element.
	doc.ScrollToParagraph(100, ui.ScrollEnd)
	if h := height(); h != 120 {
		t.Fatalf("before its element was laid out, the block is %v high, its attachment says 120", h)
	}
	bw.tt.Frame()
	if h := height(); h != 40 || cd.builds == 0 {
		t.Fatalf("built %d times, the block is %v high, its element 40", cd.builds, h)
	}
	doc.ScrollToParagraph(0, ui.ScrollStart)
	bw.tt.Frame()
	cd.builds = 0
	bw.tt.SetSize(280, 200) // every paragraph is laid out anew
	if cd.builds != 0 {
		t.Fatalf("out of view, the block was built %d times", cd.builds)
	}
	doc.ScrollToParagraph(100, ui.ScrollEnd)
	if h := height(); h != 40 {
		t.Fatalf("out of view, the block is %v high, its element was 40", h)
	}
}

// An editor whose text has no blocks builds nothing more for them: no
// element in it, and no layout of its paragraphs as it builds.
func TestNoBlocksNoCost(t *testing.T) {
	for _, blocks := range []bool{false, true} {
		paras := []Paragraph{para("Before"), blockPara(&Attachment{Name: "Image", Width: 40, Height: 40}), para("After")}
		if blocks {
			paras = append(paras, blockPara(newCard("Card", 40).at))
		}
		doc := NewDocument(TextOf(paras...))
		building, laid := false, 0
		var c0 *ui.Context
		tt := openTester(t, func(c *ui.Context) {
			c0 = c
			building = true
			EditorBase(c, doc, EditorOptions{Space: func(int) float32 {
				if building {
					laid++
				}
				return 0
			}}).Fill()
			building = false
		}, 300, 200)
		tt.Press(2, 2)
		tt.Release(2, 2)
		tt.Type("typed ")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		v := doc.shownIn.Value()
		switch {
		case !blocks && (laid != 0 || len(v.blockParas) != 0 || v.layer.Bounds(c0) != ui.Rect{}):
			t.Fatalf("without blocks, %d paragraphs laid out as the editor built, blocks %v, a layer at %v", laid, v.blockParas, v.layer.Bounds(c0))
		case blocks && (laid == 0 || len(v.blockParas) != 1):
			t.Fatalf("with a block, %d paragraphs laid out as the editor built, blocks %v", laid, v.blockParas)
		}
	}
}

// The caret steps over a block as over a line, is a line high beside it,
// before it at its top and after it at its bottom.
func TestCaretStepsOverABlock(t *testing.T) {
	cd := newCard("Card", 60)
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	bw.focus(3, 3)
	key := func(k ui.Key) int {
		t.Helper()
		bw.tt.Key(0, k)
		_, caret := doc.Selection()
		return caret
	}
	caret := func() ui.Rect {
		r, ok := bw.view(t).caretRect()
		if !ok {
			t.Fatal("the caret is out of view")
		}
		return r
	}
	if got := key(ui.KeyDown); got != 7 && got != 8 {
		t.Fatalf("down from the first line, the caret is at %d, the block at 7", got)
	}
	doc.SetSelection(7, 7)
	bw.tt.Frame()
	r, blk := caret(), bw.bounds(cd)
	if r.H >= 30 || !near(r.Y, blk.Y) {
		t.Fatalf("before the block %v, the caret is %v", blk, r)
	}
	if got := key(ui.KeyRight); got != 8 {
		t.Fatalf("right from before the block, the caret is at %d", got)
	}
	if r := caret(); r.H >= 30 || !near(r.Y+r.H, blk.Y+blk.H) {
		t.Fatalf("after the block %v, the caret is %v", blk, r)
	}
	if got := key(ui.KeyDown); got < 9 {
		t.Fatalf("down from the block, the caret is at %d", got)
	}
	if got := key(ui.KeyUp); got != 7 && got != 8 {
		t.Fatalf("up to the block, the caret is at %d", got)
	}
	if got := key(ui.KeyUp); got > 6 {
		t.Fatalf("up from the block, the caret is at %d", got)
	}
}

// Undo brings a block deleted back, with its element.
func TestUndoBringsBackABlock(t *testing.T) {
	cd := newCard("Card", 40)
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	bw.focus(7, 8)
	bw.tt.Key(0, ui.KeyBackspace)
	cd.builds = 0
	bw.tt.Frame()
	if _, ok := bw.tt.Find("Card"); ok || cd.builds != 0 || doc.Text().String() != "Before\n\nAfter" {
		t.Fatalf("deleted, the block was built %d times, and the text is %q", cd.builds, doc.Text().String())
	}
	bw.tt.Key(ui.Cmd, ui.KeyZ)
	bw.tt.Frame()
	if r, ok := bw.tt.Find("Card"); !ok || cd.builds == 0 || r.H != 40 || !near(r.Y, bw.box(t, 1).Top) {
		t.Fatalf("undone, the block was built %d times, at %v", cd.builds, r)
	}
}

// longBlockDoc returns n paragraphs, the card's block after the first few,
// and the paragraph of the block.
func longBlockDoc(cd *card, n int) (*Document, int) {
	paras := []Paragraph{para("Before"), para("Above")}
	paras = append(paras, blockPara(cd.at))
	for i := range n {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	return NewDocument(TextOf(paras...)), 2
}

// A press on a block selects its attachment, without scrolling to it or
// dragging a selection from it, and goes on to what it presses.
func TestBlockPressSelectsIt(t *testing.T) {
	cd := newCard("Card", 60)
	clicks := 0
	cd.inside = func(c *ui.Context) {
		if ui.ButtonBase(c).Size(80, 20).Label("Press me").Clicked() {
			clicks++
		}
	}
	doc, p := longBlockDoc(cd, 100)
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	start := doc.Text().paraStart(p)
	doc.ScrollToParagraph(p, ui.ScrollStart)
	bw.tt.Frame()
	doc.ScrollBy(30) // the top of the block out of view
	bw.tt.Frame()
	r := bw.bounds(cd)
	if r.Y >= 0 || r.Y+r.H <= 0 {
		t.Fatalf("the block is at %v", r)
	}
	first := bw.box(t, p)
	bw.tt.Press(200, r.Y+r.H-5)
	bw.tt.Move(200, 150)
	bw.tt.Release(200, 150)
	if a, z := doc.Selection(); a != start || z != start+1 {
		t.Fatalf("pressed and dragged from the block, the selection is %d to %d, the attachment %d", a, z, start)
	}
	if got := bw.box(t, p); got != first || clicks != 0 {
		t.Fatalf("pressed, the block went from %+v to %+v, the button clicked %d times", first, got, clicks)
	}
	if !bw.view(t).focused {
		t.Fatal("pressed, the editor has no focus")
	}
	doc.SetSelection(0, 0)
	doc.ScrollToParagraph(p, ui.ScrollStart)
	bw.tt.Frame()
	if err := bw.tt.Click("Press me"); err != nil {
		t.Fatal(err)
	}
	if a, z := doc.Selection(); clicks != 1 || a != start || z != start+1 {
		t.Fatalf("the button clicked %d times, the selection is %d to %d", clicks, a, z)
	}
}

// A right-click on a block selects its attachment and opens the editor's
// menu.
func TestBlockRightClickOpensTheMenu(t *testing.T) {
	cd := newCard("Card", 60)
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{Menu: func(m *ui.Menu) { m.Item("The app's") }})
	r := bw.bounds(cd)
	bw.tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	if a, z := doc.Selection(); a != 7 || z != 8 {
		t.Fatalf("right-clicked, the selection is %d to %d", a, z)
	}
	if m := bw.tt.Menu(); len(m) != 1 || m[0] != "The app's" {
		t.Fatalf("right-clicked, the menu is %q", m)
	}
}

// A press over the scroll bar, where a block is under it, takes the bar's
// thumb, as over the text.
func TestBlockUnderTheScrollBar(t *testing.T) {
	cd := newCard("Card", 60)
	clicks := 0
	cd.inside = func(c *ui.Context) {
		if ui.ButtonBase(c).Fill().Label("Press me").Clicked() {
			clicks++
		}
	}
	doc, p := longBlockDoc(cd, 200)
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	r := bw.bounds(cd)
	if r.X+r.W < 299 {
		t.Fatalf("the block is at %v, not under the scroll bar", r)
	}
	bw.tt.Move(297, r.Y+r.H/2)
	if c := bw.tt.Cursor(); c != ui.CursorDefault {
		t.Fatalf("over the scroll bar, the cursor is %v", c)
	}
	bw.tt.Press(297, r.Y+r.H/2)
	v := bw.view(t)
	if !v.thumb {
		t.Fatal("pressed over the scroll bar, the thumb is not held")
	}
	bw.tt.Move(297, r.Y+r.H/2+50)
	bw.tt.Release(297, r.Y+r.H/2+50)
	if v.scroll <= 0 || clicks != 0 {
		t.Fatalf("the thumb dragged, the text is at %v, the button clicked %d times", v.scroll, clicks)
	}
	if a, _ := doc.Selection(); a == doc.Text().paraStart(p) {
		t.Fatal("the press on the scroll bar selected the block")
	}
}

// The wheel over a block scrolls the text, and goes on to the containers
// around the editor at its end.
func TestBlockWheel(t *testing.T) {
	cd := newCard("Card", 60)
	doc, _ := longBlockDoc(cd, 200)
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	r := bw.bounds(cd)
	bw.tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, 40)
	if v := bw.view(t); v.scroll != 40 {
		t.Fatalf("the wheel over the block scrolled the text to %v", v.scroll)
	}

	short := newCard("Short", 60)
	doc = NewDocument(TextOf(para("Before"), blockPara(short.at), para("After")))
	var scroll ui.ScrollState
	var c0 *ui.Context
	tt := openTester(t, func(c *ui.Context) {
		c0 = c
		ui.Scroll(c).Height(200).TrackScroll(&scroll).Children(func() {
			EditorBase(c, doc).FillWidth().Height(600)
		})
	}, 300, 400)
	tt.Frame()
	r = short.handle.Bounds(c0)
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, 40)
	if scroll.Y == 0 {
		t.Fatal("the wheel over the block of an editor that does not scroll scrolled nothing around it")
	}
}

// focusedCard is a card whose element takes the focus, and reads whether
// it has it.
func focusedCard(bw **blockWorld, cd *card) func() bool {
	return func() bool { return cd.handle.Focused((*bw).c) }
}

// Return, ⌘Return and Tab on a block selected alone go into its element,
// after the app had its say; Escape comes back with it selected. With the
// caret after a block, Return starts a paragraph.
func TestBlockKeysGoIn(t *testing.T) {
	cd := newCard("Card", 60)
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	takeEnter := false
	var bw *blockWorld
	bw = newBlockWorld(t, doc, 300, 200, EditorOptions{OnKey: func(k ui.Key, _ ui.Modifiers) bool {
		return takeEnter && k == ui.KeyEnter
	}})
	inCard := focusedCard(&bw, cd)
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{{0, ui.KeyEnter}, {ui.Cmd, ui.KeyEnter}, {0, ui.KeyTab}} {
		bw.focus(7, 8)
		bw.tt.Key(k.mods, k.key)
		if !inCard() || bw.view(t).focused {
			t.Fatalf("%v %v on the block: the element has the focus %v", k.mods, k.key, inCard())
		}
		bw.tt.Key(0, ui.KeyEscape)
		if a, z := doc.Selection(); inCard() || !bw.view(t).focused || a != 7 || z != 8 {
			t.Fatalf("Escape from the element: the editor has the focus %v, the selection is %d to %d", bw.view(t).focused, a, z)
		}
	}
	takeEnter = true
	bw.focus(7, 8)
	bw.tt.Key(0, ui.KeyEnter)
	if inCard() {
		t.Fatal("the app took Return, and the element has the focus")
	}
	takeEnter = false
	bw.focus(8, 8)
	bw.tt.Key(0, ui.KeyEnter)
	if inCard() || doc.Text().String() != "Before\n￼\n\nAfter" {
		t.Fatalf("Return after the block: the element has the focus %v, the text is %q", inCard(), doc.Text().String())
	}
}

// What has the focus in a block takes Escape first, when it handles it;
// Escape comes back to the text from a focus a press put there too.
func TestBlockEscapeInside(t *testing.T) {
	cd := newCard("Card", 60)
	takes, escapes := true, 0
	var field ui.Handle
	cd.inside = func(c *ui.Context) {
		f := ui.Box(c).Size(80, 20).Focusable().Label("Field").Bind(&field)
		if takes && f.Shortcut(0, ui.KeyEscape) {
			escapes++
		}
	}
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	bw.focus(0, 0)
	if err := bw.tt.Click("Field"); err != nil {
		t.Fatal(err)
	}
	if !field.Focused(bw.c) {
		t.Fatal("clicked, the field has no focus")
	}
	bw.tt.Key(0, ui.KeyEscape)
	if escapes != 1 || !field.Focused(bw.c) {
		t.Fatalf("the field took Escape %d times, and has the focus %v", escapes, field.Focused(bw.c))
	}
	takes = false
	bw.tt.Frame()
	bw.tt.Key(0, ui.KeyEscape)
	if a, z := doc.Selection(); field.Focused(bw.c) || !bw.view(t).focused || a != 7 || z != 8 {
		t.Fatalf("Escape the field left: the editor has the focus %v, the selection is %d to %d", bw.view(t).focused, a, z)
	}
}

// A block deleted while it has the focus gives it back to the text.
func TestBlockDeletedWithTheFocus(t *testing.T) {
	cd := newCard("Card", 60)
	doc := NewDocument(TextOf(para("Before"), blockPara(cd.at), para("After")))
	var bw *blockWorld
	bw = newBlockWorld(t, doc, 300, 200, EditorOptions{})
	inCard := focusedCard(&bw, cd)
	bw.focus(7, 8)
	bw.tt.Key(0, ui.KeyEnter)
	if !inCard() {
		t.Fatal("Return on the block: the element has no focus")
	}
	doc.InsertText("")
	bw.tt.Frame()
	v := bw.view(t)
	if !v.focused || v.focusBlock != 0 || doc.Text().String() != "Before\n\nAfter" {
		t.Fatalf("the block deleted, the editor has the focus %v, the block's %d, the text %q", v.focused, v.focusBlock, doc.Text().String())
	}
	bw.tt.Type("x")
	if doc.Text().String() != "Before\nx\nAfter" {
		t.Fatalf("typed after, the text is %q", doc.Text().String())
	}
}

// The focus coming into a block from Tab brings its paragraph into view,
// as the caret's does.
func TestBlockFocusComesIntoView(t *testing.T) {
	cd := newCard("Card", 60)
	cd.at.KeepBuilt = true
	cd.inside = func(c *ui.Context) { ui.Box(c).Size(80, 20).Focusable().Label("Field") }
	var paras []Paragraph
	for i := range 100 {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	paras = append(paras, blockPara(cd.at), para("After"))
	doc := NewDocument(TextOf(paras...))
	bw := newBlockWorld(t, doc, 300, 200, EditorOptions{})
	bw.focus(0, 0)
	if blk := bw.view(t).paras[100]; blk.h != 0 && bw.view(t).last >= 100 {
		t.Fatal("the block is in view")
	}
	bw.tt.Key(0, ui.KeyTab)
	if !bw.tt.Focused("Field") {
		t.Fatal("Tab did not go into the block")
	}
	if b := bw.box(t, 100); b.Top < 0 || b.Top+b.Height > 200 {
		t.Fatalf("the focus in the block, it is at %+v", b)
	}
	if r := bw.bounds(cd); !near(r.Y, bw.box(t, 100).Top) {
		t.Fatalf("the focus in the block, its element is at %v, its paragraph at %+v", r, bw.box(t, 100))
	}
}

// Assistive technology reads each block as a group named as its
// attachment is, inside the editor's text field, in the text's order,
// those above the line being typed and below it alike; a block kept built
// above the view is not read.
func TestBlocksForAssistiveTechnology(t *testing.T) {
	cards := map[string]*card{}
	block := func(name string) Paragraph {
		cards[name] = newCard(name, 40)
		return blockPara(cards[name].at)
	}
	paras := []Paragraph{block("Kept")}
	cards["Kept"].at.KeepBuilt = true
	for i := range 40 {
		paras = append(paras, para(fmt.Sprintf("Paragraph %d", i)))
	}
	// 41 A, 42, 43 B, 44 the caret's, 45 C.
	paras = append(paras, block("A"), para("Between"), block("B"), para("Typing here"), block("C"))
	for i := range 40 {
		paras = append(paras, para(fmt.Sprintf("After %d", i)))
	}
	doc := NewDocument(TextOf(paras...))
	bw := newBlockWorld(t, doc, 300, 300, EditorOptions{})
	typing := doc.Text().paraStart(44) + 3
	bw.focus(typing, typing)
	doc.ScrollToParagraph(40, ui.ScrollStart)
	bw.tt.Frame()
	if v := bw.view(t); !v.anchorText || v.anchor != 44 {
		t.Fatalf("the editor keeps paragraph %d, by its text %v", v.anchor, v.anchorText)
	}
	var read []string
	field := -1
	for i, a := range bw.tt.AccessAll() {
		switch a.Role {
		case ui.RoleTextField:
			field = i
		case ui.RoleGroup:
			if field < 0 {
				t.Fatalf("the group %q is read before the editor", a.Name)
			}
			read = append(read, a.Name)
			if r := bw.bounds(cards[a.Name]); a.Bounds != r {
				t.Errorf("the group %q is at %v, its element at %v", a.Name, a.Bounds, r)
			}
		}
	}
	if fmt.Sprint(read) != "[A B C]" {
		t.Fatalf("assistive technology reads the blocks %q", read)
	}
}
