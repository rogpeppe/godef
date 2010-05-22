// Package canvas layers a set of movable objects onto
// a draw.Image. Objects in the canvas may be created,
// moved and deleted; the Canvas manages any necessary
// re-drawing.
package canvas

import (
	"exp/draw"
	"image"
	//	"fmt"
	"math"
	"sync"
	"freetype-go.googlecode.com/hg/freetype/raster"
)

func eqrect(r0, r1 draw.Rectangle) bool {
	return r0.Min.Eq(r1.Min) && r0.Max.Eq(r1.Max)
}

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

type Canvas struct {
	lock       sync.Mutex
	dst        *image.RGBA
	flushrect  draw.Rectangle
	r          draw.Rectangle // for convenience, the bounding rectangle of the dst
	scratch    *image.RGBA
	waste      int
	background image.Image
	objects    *canvasObjects // foreground objects are at the end of the list
	flushFunc  func(r draw.Rectangle)
}

type canvasObjects struct {
	obj  CanvasObject
	next *canvasObjects
}

// Objects that implement the CanvasObject interface may be added
// to the canvas. The objects should adhere to the following rules:
// - No calls to the canvas should be made while in the Draw or Bbox methods.
// - All changes to the appearance of the object should be made using
// the Atomic function.
//
type CanvasObject interface {
	Draw(img *image.RGBA, clip draw.Rectangle)
	Bbox() draw.Rectangle
}

// NewCanvas return a new Canvas object that uses dst for its
// underlying image. The background image will used to
// draw the background, and the flush function, if non-nil
// will be called when dst has been modified, with
// a bounding rectangle of the changed area.
//
func NewCanvas(dst *image.RGBA, background image.Image, flush func(r draw.Rectangle)) *Canvas {
	c := new(Canvas)
	c.dst = dst

	c.r = draw.Rect(0, 0, dst.Width(), dst.Height())
	c.flushrect = c.r
	c.background = background
	c.flushFunc = flush

	return c
}

// stolen from inferno's devdraw
func (c *Canvas) addFlush(r draw.Rectangle) {
	defer func() {
		if !eqrect(c.flushrect, c.flushrect.Canon()) {
			panic("setting non-canonical flushrect")
		}
	}()
	if c.flushrect.Empty() {
		c.flushrect = r
		c.waste = 0
		return
	}
	nbb := c.flushrect.Combine(r)
	ar := r.Dx() * r.Dy()
	abb := c.flushrect.Dx() * c.flushrect.Dy()
	anbb := nbb.Dx() * nbb.Dy()

	// Area of new waste is area of new bb minus area of old bb,
	// less the area of the new segment, which we assume is not waste.
	// This could be negative, but that's OK.
	c.waste += anbb - abb - ar
	if c.waste < 0 {
		c.waste = 0
	}

	//absorb if:
	//	total area is small
	//	waste is less than half total area
	// 	rectangles touch
	if anbb <= 1024 || c.waste*2 < anbb || c.flushrect.Overlaps(r) {
		c.flushrect = nbb
		return
	}
	//  emit current state
	if !c.flushrect.Empty() {
		c.flush()
	}
	c.flushrect = r
}

// Width returns the width of the canvas, which is
// the width of its underlying image.
//
func (c *Canvas) Width() int {
	return c.dst.Width()
}

// Width returns the height of the canvas, which is
// the height of its underlying image.
//
func (c *Canvas) Height() int {
	return c.dst.Height()
}

func (c *Canvas) scratchImage(x, height int) *image.RGBA {
	if c.scratch == nil || x > c.scratch.Width() || height > c.scratch.Height() {
		c.scratch = image.NewRGBA(x, height)
	}
	return c.scratch
}

// Flush flushes any pending changes to the underlying image.
//
func (c *Canvas) Flush() {
	c.lock.Lock()
	c.flush()
	c.lock.Unlock()
}

func (c *Canvas) flush() {
	c.flushrect = c.flushrect.Clip(c.r)
	if c.flushrect.Empty() {
		return
	}
	//	fmt.Println("draw:", c.flushrect, c.flushrect.Min)
	//	fmt.Printf("images: dst %v; background %v\n", size(c.dst), size(c.background) )
	//fmt.Printf("redraw %v\n", c.flushrect)
	draw.DrawMask(c.dst, c.flushrect, c.background, c.flushrect.Min, nil, draw.ZP, draw.Src)
	for objs := c.objects; objs != nil; objs = objs.next {
		obj := objs.obj
		r := obj.Bbox()
		if r.Overlaps(c.flushrect) {
			//fmt.Printf("drawing %T\n", obj)
			obj.Draw(c.dst, c.flushrect)
			//fmt.Printf("done draw of %T\n", obj)
		}
	}
	//fmt.Printf("done redraw\n")
	if c.flushFunc != nil {
		c.flushFunc(c.flushrect)
	}
	c.flushrect = draw.ZR
}

