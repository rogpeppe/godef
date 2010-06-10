package canvas

import (
	"exp/draw"
	"image"
	"freetype-go.googlecode.com/hg/freetype"
	"freetype-go.googlecode.com/hg/freetype/raster"
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
	rp   *raster.RGBAPainter
	gp   *raster.GammaCorrectionPainter
	cp   clippedPainter
}

func (d *TextItem) Init() *TextItem {
	d.Context = freetype.NewContext()
	d.rp = raster.NewRGBAPainter(nil)
	d.gp = raster.NewGammaCorrectionPainter(d.rp, gamma)
	d.cp.Painter = d.gp
	return d
}

func (d *TextItem) CalcBbox() {
	var bbox bboxPainter
	d.DrawText(&bbox, d.Pt, d.Text)
	d.bbox = bbox.R
}

func (d *TextItem) Opaque() bool {
	return false
}

func (d *TextItem) Draw(dst *image.RGBA, clip draw.Rectangle) {
	d.cp.Clipr = clip
	d.rp.Image = dst
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
	canvas Backing
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
	t.item.rp.SetColor(image.Black)
	t.item.Text = s
	t.p = p
	t.anchor = where
	t.recalc(true)
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
		t.canvas.Flush()
	}
}

func (t *Text) SetContainer(c Backing) {
	t.canvas = c
}

func (t *Text) SetCentre(cp draw.Point) {
	delta := cp.Sub(centre(t.Bbox()))
	t.SetPoint(t.p.Add(delta))
}

func (t *Text) SetPoint(p0 draw.Point) {
	t.canvas.Atomically(func(flush FlushFunc) {
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

func (t *Text) SetColor(col image.Color) {
	t.canvas.Atomically(func(flush FlushFunc) {
		t.item.rp.SetColor(col)
		flush(t.item.Bbox(), nil)
	})
}

func (t *Text) SetText(s string) {
	t.canvas.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.item.Text = s
		t.recalc(true)
		flush(r, nil)
		flush(t.item.Bbox(), nil)
	})
}

func (t *Text) SetFontSize(size float) {
	t.canvas.Atomically(func(flush FlushFunc) {
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
