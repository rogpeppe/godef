package canvas

import (
	"exp/draw"
	"image"
	//	"fmt"
	"math"
	"freetype-go.googlecode.com/hg/freetype/raster"
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
	draw.Draw(img, r.Inset(border), col, draw.ZP)
	draw.Border(img, r, border, borderCol, draw.ZP)
	return img
}

// An ImageDrawer is a CanvasObject that uses an image
// to draw itself.
type ImageDrawer struct {
	r   draw.Rectangle
	img image.Image
}

func (obj *ImageDrawer) Draw(dst *image.RGBA, clip draw.Rectangle) {
	dr := obj.r.Clip(clip)
	sp := dr.Min.Sub(obj.r.Min)
	draw.Draw(dst, dr, obj.img, sp)
}

func (obj *ImageDrawer) Bbox() draw.Rectangle {
	return obj.r
}

// An Image represents an rectangular (but possibly
// transparent) image.
//
type Image struct {
	drawer ImageDrawer
	canvas *Canvas
}

// Image returns a new Image which will be drawn using img,
// with p giving the coordinate of the image's top left corner.
//
func NewImage(c *Canvas, img image.Image, p draw.Point) *Image {
	obj := new(Image)
	obj.canvas = c
	obj.drawer.r = draw.Rectangle{p, p.Add(draw.Pt(img.Width(), img.Height()))}
	obj.drawer.img = img
	c.AddObject(&obj.drawer)
	return obj
}

// Move moves the image's lower left corner to p.
//
func (obj *Image) Move(p draw.Point) {
	if p.Eq(obj.drawer.r.Min) {
		return
	}
	obj.canvas.Atomic(func(flush func(r draw.Rectangle)) {
		r := obj.drawer.r
		obj.drawer.r = r.Add(p.Sub(r.Min))
		flush(r)
		flush(obj.drawer.r)
	})
}

func (obj *Image) Delete() {
	obj.canvas.Delete(&obj.drawer)
}

// A RasterDrawer is a low level canvas object that
// can be used to build higher level primitives.
// It implements CanvasObject, and will calculate
// (and remember) its bounding box on request.
//
// Otherwise it can be used as a raster.Rasterizer.
//
type RasterDrawer struct {
	raster.Rasterizer
	bbox draw.Rectangle
	raster.RGBAPainter
	clipper clippedPainter
}

// CalcBbox calculates the current bounding box of
// all the pixels in the current path.
func (obj *RasterDrawer) CalcBbox() {
	obj.bbox = rasterBbox(&obj.Rasterizer)
}

func (obj *RasterDrawer) Draw(dst *image.RGBA, clipr draw.Rectangle) {
	obj.clipper.Clipr = clipr
	obj.Image = dst
	obj.clipper.Painter = &obj.RGBAPainter
	obj.Rasterize(&obj.clipper)
}

func (obj *RasterDrawer) Bbox() draw.Rectangle {
	return obj.bbox
}

// A Polygon represents a filled polygon.
//
type Polygon struct {
	raster RasterDrawer
	canvas *Canvas
	points []raster.Point
}

// Polygon returns a new PolyObject, using col for its fill colour, and
// using points for its vertices.
//
func NewPolygon(c *Canvas, col image.Color, points []draw.Point) *Polygon {
	obj := new(Polygon)
	rpoints := make([]raster.Point, len(points))
	for i, p := range points {
		rpoints[i] = pixel2fixPoint(p)
	}
	obj.raster.SetColor(col)
	obj.raster.SetBounds(c.Width(), c.Height())
	obj.rasterize()
	c.AddObject(&obj.raster)
	return obj
}

func (obj *Polygon) Delete() {
	obj.canvas.Delete(&obj.raster)
}

func (obj *Polygon) rasterize() {
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
	raster RasterDrawer
	canvas *Canvas
	p0, p1 raster.Point
	width  raster.Fixed
}

// Line returns a new Line, coloured with col, from p0 to p1,
// of the given width.
//
func NewLine(c *Canvas, col image.Color, p0, p1 draw.Point, width float) *Line {
	obj := new(Line)
	obj.canvas = c
	obj.p0 = pixel2fixPoint(p0)
	obj.p1 = pixel2fixPoint(p1)
	obj.width = float2fix(width)
	obj.raster.SetColor(col)
	obj.raster.SetBounds(c.Width(), c.Height())
	obj.rasterize()
	c.AddObject(&obj.raster)
	return obj
}

