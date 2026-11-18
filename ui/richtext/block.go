package richtext

import (
	"maps"
	"math"
	"slices"

	"github.com/egoist/mygo/ui"
)

// An attachment alone in its paragraph with an Element is a block: the
// paragraph shows the app's element in place of its line. The editor
// builds the elements in one layer over the text, a child of its own whose
// children are keyed by their paragraphs' ids, so that each element keeps
// its identity, and what it holds, as the text and the view change around
// it. It places them as it builds, from where the view keeps its place,
// the reference: the anchor's top, or the caret's line while the editor
// keeps it (anchorText). The blocks on one side of the reference are in
// the layer's flow, each after a spacer as tall as the text between them,
// so that the layout moves those after one that changes height in the
// same pass; the others are placed by their own edge nearest the
// reference. Painting reads the height each element was laid out at and
// gives its paragraph that height before it lays the text out, so that
// the text moves with the blocks in the frame they change in.
//
// The flow holds the blocks below the reference, and holds those above it
// instead when some of them show, as above the caret's line, where they
// hang from it by their bottoms: what grows there moves the text above it
// up, as the line being typed stays. The flow keeps the blocks it holds in
// the order of the text, ahead of the others, as assistive technology and
// Tab read them. Those above the reference out of view are measured but
// not shown.

// block is what the editor keeps of a block from frame to frame, by its
// paragraph's id.
type block struct {
	// box is the element the editor builds around the app's, elem.
	box, elem ui.Handle
	// h is the height the element was laid out at, once measured.
	h        float32
	measured bool
	// focused tells that the focus was in the block at the last build.
	focused bool
	// used is the build that last found the block in the text.
	used uint64
}

// placement is how the layer holds a block.
type placement uint8

const (
	flowing  placement = iota // in the layer's flow
	hanging                   // above the reference, in view: in the flow
	standing                  // below the reference, by its top, or in the flow
	hidden                    // above the reference out of view: measured only
)

// builtBlock is a block built in a frame, of paragraph p.
type builtBlock struct {
	p   int
	id  uint64
	b   *block
	how placement
}

// blockAttachment returns the attachment of the paragraph when it is one
// alone in it with an element to build, and nil otherwise.
func blockAttachment(para *richPara) *Attachment {
	if para.n != 1 || len(para.runs) == 0 {
		return nil
	}
	r := para.runs[0]
	if r.n != 1 || r.a.Attachment == nil || r.a.Attachment.Element == nil {
		return nil
	}
	return r.a.Attachment
}

// blockAt returns the attachment of paragraph p when the editor shows it
// as a block. A paragraph an input method composes in is none: what is
// composed shows where the element would.
func (v *view) blockAt(p int) *Attachment {
	if p < 0 || p >= len(v.paras) {
		return nil
	}
	at := blockAttachment(v.para(p))
	if at == nil || v.compose != "" && v.shown.paraAt(v.caret) == p {
		return nil
	}
	return at
}

// blockHeight returns how high the block of paragraph p is: as its element
// was laid out, or as its attachment says before it was, or a line.
func (v *view) blockHeight(p int, at *Attachment) float32 {
	if b := v.blocks[v.para(p).id]; b != nil && b.measured {
		return b.h
	}
	if at.Height > 0 {
		return at.Height
	}
	return v.look.size * 1.3
}

// blockFor returns what the editor keeps of the block of the paragraph
// with id, made the first time.
func (v *view) blockFor(id uint64) *block {
	b := v.blocks[id]
	if b == nil {
		if v.blocks == nil {
			v.blocks = map[uint64]*block{}
		}
		b = &block{}
		v.blocks[id] = b
	}
	return b
}

// findBlocks appends the paragraphs from a to z of t that are blocks to
// out.
func findBlocks(out []int, t Text, a, z int) []int {
	seqEach(t.seq(), a, func(i int, p *richPara) bool {
		if i > z {
			return false
		}
		if blockAttachment(p) != nil {
			out = append(out, i)
		}
		return true
	})
	return out
}

