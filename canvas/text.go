package canvas

import (
	"rog-go.googlecode.com/hg/draw"
	"image"
	"rog-go.googlecode.com/hg/freetype"
	"freetype-go.googlecode.com/hg/freetype/raster"
	"freetype-go.googlecode.com/hg/freetype/truetype"
)

const (
	dpi   = 72
	gamma = 1
)

type TextItem struct {
	freetype.Context
	Text string
	Pt   raster.Point
	bbox draw.Rectangle
	rp   *raster.RGBAPainter
	gp   *raster.GammaCorrectionPainter
	cp   clippedPainter
}

func (d *TextItem) Init() *TextItem {
	d.Context.Init()
	d.rp = raster.NewRGBAPainter(nil)
	d.gp = raster.NewGammaCorrectionPainter(d.rp, gamma)
	d.cp.Painter = d.gp
	return d
}

func (d *TextItem) CalcBbox() {
	var bbox bboxPainter
	d.DrawText(d.Pt, d.Text, &bbox)
	d.bbox = bbox.R
}

func (d *TextItem) Opaque() bool {
	return false
}

func (d *TextItem) Draw(dst *image.RGBA, clip draw.Rectangle) {
	d.cp.Clipr = clip
	d.rp.Image = dst
	d.DrawText(d.Pt, d.Text, &d.cp)
}

func (d *TextItem) HitTest(p draw.Point) bool {
	var hit hitTestPainter
	hit.P = p
	d.DrawText(d.Pt, d.Text, &hit)
	return hit.Hit
}

func (d *TextItem) Bbox() draw.Rectangle {
	return d.bbox
}

func (d *TextItem) SetContainer(_ *Canvas) {
}

type Text struct {
	Item
	item   TextItem
	delta  draw.Point // vector from upper left of bbox to text origin
	p      draw.Point
	anchor Anchor
	canvas *Canvas
}

func NewText(p draw.Point, where Anchor, s string, font *truetype.Font, size float) *Text {
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
	return t
}

func (t *Text) SetContainer(c *Canvas) {
	t.canvas = c
}

func (t *Text) Move(delta draw.Point) {
	t.SetPoint(t.p.Add(delta))
}

func (t *Text) SetPoint(p0 draw.Point) {
	t.canvas.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.p = p0
		t.recalc(false)
		flush(r, false, &t.item)
		flush(t.item.Bbox(), false, &t.item)
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
		flush(t.item.Bbox(), false, &t.item)
	})
}

func (t *Text) SetText(s string) {
	t.canvas.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.item.Text = s
		t.recalc(true)
		flush(r, false, &t.item)
		flush(t.item.Bbox(), false, &t.item)
	})
}

func (t *Text) SetFontSize(size float) {
	t.canvas.Atomically(func(flush FlushFunc) {
		r := t.item.Bbox()
		t.item.SetFontSize(size)
		t.recalc(true)
		flush(r, false, &t.item)
		flush(t.item.Bbox(), false, &t.item)
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
	case E | W:
		dp.X = r.Dx() / 2
	}
	switch flags & (N | S) {
	case S:
		dp.Y = r.Dy()
	case S | N:
		dp.Y = r.Dy() / 2
	}
	return r.Add(p.Sub(r.Min).Sub(dp))
}
