package canvas

import (
	"code.google.com/p/freetype-go/freetype"
	"code.google.com/p/freetype-go/freetype/raster"
	"code.google.com/p/freetype-go/freetype/truetype"
	"code.google.com/p/rog-go/values"
	"image"
	"image/draw"
)

const (
	dpi   = 72
	gamma = 1
)

type TextItem struct {
	*freetype.Context
	Text string
	Pt   raster.Point
	bbox image.Rectangle
	dst  draw.Image
	fill image.Image

	// these three painters are arranged in a stack, with the
	// first at the bottom and the last at the top.
	rp raster.Painter
	gp *raster.GammaCorrectionPainter
	cp clippedPainter
}

func (d *TextItem) Init() *TextItem {
	d.Context = freetype.NewContext()
	d.gp = raster.NewGammaCorrectionPainter(nil, gamma)
	d.cp.Painter = d.gp
	return d
}

func (d *TextItem) makePainter() {
	if d.dst != nil {
		d.rp = NewPainter(d.dst, d.fill, draw.Over)
	} else {
		d.rp = nil
	}
	d.gp.Painter = d.rp
}

func (d *TextItem) CalcBbox() {
	var bbox bboxPainter
	//	d.DrawText(&bbox, d.Pt, d.Text)
	d.bbox = bbox.R
}

func (d *TextItem) SetFill(fill image.Image) {
	d.fill = fill
	d.makePainter()
}

func (d *TextItem) Opaque() bool {
	return false
}

func (d *TextItem) Draw(dst draw.Image, clip image.Rectangle) {
	if dst != d.dst {
		d.dst = dst
		d.makePainter()
	}
	d.cp.Clipr = clip
	//	d.DrawText(&d.cp, d.Pt, d.Text)
}

func (d *TextItem) HitTest(p image.Point) bool {
	var hit hitTestPainter
	hit.P = p
	//	d.DrawText(&hit, d.Pt, d.Text)
	return hit.Hit
}

func (d *TextItem) Bbox() image.Rectangle {
	return d.bbox
}

func (d *TextItem) SetContainer(_ Backing) {
}

type Text struct {
	Item
	item    TextItem
	delta   image.Point // vector from upper left of bbox to text origin
	p       image.Point
	anchor  Anchor
	backing Backing
	value   values.Value
}

// NewText creates a new item to display a line of text.
// If val is non-nil, it should be a string-typed Value,
// and the Value's text will be displayed instead of s.
//
func NewText(p image.Point, where Anchor, s string, font *truetype.Font, size float64, val values.Value) *Text {
	t := new(Text)
	t.item.Init()
	t.item.SetFont(font)
	t.item.SetFontSize(size)
	t.item.fill = image.Black
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
	g := t.value.Getter()
	for {
		x, ok := g.Get()
		if !ok {
			break
		}
		t.SetText(x.(string))
		t.backing.Flush()
	}
}

func (t *Text) SetContainer(c Backing) {
	t.backing = c
}

func (t *Text) SetCentre(cp image.Point) {
	delta := cp.Sub(centre(t.Bbox()))
	t.SetPoint(t.p.Add(delta))
}

func (t *Text) SetPoint(p0 image.Point) {
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
	var bbox image.Rectangle
	if sizechanged {
		t.item.Pt = raster.Point{0, 0}
		t.item.CalcBbox()
		bbox = t.item.Bbox()
		t.delta = image.Point{0, 0}.Sub(bbox.Min)
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

func (t *Text) SetFontSize(size float64) {
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

func anchor(r image.Rectangle, flags Anchor, p image.Point) image.Rectangle {
	var dp image.Point
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