// replaceBlocks returns the paragraphs of blocks bp of a text whose
// paragraphs from pa to pz gave way to those from pa to npz of now.
func replaceBlocks(bp []int, pa, pz, npz int, now Text) []int {
	found := findBlocks(nil, now, pa, npz)
	i, _ := slices.BinarySearch(bp, pa)
	j, _ := slices.BinarySearch(bp, pz+1)
	if len(found) == 0 && i == j && (npz == pz || j == len(bp)) {
		return bp
	}
	out := make([]int, 0, i+len(found)+len(bp)-j)
	out = append(append(out, bp[:i]...), found...)
	for _, p := range bp[j:] {
		out = append(out, p+npz-pz)
	}
	return out
}

// blockResized gives paragraph p, laid out as a block, the height of its
// element.
func (v *view) blockResized(p int, at *Attachment) {
	pv := &v.paras[p]
	if pv.h <= 0 || !pv.block {
		return
	}
	if h := pv.space + v.blockHeight(p, at); h != pv.h {
		v.hs.add(p, float64(h-pv.h), 0)
		pv.h = h
	}
}

// shows returns what of the content shows: the view, within what the
// painter's clip showed of it.
func (v *view) shows() (lo, hi float64) {
	lo, hi = v.scroll, v.scroll+float64(v.viewH)
	if v.clipped {
		lo, hi = max(lo, v.scroll+float64(v.clip[0])), min(hi, v.scroll+float64(v.clip[1]))
	}
	return lo, hi
}

// paraInView reports whether paragraph p is in what shows, from lo to hi.
func (v *view) paraInView(p int, lo, hi float64) bool {
	return p >= v.first && p <= v.last && v.hs.top(p+1) >= lo && v.hs.top(p) <= hi
}

// buildBlocks builds the elements of the blocks in view, and of those
// kept built, in a layer over the text, where their paragraphs are.
func (v *view) buildBlocks(c *ui.Context, e ui.Element) {
	v.built = v.built[:0]
	v.stripBuilt = false
	v.gen++
	focusBlock := v.focusBlock
	v.focusBlock = 0
	if len(v.blockParas) > 0 && v.width > 0 {
		v.c = c
		v.placeBlocks(c, e)
	}
	if focusBlock != 0 && !slices.ContainsFunc(v.built, func(bb builtBlock) bool { return bb.id == focusBlock }) {
		// The block with the focus went from the text, and what had it with
		// it: the focus comes back to the text rather than going nowhere.
		if b := v.blocks[focusBlock]; b != nil && b.box.FocusWithin(c) {
			v.takeFocus(c, e)
		}
	}
	v.pressedBlock = false
}

// takeFocus gives the editor e the focus back from a block as it builds.
func (v *view) takeFocus(c *ui.Context, e ui.Element) {
	e.Focus(ui.FocusOptions{PreventScroll: true})
	v.focused = true
	v.blinkStart = c.Now()
}

// followFocus notes the blocks the focus is in, and returns the paragraph
// of one it came into from the keyboard or the app since the last build,
// or -1. It ends a focus the editor gave a block (enterBlock) once the
// block has it, or a build after it could not take it.
func (v *view) followFocus(c *ui.Context, e ui.Element) int {
	inside := e.FocusWithin() && !e.Focused()
	arrived := -1
	for _, p := range v.blockParas {
		id := v.para(p).id
		b := v.blocks[id]
		if b == nil {
			continue
		}
		was := b.focused
		b.focused = inside && b.box.FocusWithin(c)
		if b.focused {
			v.focusBlock = id
			if !was && !v.pressedBlock && arrived < 0 {
				arrived = p
			}
		}
	}
	if v.focusing != 0 {
		if b := v.blocks[v.focusing]; b == nil || b.focused || v.focusAsked {
			if b != nil && !b.focused {
				b.elem.CancelFocus(c)
			}
			v.focusing, v.focusAsked = 0, false
		}
	}
	return arrived
}