func (obj *Line) rasterize() {
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

// Move changes the end coordinates of the Line.
//
func (obj *Line) Move(p0, p1 draw.Point) {
	obj.canvas.Atomic(func(flush func(r draw.Rectangle)) {
		obj.p0 = pixel2fixPoint(p0)
		obj.p1 = pixel2fixPoint(p1)
		r := obj.raster.Bbox()
		obj.rasterize()
		flush(r)
		flush(obj.raster.Bbox())
	})
}

func (obj *Line) Delete() {
	obj.canvas.Delete(&obj.raster)
}

// SetColor changes the colour of the line
//
func (obj *Line) SetColor(col image.Color) {
	obj.canvas.Atomic(func(flush func(r draw.Rectangle)) {
		obj.raster.SetColor(col)
		flush(obj.raster.Bbox())
	})
}

type clippedPainter struct {
	Painter raster.Painter
	Clipr   draw.Rectangle
}

func (p *clippedPainter) Paint(ss []raster.Span, last bool) {
	r := p.Clipr
	j := 0
	// quick check that we've at least got some rows that might be painted
	if len(ss) > 0 && ss[0].Y < r.Max.Y && ss[len(ss)-1].Y > r.Min.Y {
		for i, s := range ss {
			if s.Y >= r.Min.Y {
				ss = ss[i:]
				break
			}
		}
		for i, s := range ss {
			if s.Y >= r.Max.Y {
				break
			}
			if s.X0 < r.Max.X && r.Min.X < s.X1 {
				sp := &ss[j]
				if i != j {
					*sp = s
				}
				if s.X0 < r.Min.X {
					sp.X0 = r.Min.X
				}
				if s.X1 > r.Max.X {
					sp.X1 = r.Max.X
				}
				j++
			}
		}
	}
	if j > 0 || last {
		p.Painter.Paint(ss[0:j], last)
	}
}

func rasterBbox(rasterizer *raster.Rasterizer) (r draw.Rectangle) {
	r.Min.X = 1e9
	r.Min.Y = 1e9
	r.Max.X = -1e9
	r.Max.Y = -1e9
	rasterizer.Rasterize(raster.PainterFunc(func(ss []raster.Span, end bool) {
		if len(ss) > 0 {
			sp := &ss[0]
			if sp.Y < r.Min.Y {
				r.Min.Y = sp.Y
			}
			sp = &ss[len(ss)-1]
			if sp.Y > r.Max.Y {
				r.Max.Y = sp.Y
			}
			for i := range ss {
				sp := &ss[i]
				if sp.X0 < r.Min.X {
					r.Min.X = sp.X0
				}
				if sp.X1 > r.Max.X {
					r.Max.X = sp.X1
				}
			}
		}
	}))
	if r.Min.X > r.Max.X || r.Min.Y > r.Max.Y {
		return draw.ZR
	}
	return
}

func spans2ys(ss []raster.Span) []int {
	f := make([]int, len(ss))
	for i, s := range ss {
		f[i] = s.Y
	}
	return f
}


const (
	fixBits  = 8
	fixScale = 1 << fixBits // matches raster.Fixed
)

func float2fix(f float) raster.Fixed {
	return raster.Fixed(f*fixScale + 0.5)
}

func int2fix(i int) raster.Fixed {
	return raster.Fixed(i << fixBits)
}

func fix2int(i raster.Fixed) int {
	return int((i + fixScale/2) >> fixBits)
}

func pixel2fixPoint(p draw.Point) raster.Point {
	return raster.Point{raster.Fixed(p.X << fixBits), raster.Fixed(p.Y << fixBits)}
}

func fix2pixelPoint(p raster.Point) draw.Point {
	return draw.Point{int((p.X + fixScale/2) >> fixBits), int((p.Y + fixScale/2) >> fixBits)}
}

// could do it in fixed point, but what's 0.5us between friends?
func isincos2(x, y raster.Fixed) (isin, icos raster.Fixed) {
	sin, cos := math.Sincos(math.Atan2(fixed2float(x), fixed2float(y)))
	isin = float2fixed(sin)
	icos = float2fixed(cos)
	return
}

func float2fixed(f float64) raster.Fixed {
	if f < 0 {
		return raster.Fixed(f * 256 + 0.5)
	}
	return raster.Fixed(f * 256 - 0.5)
}

func fixed2float(f raster.Fixed) float64 {
	return float64(f) / 256
}
