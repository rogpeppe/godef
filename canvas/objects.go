package canvas

import (
	"exp/draw"
	"image"
	"math"
	"freetype-go.googlecode.com/hg/freetype/raster"
	"rog-go.googlecode.com/hg/values"
)

// Box creates a rectangular image of the given size, filled with the given colour,
// with a border-size border of colour borderCol.
//
func Box(width, height int, col image.Image, border int, borderCol image.Image) image.Image {
	img := image.NewRGBA(width, height)
	if border < 0 {
		border = 0
	}
	r := draw.Rect(0, 0, width, height)
	draw.DrawMask(img, r.Inset(border), col, draw.ZP, nil, draw.ZP, draw.Src)
	BorderOp(img, r, border, borderCol, draw.ZP, draw.Src)
	return img
}

// An ImageItem is an Item that uses an image
// to draw itself. It is intended to be used as a building
// block for other Items.
type ImageItem struct {
	R      draw.Rectangle
	Image    image.Image
	IsOpaque bool
}

func (obj *ImageItem) Draw(dst draw.Image, clip draw.Rectangle) {
	dr := obj.R.Clip(clip)
	sp := dr.Min.Sub(obj.R.Min)
	op := draw.Over
	if obj.IsOpaque {
		op = draw.Src
	}
	draw.DrawMask(dst, dr, obj.Image, sp, nil, draw.ZP, op)
}

func (obj *ImageItem) SetContainer(c Backing) {
}

func (obj *ImageItem) Opaque() bool {
	return obj.IsOpaque
}

func (obj *ImageItem) Bbox() draw.Rectangle {
	return obj.R
}

func (obj *ImageItem) HitTest(p draw.Point) bool {
	return p.In(obj.R)
}

// An Image represents an rectangular (but possibly
// transparent) image.
//
type Image struct {
	Item
	item   ImageItem // access to the fields of the ImageItem
	backing Backing
}

// Image returns a new Image which will be drawn using img,
// with p giving the coordinate of the image's top left corner.
//
func NewImage(img image.Image, opaque bool, p draw.Point) *Image {
	obj := new(Image)
	obj.Item = &obj.item
	obj.item.R = draw.Rectangle{p, p.Add(draw.Pt(img.Width(), img.Height()))}
	obj.item.Image = img
	obj.item.IsOpaque = opaque
	return obj
}

func (obj *Image) SetContainer(c Backing) {
	obj.backing = c
}

func (obj *Image) SetCentre(p draw.Point) {
	p = p.Sub(centreDist(obj.Bbox()))
	if p.Eq(obj.item.R.Min) {
		return
	}
	obj.backing.Atomically(func(flush FlushFunc) {
		r := obj.item.R
		obj.item.R = r.Add(p.Sub(r.Min))
		flush(r, nil)
		flush(obj.item.R, nil)
	})
}

// A Polygon represents a filled polygon.
//
type Polygon struct {
	Item
	raster RasterItem
	backing Backing
	points []raster.Point
}

// Polygon returns a new PolyObject, using col for its fill colour, and
// using points for its vertices.
//
func NewPolygon(fill image.Image, points []draw.Point) *Polygon {
	obj := new(Polygon)
	rpoints := make([]raster.Point, len(points))
	for i, p := range points {
		rpoints[i] = pixel2fixPoint(p)
	}
	obj.raster.SetFill(fill)
	obj.points = rpoints
	obj.Item = &obj.raster
	return obj
}

func (obj *Polygon) SetContainer(c Backing) {
	obj.backing = c
	obj.raster.SetContainer(c)
	obj.makeOutline()
}

func (obj *Polygon) makeOutline() {
	obj.raster.Clear()
	if len(obj.points) > 0 {
		obj.raster.Start(obj.points[0])
		for _, p := range obj.points[1:] {
			obj.raster.Add1(p)
		}
		obj.raster.Add1(obj.points[0])
	}
	obj.raster.CalcBbox()
}

// A line object represents a single straight line.
type Line struct {
	Item
	raster RasterItem
	backing Backing
	p0, p1 raster.Point
	width  raster.Fixed
}