// placeBlocks finds the blocks to build and builds them.
func (v *view) placeBlocks(c *ui.Context, e ui.Element) {
	// Where the paragraphs are, after the edits and scrolls since the last
	// paint. The paint lays them out again at its width, and reveals the
	// caret there.
	reveal := v.reveal
	v.layout(v.width, v.viewH)
	if p := v.followFocus(c, e); p >= 0 && v.scrollToPara(p, ui.ScrollNearest) {
		// The focus came into a block: its paragraph comes into view, as
		// the caret's does, unless a press there gave it.
		v.layout(v.width, v.viewH)
	}
	v.reveal = reveal
	ref := v.hs.top(v.anchor)
	if v.anchorText {
		ref = v.anchorY
	}
	lo, hi := v.shows()
	up := false
	for _, p := range v.blockParas {
		at := v.blockAt(p)
		if at == nil {
			continue
		}
		id := v.para(p).id
		b := v.blocks[id]
		if b != nil {
			b.used = v.gen
		}
		shows := v.paraInView(p, lo, hi)
		// What has the focus is built, and shown, wherever it is, so that it
		// keeps it.
		focused := b != nil && b.focused || id == v.focusing
		if !shows && !at.KeepBuilt && !focused {
			continue
		}
		b = v.blockFor(id)
		b.used = v.gen
		if id == v.focusing {
			v.focusAsked = true
		}
		how := standing
		if p < v.anchor {
			how = hanging
			if !shows && !focused {
				how = hidden
			}
		}
		up = up || how == hanging
		v.built = append(v.built, builtBlock{p: p, id: id, b: b, how: how})
	}
	if len(v.blocks) > 2*len(v.blockParas)+64 {
		// What the blocks deleted kept, for undo to bring back, until
		// there is much of it.
		maps.DeleteFunc(v.blocks, func(_ uint64, b *block) bool { return b.used != v.gen })
	}
	// The scroll bar takes the pointer over the blocks, while there are
	// any in the text, as a press on it holds the thumb whatever comes into
	// view.
	strip := v.barShown()
	if len(v.built) == 0 && !strip {
		return
	}
	for i := range v.built {
		bb := &v.built[i]
		if up && bb.how == hanging || !up && bb.how == standing {
			bb.how = flowing
		}
	}
	e.Children(func() {
		if len(v.built) > 0 {
			v.buildLayer(c, e, ref, up)
		}
		if strip {
			v.buildStrip(c)
		}
	})
}

// buildLayer builds the layer of the blocks, its flow from the reference
// ref of the content: down from it, or up to it when up is set.
func (v *view) buildLayer(c *ui.Context, e ui.Element, ref float64, up bool) {
	in := v.insideBorder()
	left, right := v.padX-in[3], v.padX-in[1]
	top := v.padY - in[0] + float32(ref-v.scroll)
	// above is the room the flow hangs in above the reference, when it
	// does: twice what it holds, and more, so that what grows there grows
	// up into it.
	var above float32
	if up {
		for _, bb := range v.built {
			if bb.how == flowing {
				_, y := v.origin(bb.p)
				above = 2*float32(ref-y) + 4096
				break
			}
		}
	}
	layer := ui.Box(c).Key("richtext blocks").Absolute().Left(left).Right(right).Column().AlignItems(ui.Stretch)
	if up {
		layer.Top(top - above).Height(above).Justify(ui.End)
	} else {
		layer.Top(top).Height(0)
	}
	layer.Bind(&v.layer)
	v.layerAt = [3]float32{left, top - above, right}
	layer.Children(func() {
		cursor, spacers, flowed := ref, 0, false
		spacer := func(to float64) {
			if gap := float32(to - cursor); gap > 0 {
				ui.Box(c).Key(spacers).Height(gap).Shrink(0)
				spacers++
			}
		}
		for _, bb := range v.built {
			p := bb.p
			at := blockAttachment(v.para(p))
			l, r := v.insets(v.para(p).style)
			_, y := v.origin(p)
			h := v.blockHeight(p, at)
			if bb.how == flowing {
				if up && !flowed {
					cursor = y
				}
				spacer(y) // before the block, as the layer's children go
				flowed = true
				cursor = y + float64(h)
			}
			box := ui.Box(c).Key(bb.id)
			switch bb.how {
			case flowing:
				box.Shrink(0).Margin(0, r, 0, l)
			default:
				box.Absolute().Left(l).Right(r).Top(above + float32(y-ref))
				if bb.how == hidden {
					box.Invisible()
				}
			}
			id := bb.id
			// A press on it goes on to what it presses; the wheel over it,
			// which it leaves, scrolls the text.
			box.Cursor(ui.CursorDefault).HandleInput(func(ev ui.InputEvent) bool { return v.blockInput(id, ev) })
			if box.Shortcut(0, ui.KeyEscape) {
				// Escape in the element, unless what has the focus in it
				// takes it first, brings the focus back to the text.
				v.leaveBlock(c, e, p)
			}
			box.Bind(&bb.b.box)
			w := max(v.width-l-r, 1)
			box.Children(func() { at.Element(c, w).Bind(&bb.b.elem) })
		}
		if up {
			spacer(ref)
		}
	})
}

