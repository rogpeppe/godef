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

func Box(width, height int, col image.Image, border int, bordercol image.Image) image.Image {
	img := image.NewRGBA(width, height)
	if border < 0 {
		border = 0
	}
	r := draw.Rect(0, 0, width, height)
	draw.Draw(img, r.Inset(border), col, draw.ZP)
	draw.Border(img, r, border, bordercol, draw.ZP)
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
	obj canvasObject
	next *canvasObjects
}

type canvasObject interface {
	Draw(clip draw.Rectangle)
	Bbox() draw.Rectangle
}

func NewCanvas(dst draw.Image, background image.Image, flush func(r draw.Rectangle)) *Canvas {
	c := new(Canvas)
	c.dst = dst.(*image.RGBA)

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

func (c *Canvas) Width() int {
	return c.dst.Width()
}

func (c *Canvas) Height() int {
	return c.dst.Height()
}

func (c *Canvas) scratchImage(width, height int) *image.RGBA {
	if c.scratch == nil || width > c.scratch.Width() || height > c.scratch.Height() {
		c.scratch = image.NewRGBA(width, height)
	}
	return c.scratch
}

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
			obj.Draw(c.flushrect)
//fmt.Printf("done draw of %T\n", obj)
		}
	}
//fmt.Printf("done redraw\n")
	if c.flushFunc != nil {
		c.flushFunc(c.flushrect)
	}
	c.flushrect = draw.ZR
}

func (c *Canvas) Delete(obj canvasObject) {
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

func (c *Canvas) BackgroundChanged(r draw.Rectangle) {
	c.lock.Lock()
	c.addFlush(r)
	c.lock.Unlock()
}

func (c *Canvas) addObject(obj canvasObject) {
	c.objects = &canvasObjects{obj, c.objects}
	r := obj.Bbox()
	if c.flushrect.Empty() {
		obj.Draw(r)
	}else{
		c.addFlush(r)
	}
}

type ImageObject struct {
	canvas *Canvas
	r draw.Rectangle
	img image.Image
}

func (c *Canvas) Image(img image.Image, p draw.Point) *ImageObject {
	c.lock.Lock()
	obj := new(ImageObject)
	obj.r = draw.Rectangle{p, p.Add(draw.Pt(img.Width(), img.Height()))}
	obj.canvas = c
	obj.img = img
	c.addObject(obj)
	c.lock.Unlock()
	return obj
}

func (obj *ImageObject) Move(p draw.Point) {
	c := obj.canvas
	c.lock.Lock()
	c.addFlush(obj.r)
	obj.r = obj.r.Add(p.Sub(obj.r.Min))
	c.addFlush(obj.r)
	c.lock.Unlock()
}

func (obj *ImageObject) Draw(clip draw.Rectangle) {
	c := obj.canvas
	dr := obj.r.Clip(c.flushrect)
	sp := dr.Min.Sub(obj.r.Min)
	draw.Draw(c.dst, dr, obj.img, sp)
}

func (obj *ImageObject) Bbox() draw.Rectangle {
	return obj.r
}


type PolyObject struct {
	canvas *Canvas
	col image.Color
	points []raster.Point
	bbox draw.Rectangle
	painter raster.RGBAPainter
	clipper ClippedPainter
	rasterizer *raster.Rasterizer
}

const lineSafety = 2

func (c *Canvas) Polygon(col image.Color, points []draw.Point) *PolyObject {
	c.lock.Lock()
	obj := new(PolyObject)
	rpoints := make([]raster.Point, len(points))
	for i, p := range points {
		rpoints[i] = pixel2fixPoint(p)
	}
	obj.init(c, col, rpoints)
	c.addObject(obj)
	c.lock.Unlock()
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

func (obj *PolyObject) Draw(clipr draw.Rectangle) {
	c := obj.canvas
	obj.clipper.Clipr = clipr
	obj.painter.Image = c.dst
	obj.rasterizer.Rasterize(&obj.clipper)
}

type LineObject struct {
	PolyObject
	p0, p1 raster.Point
	width raster.Fixed
}

func (c *Canvas) Line(col image.Color, p0, p1 draw.Point, width float) *LineObject {
	obj := new(LineObject)
	obj.p0 = pixel2fixPoint(p0)
	obj.p1 = pixel2fixPoint(p1)
	obj.width = float2fix(width)

	c.lock.Lock()
	obj.init(c, col, linePoly(obj.p0, obj.p1, obj.width))
	c.addObject(obj)
	c.lock.Unlock()
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


type ClippedPainter struct {
	Painter raster.Painter
	Clipr draw.Rectangle
}

func (p *ClippedPainter) Paint(ss []raster.Span) {
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

type Sizer interface {
	Width() int
	Height() int
}
func size(x Sizer) draw.Point {
	return draw.Point{x.Width(), x.Height()}
}