// Line returns a new Line, coloured with col, from p0 to p1,
// of the given width.
//
func NewLine(fill image.Image, p0, p1 draw.Point, width float) *Line {
	obj := new(Line)
	obj.p0 = pixel2fixPoint(p0)
	obj.p1 = pixel2fixPoint(p1)
	obj.width = float2fix(width)
	obj.raster.SetFill(fill)
	obj.Item = &obj.raster
	obj.makeOutline()
	return obj
}

func (obj *Line) SetContainer(b Backing) {
	obj.backing = b
	obj.raster.SetContainer(b)
	obj.makeOutline()
}

func (obj *Line) makeOutline() {
	obj.raster.Clear()
	sin, cos := isincos2(obj.p1.X-obj.p0.X, obj.p1.Y-obj.p0.Y)
	dx := (cos * obj.width) / (2 * fixScale)
	dy := (sin * obj.width) / (2 * fixScale)
	q := raster.Point{
		obj.p0.X + fixScale/2 - sin/2,
		obj.p0.Y + fixScale/2 - cos/2,
	}
	p0 := raster.Point{q.X - dx, q.Y + dy}
	obj.raster.Start(p0)
	obj.raster.Add1(raster.Point{q.X + dx, q.Y - dy})

	q = raster.Point{
		obj.p1.X + fixScale/2 + sin/2,
		obj.p1.Y + fixScale/2 + cos/2,
	}
	obj.raster.Add1(raster.Point{q.X + dx, q.Y - dy})
	obj.raster.Add1(raster.Point{q.X - dx, q.Y + dy})
	obj.raster.Add1(p0)
	obj.raster.CalcBbox()
}

// SetEndPoints changes the end coordinates of the Line.
//
func (obj *Line) SetEndPoints(p0, p1 draw.Point) {
	obj.backing.Atomically(func(flush FlushFunc) {
		r := obj.raster.Bbox()
		obj.p0 = pixel2fixPoint(p0)
		obj.p1 = pixel2fixPoint(p1)
		obj.makeOutline()
		flush(r, nil)
		flush(obj.raster.Bbox(), nil)
	})
}

// SetColor changes the colour of the line
//
func (obj *Line) SetFill(fill image.Image) {
	obj.backing.Atomically(func(flush FlushFunc) {
		obj.raster.SetFill(fill)
		flush(obj.raster.Bbox(), nil)
	})
}

// could do it in fixed point, but what's 0.5us between friends?
func isincos2(x, y raster.Fixed) (isin, icos raster.Fixed) {
	sin, cos := math.Sincos(math.Atan2(fixed2float(x), fixed2float(y)))
	isin = float2fixed(sin)
	icos = float2fixed(cos)
	return
}

type Slider struct {
	backing Backing
	value values.Value
	Item
	c      *Canvas
	val    float64
	box    ImageItem
	button ImageItem
}

// A Slider shows a mouse-adjustable slider bar.
// NewSlider returns the Slider item.
// The value is used to set and get the current slider value;
// its Type() should be float64; the slider's value is in the
// range [0, 1].
//
func NewSlider(r draw.Rectangle, fg, bg image.Color, value values.Value) (obj *Slider) {
	obj = new(Slider)
	obj.value = value
	obj.c = NewCanvas(nil, r)
	obj.box.R = r
	obj.box.Image = Box(r.Dx(), r.Dy(), image.ColorImage{bg}, 1, image.Black)
	obj.box.IsOpaque = opaqueColor(bg)

	br := obj.buttonRect()
	obj.button.R = br
	obj.button.Image = Box(br.Dx(), br.Dy(), image.ColorImage{fg}, 1, image.Black)
	obj.button.IsOpaque = opaqueColor(fg)
	obj.c.AddItem(&obj.box)
	obj.c.AddItem(&obj.button)

	go obj.listener()

	obj.Item = obj.c
	return obj
}

const buttonWidth = 6

func (obj *Slider) SetContainer(c Backing) {
	obj.backing = c
}