// insideBorder returns the editor's border, which absolute insets are
// measured inside: as the layer's box measured it, or as the editor has
// it before.
func (v *view) insideBorder() [4]float32 {
	if v.borderKnown {
		return v.border
	}
	var b float32
	if v.styled {
		b = 1
	}
	return [4]float32{b, b, b, b}
}

// barShown reports whether the text is taller than the view, with a scroll
// bar of the editor's.
func (v *view) barShown() bool {
	return !v.opts.NoScrollbar && v.hs.top(len(v.paras)) > float64(v.viewH)
}

// buildStrip builds what takes the pointer over the editor's scroll bar,
// above the blocks: a press there takes the bar's thumb, as it does over
// the text.
func (v *view) buildStrip(c *ui.Context) {
	width := v.barStyle.TrackWidth
	if width <= 0 {
		width = scrollbarWidth
	}
	in := v.insideBorder()
	ui.Box(c).Key("richtext bar").Absolute().Top(-in[0]).Bottom(-in[0]).Right(-in[1]).Width(width).
		Cursor(ui.CursorDefault).HandleInput(v.stripInput).Bind(&v.strip)
	v.stripBuilt = true
}

// stripInput gives the editor the pointer over its scroll bar, in its own
// axes.
func (v *view) stripInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp:
		if v.c != nil {
			r := v.strip.Bounds(v.c)
			ev.X, ev.Y = ev.X+r.X-v.rect.X, ev.Y+r.Y-v.rect.Y
		}
		return v.input(ev)
	}
	return false
}

// blockPara returns the paragraph of the block with id, and whether it is
// in the text.
func (v *view) blockPara(id uint64) (int, bool) {
	for _, p := range v.blockParas {
		if v.para(p).id == id {
			return p, true
		}
	}
	return 0, false
}

// blockInput takes what comes to the box of the block with id: a press
// selects its attachment, without scrolling to it or dragging a selection
// from it, and goes on to what it presses, as a button, or to the editor's
// menu. The rest goes on as it would: the wheel to the editor.
func (v *view) blockInput(id uint64, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputPointerDown {
		return false
	}
	v.sync()
	defer v.publish()
	p, ok := v.blockPara(id)
	if !ok {
		return false
	}
	v.pressedBlock = true
	v.commitCompose()
	start := v.shown.paraStart(p)
	a, z := v.selection()
	switch {
	case ev.Button == 1 && a <= start && z > start:
		// A right-click in the selection keeps it, for the menu.
	case ev.Button == 0 && ev.Mods&ui.Shift != 0:
		if start >= v.selAnchor {
			v.caret = start + 1
		} else {
			v.caret = start
		}
	default:
		v.selAnchor, v.caret = start, start+1
	}
	v.affinity = ui.TextDownstream
	v.hasDesired, v.coalesce, v.dragging = false, false, false
	v.blinkStart = ev.Time
	return false
}

// enterBlock gives the focus to the element of the block whose attachment
// alone is selected, scrolled into view, and reports whether there is one.
func (v *view) enterBlock() bool {
	a, z := v.selection()
	if z != a+1 || v.c == nil {
		return false
	}
	p := v.shown.paraAt(a)
	if v.shown.paraStart(p) != a || v.blockAt(p) == nil {
		return false
	}
	id := v.para(p).id
	v.focusing, v.focusAsked = id, false
	v.scrollToPara(p, ui.ScrollNearest)
	v.blockFor(id).elem.Focus(v.c)
	return true
}