// Delete deletes the object obj from the canvas.
//
func (c *Canvas) Delete(obj CanvasObject) {
	c.lock.Lock()
	prev := &c.objects
	for o := c.objects; o != nil; o = o.next {
		if o.obj == obj {
			*prev = o.next
			o.next = nil
			c.addFlush(obj.Bbox())
			break
		}
		prev = &o.next
	}
	c.lock.Unlock()
}

// Atomic calls f while the canvas's lock is held,
// allowing objects to adjust their appearance without
// risk of drawing anomalies. The addflush function
// provided as an argument allows the code to specify
// that certain areas will need to be redrawn.
//
// N.B. addflush may actually cause an area to be
// redrawn immediately, so any changes in appearance
// should have been made before calling it.
//
func (c *Canvas) Atomic(f func(addFlush func(r draw.Rectangle))) {
	// could pre-allocate inside c if we cared.
	addFlush := func(r draw.Rectangle) {
		c.addFlush(r)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	f(addFlush)
}

// BackgroundChanged informs the canvas that the background
// image has changed. R should give the bounding rectangle
// of the change.
//
func (c *Canvas) BackgroundChanged(r draw.Rectangle) {
	c.lock.Lock()
	c.addFlush(r)
	c.lock.Unlock()
}

func (c *Canvas) AddObject(obj CanvasObject) {
	c.lock.Lock()
	c.objects = &canvasObjects{obj, c.objects}
	r := obj.Bbox()
	if c.flushrect.Empty() {
		obj.Draw(c.dst, r)
	} else {
		c.addFlush(r)
	}
	c.lock.Unlock()
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
func (c *Canvas) Image(img image.Image, p draw.Point) *Image {
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
func (c *Canvas) Polygon(col image.Color, points []draw.Point) *Polygon {
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
func (c *Canvas) Line(col image.Color, p0, p1 draw.Point, width float) *Line {
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
	dx := (sin * obj.width) / (2 * fixScale)
	dy := (cos * obj.width) / (2 * fixScale)
	q := raster.Point{
		obj.p0.X + fixScale/2 - cos/2,
		obj.p0.Y + fixScale/2 - sin/2,
	}
	p0 := raster.Point{q.X - dx, q.Y + dy}
	obj.raster.Start(p0)
	obj.raster.Add1(raster.Point{q.X + dx, q.Y - dy})

	q = raster.Point{
		obj.p1.X + fixScale/2 + cos/2,
		obj.p1.Y + fixScale/2 + sin/2,
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


// integer sincos stolen from inferno's libdraw.
// is it really worth it?

var sinus = maketable(math.Sin)
var cosinus = maketable(math.Cos)

func maketable(f func(float64) float64) []raster.Fixed {
	tbl := make([]raster.Fixed, 100+2)
	for i := range tbl {
		tbl[i] = raster.Fixed(f(math.Atan(float64(i)/100))*fixScale + 0.5)
	}
	return tbl
}

func isincos2(x, y raster.Fixed) (sin, cos raster.Fixed) {
	if x == 0 {
		if y >= 0 {
			sin, cos = fixScale, 0
		} else {
			sin, cos = -fixScale, 0
		}
		return
	}
	sinsign := raster.Fixed(1)
	cossign := raster.Fixed(1)
	if x < 0 {
		cossign = -1
		x = -x
	}
	if y < 0 {
		sinsign = -1
		y = -y
	}
	var tan, tan10 raster.Fixed
	var stp, ctp []raster.Fixed
	if y > x {
		tan = 1000 * x / y
		tan10 = tan / 10
		stp = cosinus[tan10:]
		ctp = sinus[tan10:]
	} else {
		tan = 1000 * y / x
		tan10 = tan / 10
		stp = sinus[tan10:]
		ctp = cosinus[tan10:]
	}
	rem := tan - (tan10 * 10)
	sin = sinsign * (stp[0] + (stp[1]-stp[0])*rem/10)
	cos = cossign * (ctp[0] + (ctp[1]-ctp[0])*rem/10)
	return
}

type sizer interface {
	Width() int
	Height() int
}

func size(x sizer) draw.Point {
	return draw.Point{x.Width(), x.Height()}
}