func (obj *Slider) buttonRect() (r draw.Rectangle) {
	r.Min.Y = obj.box.R.Min.Y
	r.Max.Y = obj.box.R.Max.Y
	p := obj.val
	centre := int(p*float64(obj.box.R.Max.X-obj.box.R.Min.X-buttonWidth)+0.5) + obj.box.R.Min.X + buttonWidth/2
	r.Min.X = centre - buttonWidth/2
	r.Max.X = centre + buttonWidth/2
	return
}

func (obj *Slider) listener() {
	for x := range obj.value.Iter() {
		v := x.(float64)
		obj.backing.Atomically(func(flush FlushFunc) {
			if v > 1 {
				v = 1
			}
			if v < 0 {
				v = 0
			}
			obj.val = v
			r := obj.button.R
			obj.button.R = obj.buttonRect()
			flush(r, nil)
			flush(obj.button.R, nil)
		})
		obj.backing.Flush()
	}
}

func (obj *Slider) x2val(x int) float64 {
	return float64(x-(obj.box.R.Min.X+buttonWidth/2)) / float64(obj.box.R.Dx()-buttonWidth)
}

func (obj *Slider) HandleMouse(f Flusher, m draw.Mouse, mc <-chan draw.Mouse) bool {
	if m.Buttons&1 == 0 {
		return false
	}
	offset := 0
	br := obj.buttonRect()
	if !m.In(br) {
		obj.value.Set(obj.x2val(m.X))
	} else {
		offset = m.X - (br.Min.X+br.Max.X)/2
	}

	but := m.Buttons
	for {
		m = <-mc
		obj.value.Set(obj.x2val(m.X - offset))
		if (m.Buttons & but) != but {
			break
		}
	}
	return true
}

func opaqueColor(col image.Color) bool {
	_, _, _, a := col.RGBA()
	return a == 0xffff
}

func DrawOp(dst draw.Image, r draw.Rectangle, src image.Image, sp draw.Point, op draw.Op) {
	draw.DrawMask(dst, r, src, sp, nil, draw.ZP, op)
}

// Border aligns r.Min in dst with sp in src and then replaces pixels
// in a w-pixel border around r in dst with the result of the Porter-Duff compositing
// operation ``src over dst.''  If w is positive, the border extends w pixels inside r.
// If w is negative, the border extends w pixels outside r.
func BorderOp(dst draw.Image, r draw.Rectangle, w int, src image.Image, sp draw.Point, op draw.Op) {
	i := w
	if i > 0 {
		// inside r
		DrawOp(dst, draw.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+i), src, sp, op)                          // top
		DrawOp(dst, draw.Rect(r.Min.X, r.Min.Y+i, r.Min.X+i, r.Max.Y-i), src, sp.Add(draw.Pt(0, i)), op)        // left
		DrawOp(dst, draw.Rect(r.Max.X-i, r.Min.Y+i, r.Max.X, r.Max.Y-i), src, sp.Add(draw.Pt(r.Dx()-i, i)), op) // right
		DrawOp(dst, draw.Rect(r.Min.X, r.Max.Y-i, r.Max.X, r.Max.Y), src, sp.Add(draw.Pt(0, r.Dy()-i)), op)     // bottom
		return
	}

	// outside r;
	i = -i
	DrawOp(dst, draw.Rect(r.Min.X-i, r.Min.Y-i, r.Max.X+i, r.Min.Y), src, sp.Add(draw.Pt(-i, -i)), op) // top
	DrawOp(dst, draw.Rect(r.Min.X-i, r.Min.Y, r.Min.X, r.Max.Y), src, sp.Add(draw.Pt(-i, 0)), op)      // left
	DrawOp(dst, draw.Rect(r.Max.X, r.Min.Y, r.Max.X+i, r.Max.Y), src, sp.Add(draw.Pt(r.Dx(), 0)), op)  // right
	DrawOp(dst, draw.Rect(r.Min.X-i, r.Max.Y, r.Max.X+i, r.Max.Y+i), src, sp.Add(draw.Pt(-i, 0)), op)  // bottom
}

func centreDist(r draw.Rectangle) draw.Point {
	return draw.Pt(r.Dx() / 2, r.Dy() / 2)
}

func centre(r draw.Rectangle) draw.Point {
	return draw.Pt((r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2)
}
