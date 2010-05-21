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
	lock sync.Mutex
	dst *image.RGBA
	flushrect draw.Rectangle
	r draw.Rectangle			// for convenience, the bounding rectangle of the dst
	scratch *image.RGBA
	waste int
	background image.Image
	objects *canvasObjects			// foreground objects are at the end of the list
	flushFunc func(r draw.Rectangle)
}

type canvasObjects struct {
	obj CanvasObject
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

// NewCancas return a new Canvas object that uses dst for its
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
	defer func(){
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
	abb := c.flushrect.Dx()*c.flushrect.Dy()
	anbb := nbb.Dx()*nbb.Dy()

	// Area of new waste is area of new bb minus area of old bb,
	// less the area of the new segment, which we assume is not waste.
	// This could be negative, but that's OK.
	c.waste += anbb-abb - ar
	if c.waste < 0 {
		c.waste = 0
	}

	//absorb if:
	//	total area is small
	//	waste is less than half total area
	// 	rectangles touch
	if anbb<=1024 || c.waste*2<anbb || c.flushrect.Overlaps(r) {
		c.flushrect = nbb;
		return;
	}
	//  emit current state
	if(!c.flushrect.Empty()) {
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

func (c *Canvas) scratchImage(width, height int) *image.RGBA {
	if c.scratch == nil || width > c.scratch.Width() || height > c.scratch.Height() {
		c.scratch = image.NewRGBA(width, height)
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
	c.lock.Lock()
	defer c.lock.Unlock()

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
	}else{
		c.addFlush(r)
	}
	c.lock.Unlock()
}

// An ImageObject represents an rectangular (but possibly
// transparent) image.
//
type ImageObject struct {
	canvas *Canvas
	r draw.Rectangle
	img image.Image
}

// Image returns a new ImageObject which will be drawn using img,
// with p giving the coordinate of the image's top left corner.
//
func (c *Canvas) Image(img image.Image, p draw.Point) *ImageObject {
	obj := new(ImageObject)
	obj.r = draw.Rectangle{p, p.Add(draw.Pt(img.Width(), img.Height()))}
	obj.canvas = c
	obj.img = img
	c.AddObject(obj)
	return obj
}

// Move moves the image's lower left corner to p.
//
func (obj *ImageObject) Move(p draw.Point) {
	c := obj.canvas
	c.lock.Lock()
	r := obj.r
	obj.r = obj.r.Add(p.Sub(obj.r.Min))
	c.addFlush(r)
	c.addFlush(obj.r)
	c.lock.Unlock()
}

func (obj *ImageObject) Draw(dst *image.RGBA, clip draw.Rectangle) {
	dr := obj.r.Clip(clip)
	sp := dr.Min.Sub(obj.r.Min)
	draw.Draw(dst, dr, obj.img, sp)
}

func (obj *ImageObject) Bbox() draw.Rectangle {
	return obj.r
}

// A PolyObject represents a filled polygon.
//
type PolyObject struct {
	canvas *Canvas
	col image.Color
	points []raster.Point
	bbox draw.Rectangle
	painter raster.RGBAPainter
	clipper clippedPainter
	rasterizer *raster.Rasterizer
}

const lineSafety = 2

// Polygon returns a new PolyObject, using col for its fill colour, and
// using points for its vertices.
//
func (c *Canvas) Polygon(col image.Color, points []draw.Point) *PolyObject {
	obj := new(PolyObject)
	rpoints := make([]raster.Point, len(points))
	for i, p := range points {
		rpoints[i] = pixel2fixPoint(p)
	}
	obj.init(c, col, rpoints)
	c.AddObject(obj)
	return obj
}

func (obj *PolyObject) init(c *Canvas, col image.Color, points []raster.Point) {
	obj.canvas = c
	obj.col = col
	obj.points = points
	obj.rasterizer = raster.NewRasterizer(c.dst.Width(), c.dst.Height())

	obj.painter.SetColor(obj.col)
	obj.clipper.Painter = &obj.painter
	obj.makeShape()
}

func (obj *PolyObject) makeShape() {
	obj.rasterizer.Clear()
	obj.rasterizer.Start(obj.points[0])
	min := obj.points[0]
	max := obj.points[0]
	for _, p := range obj.points[1:] {
		obj.rasterizer.Add1(p)
		if p.X < min.X {
			min.X = p.X
		}
		if p.X > max.X {
			max.X = p.X
		}
		if p.Y < min.Y {
			min.Y = p.Y
		}
		if p.Y > max.Y {
			max.Y = p.Y
		}
	}
	r := draw.Rectangle{fix2pixelPoint(min), fix2pixelPoint(max)}
	obj.bbox= r.Inset(-1)
//	fmt.Printf("poly bbox %v\n", obj.bbox)
}

func (obj *PolyObject) Bbox() draw.Rectangle {
	return obj.bbox
}

func (obj *PolyObject) Draw(dst *image.RGBA, clipr draw.Rectangle) {
	obj.clipper.Clipr = clipr
	obj.painter.Image = dst
	obj.rasterizer.Rasterize(&obj.clipper)
}

// A line object represents a single straight line.
type LineObject struct {
	PolyObject
	p0, p1 raster.Point
	width raster.Fixed
}

// Line returns a new LineObject, coloured with col, from p0 to p1,
// of the given width.
//
func (c *Canvas) Line(col image.Color, p0, p1 draw.Point, width float) *LineObject {
	obj := new(LineObject)
	obj.p0 = pixel2fixPoint(p0)
	obj.p1 = pixel2fixPoint(p1)
	obj.width = float2fix(width)
	obj.init(c, col, linePoly(obj.p0, obj.p1, obj.width))

	c.AddObject(obj)
	return obj
}

func linePoly(p0, p1 raster.Point, width raster.Fixed) []raster.Point {
	sin, cos := isincos2(p1.X - p0.X, p1.Y - p0.Y)
	dx := (sin * width) / (2 * fixScale)
	dy := (cos * width) / (2 * fixScale)
	pts := make([]raster.Point, 10)
	n := 0
	q := raster.Point{
		p0.X + fixScale/2 - cos/2,
		p0.Y + fixScale/2 - sin/2,
	}
	pts[n] = raster.Point{q.X - dx, q.Y + dy}
	n++
	pts[n] = raster.Point{q.X + dx, q.Y - dy}
	n++

	q = raster.Point{
		p1.X + fixScale/2 + cos/2,
		p1.Y + fixScale/2 + sin/2,
	}
	pts[n] = raster.Point{q.X+dx, q.Y-dy}
	n++
	pts[n] = raster.Point{q.X-dx, q.Y+dy}
	n++
	pts[n] = pts[0]
	n++

	return pts[0:n]
}

// Move changes the end coordinates of the LineObject.
//
func (obj *LineObject) Move(p0, p1 draw.Point) {
	c := obj.canvas
	c.lock.Lock()
	r := obj.bbox
	obj.p0 = pixel2fixPoint(p0)
	obj.p1 = pixel2fixPoint(p1)
	obj.points = linePoly(obj.p0, obj.p1, obj.width)
	obj.makeShape()
	c.addFlush(r)
	c.addFlush(obj.bbox)
	c.lock.Unlock()
}


type clippedPainter struct {
	Painter raster.Painter
	Clipr draw.Rectangle
}

func (p *clippedPainter) Paint(ss []raster.Span) {
	r := p.Clipr

	// quick check that we've at least got some rows that might be painted
	lastpt := ss[len(ss)-1]
	last := lastpt.X0 == lastpt.X1
//fmt.Printf("paint %v: %v\n", last, spans2ys(ss))
	if last {
		if len(ss) == 1 {
			p.Painter.Paint(ss)
			return
		}
		lastpt = ss[len(ss)-2]
	}
	if ss[0].Y >= r.Max.Y || lastpt.Y <= r.Min.Y {
		return
	}

	for i, s := range ss {
		if s.Y >= r.Min.Y {
			ss = ss[i:]
			break
		}
	}
	j := 0
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
	if last {
		ss[j] = raster.Span{}
		j++
	}
	if j > 0 {
		p.Painter.Paint(ss[0:j])
	}
}

func spans2ys(ss []raster.Span) []int {
	f := make([]int, len(ss))
	for i, s := range ss {
		f[i] = s.Y
	}
	return f
}

const (
	fixBits = 8
	fixScale = 1<<fixBits		// matches raster.Fixed
)

func float2fix(f float) raster.Fixed {
	return raster.Fixed(f * fixScale + 0.5)
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
	tbl := make([]raster.Fixed, 100 + 2)
	for i := range tbl {
		tbl[i] = raster.Fixed(f(math.Atan(float64(i) / 100))*fixScale+0.5)
	}
	return tbl
}

func isincos2(x, y raster.Fixed) (sin, cos raster.Fixed) {
	if x == 0 {
		if y >= 0 {
			sin, cos = fixScale, 0
		}else{
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
	}else{
		tan = 1000 * y / x
		tan10 = tan / 10
		stp = sinus[tan10:]
		ctp = cosinus[tan10:]
	}
	rem := tan - (tan10 * 10)
	sin = sinsign * (stp[0]+(stp[1]-stp[0])*rem/10)
	cos = cossign * (ctp[0]+(ctp[1]-ctp[0])*rem/10)
	return
}

type sizer interface {
	Width() int
	Height() int
}
func size(x sizer) draw.Point {
	return draw.Point{x.Width(), x.Height()}
}
