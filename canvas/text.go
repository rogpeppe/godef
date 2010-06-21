package canvas

import (
	"exp/draw"
	"image"
	"freetype-go.googlecode.com/hg/freetype"
	"rog-go.googlecode.com/hg/freetype/raster"
	"freetype-go.googlecode.com/hg/freetype/truetype"
	"rog-go.googlecode.com/hg/values"
)

const (
	dpi   = 72
	gamma = 1
)

type TextItem struct {
	*freetype.Context
	Text string
	Pt   raster.Point
	bbox draw.Rectangle
	dst draw.Image
	fill image.Image

	// these three painters are arranged in a stack, with the
	// first at the bottom and the last at the top.
	rp   raster.Painter
	gp   *raster.GammaCorrectionPainter
	cp   clippedPainter
}

func (d *TextItem) Init() *TextItem {
	d.Context = freetype.NewContext()
	d.gp = raster.NewGammaCorrectionPainter(nil, gamma)
	d.cp.Painter = d.gp
	return d
}

func (d *TextItem) makePainter() {
	if d.dst != nil {
		d.rp = raster.NewPainter(d.dst, d.fill, draw.Over)
	}else{
		d.rp = nil
	}
	d.gp.Painter = d.rp
}

func (d *TextItem) CalcBbox() {
	var bbox bboxPainter
	d.DrawText(&bbox, d.Pt, d.Text)
	d.bbox = bbox.R
}

func (d *TextItem) SetFill(fill image.Image) {
	d.fill = fill
	d.makePainter()
}

func (d *TextItem) Opaque() bool {
	return false
}

func (d *TextItem) Draw(dst draw.Image, clip draw.Rectangle) {
	if dst != d.dst {
		d.dst = dst
		d.makePainter()
	}
	d.cp.Clipr = clip
	d.DrawText(&d.cp, d.Pt, d.Text)
}

func (d *TextItem) HitTest(p draw.Point) bool {
	var hit hitTestPainter
	hit.P = p
	d.DrawText(&hit, d.Pt, d.Text)
	return hit.Hit
}

func (d *TextItem) Bbox() draw.Rectangle {
	return d.bbox
}

func (d *TextItem) SetContainer(_ Backing) {
}

type Text struct {
	Item
	item   TextItem
	delta  draw.Point // vector from upper left of bbox to text origin
	p      draw.Point
	anchor Anchor
	backing Backing
	value values.Value
}

// NewText creates a new item to display a line of text.
// If val is non-nil, it should be a string-typed Value,
// and the Value's text will be displayed instead of s.
//
func NewText(p draw.Point, where Anchor, s string, font *truetype.Font, size float, val values.Value) *Text {
	t := new(Text)
	t.item.Init()
	t.item.SetFont(font)
	t.item.SetFontSize(size)
	t.item.fill = image.ColorImage{draw.Black}
	t.item.Text = s
	t.p = p
	t.anchor = where
	t.recalc(true)
	t.backing = NullBacking()
	t.Item = &t.item
	if val != nil {
		t.value = val
		go t.listener()
	}
	return t
}

func (t *Text) listener() {
	for x := range t.value.Iter() {
		t.SetText(x.(string))
		t.backing.Flush()
	}
}

func (t *Text) SetContainer(c Backing) {
	t.backing = c
}

func (t *Text) SetCentre(cp draw.Point) {
	delta := cp.Sub(centre(t.Bbox()))
	t.SetPoint(t.p.Add(delta))
}

func (t *Text) SetPoint(p0 draw.Point) {
	t.backing.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.p = p0
		t.recalc(false)
		flush(r, nil)
		flush(t.item.Bbox(), nil)
	})
}

// calculate bounding box and text origin.
func (t *Text) recalc(sizechanged bool) {
	var bbox draw.Rectangle
	if sizechanged {
		t.item.Pt = raster.Point{0, 0}
		t.item.CalcBbox()
		bbox = t.item.Bbox()
		t.delta = draw.Point{0, 0}.Sub(bbox.Min)
	} else {
		bbox = t.item.Bbox()
	}
	bbox = anchor(bbox, t.anchor, t.p)
	t.item.bbox = bbox
	t.item.Pt = pixel2fixPoint(bbox.Min.Add(t.delta))
}

func (t *Text) SetFill(fill image.Image) {
	t.backing.Atomically(func(flush FlushFunc) {
		t.item.SetFill(fill)
		flush(t.item.Bbox(), nil)
	})
}

func (t *Text) SetText(s string) {
	t.backing.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.item.Text = s
		t.recalc(true)
		flush(r, nil)
		flush(t.item.Bbox(), nil)
	})
}

func (t *Text) SetFontSize(size float) {
	t.backing.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.item.SetFontSize(size)
		t.recalc(true)
		flush(r, nil)
		flush(t.item.Bbox(), nil)
	})
}

type Anchor int

const (
	N = Anchor(1 << iota)
	S
	E
	W
	Baseline
)

func anchor(r draw.Rectangle, flags Anchor, p draw.Point) draw.Rectangle {
	var dp draw.Point
	switch flags & (E | W) {
	case E:
		dp.X = r.Dx()
	case E | W, 0:
		dp.X = r.Dx() / 2
	}
	switch flags & (N | S) {
	case S:
		dp.Y = r.Dy()
	case S | N, 0:
		dp.Y = r.Dy() / 2
	}
	return r.Add(p.Sub(r.Min).Sub(dp))
}