// leaveBlock gives the editor e the focus back from the block of paragraph
// p, with its attachment selected, as it builds. The text stays where it
// is, as the block was where the focus left it.
func (v *view) leaveBlock(c *ui.Context, e ui.Element, p int) {
	start := v.shown.paraStart(p)
	v.selAnchor, v.caret, v.affinity = start, start+1, ui.TextDownstream
	v.hasDesired = false
	v.publish()
	v.takeFocus(c, e)
}

// measureBlocks gives the paragraphs of the blocks built in this frame the
// heights their elements were laid out at, before painting lays out the
// text in r, which then moves with them in this frame. It reports whether
// one changed.
func (v *view) measureBlocks(r ui.Rect) bool {
	if len(v.built) == 0 || v.c == nil {
		return false
	}
	if lb := v.layer.Bounds(v.c); lb != (ui.Rect{}) {
		// The editor's border, inside which the layer's insets are measured.
		v.border = [4]float32{
			lb.Y - r.Y - v.layerAt[1],
			r.X + r.W - lb.X - lb.W - v.layerAt[2],
			0,
			lb.X - r.X - v.layerAt[0],
		}
		v.borderKnown = true
	}
	changed := false
	for _, bb := range v.built {
		if bb.p >= len(v.paras) || v.para(bb.p).id != bb.id {
			continue // the text changed since it was built
		}
		box := bb.b.box.Bounds(v.c)
		if box == (ui.Rect{}) || bb.b.measured && box.H == bb.b.h {
			continue
		}
		bb.b.h, bb.b.measured = box.H, true
		if at := v.blockAt(bb.p); at != nil {
			v.blockResized(bb.p, at)
		}
		changed = true
	}
	return changed
}

// blocksMoved asks for another frame when painting in r placed the text
// otherwise than the blocks built for it: a block is not where its
// paragraph is, or one in view was not built, as when the width changed,
// a block above the caret's line pushed the text to the top of the
// content, or one changed the height of the text so that another came
// into view. resized tells that a block changed height, which an app that
// read ContentHeight sizes the editor to.
func (v *view) blocksMoved(r ui.Rect, resized bool) {
	if len(v.blockParas) == 0 && len(v.built) == 0 {
		return
	}
	moved := resized && v.readHeight
	ox, oy := r.X+v.padX, r.Y+v.padY
	if v.c != nil {
		for _, bb := range v.built {
			if bb.how == hidden {
				continue
			}
			if bb.p >= len(v.paras) || v.para(bb.p).id != bb.id {
				moved = true
				break
			}
			box := bb.b.box.Bounds(v.c)
			l, rr := v.insets(v.para(bb.p).style)
			_, y := v.origin(bb.p)
			if far(box.X, ox+l) || far(box.Y, oy+float32(y-v.scroll)) || far(box.W, max(v.width-l-rr, 1)) {
				moved = true
				break
			}
		}
	}
	if !moved && len(v.blockParas) > 0 && v.stripBuilt != v.barShown() {
		moved = true // the text grew taller than the view, or no more
	}
	if !moved {
		lo, hi := v.shows()
		i, _ := slices.BinarySearch(v.blockParas, v.first)
		for _, p := range v.blockParas[i:] {
			if p > v.last {
				break
			}
			if v.paraInView(p, lo, hi) && v.blockAt(p) != nil && !slices.ContainsFunc(v.built, func(bb builtBlock) bool { return bb.p == p }) {
				moved = true
				break
			}
		}
	}
	switch {
	case !moved:
		v.misplaced = 0
	case resized:
		// The blocks the frame after builds follow the heights this one
		// measured.
		v.misplaced = 0
		v.services.Invalidate()
	default:
		// A layout that cannot place the blocks where the text is, frame
		// after frame with nothing changing, gets no frames more for them.
		if v.misplaced++; v.misplaced <= 3 {
			v.services.Invalidate()
		}
	}
}

// far reports whether a and b are more than half a DIP apart.
func far(a, b float32) bool { return math.Abs(float64(a-b)) > 0.5 }
