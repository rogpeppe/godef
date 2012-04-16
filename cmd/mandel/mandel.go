package main

import (
	"code.google.com/p/rog-go/canvas"
	"exp/draw"
	"exp/draw/x11"
	"flag"
	"image"
	"image/color"
	"log"
	"time"
)

type stack struct {
	f          Fractal
	centre     draw.Point
	iterations int
	next       *stack
}

type context struct {
	cvs        *canvas.Canvas
	f          Fractal
	pushed     *stack
	tiler      *Tiler
	item       canvas.MoveableItem
	tileSize   int
	iterations int
	cache      tileTable
}

type Fractal interface {
	At(p draw.Point) color.RGBA
	Zoom(r draw.Rectangle) Fractal
	Resize(r draw.Rectangle) Fractal
	Associated(p draw.Point) Fractal
}

var topArea = crect{cmplx(-2.0, -1.5), cmplx(1.0, 1.5)}
var iterations = flag.Int("iter", 300, "max number of iterations per point")
var tileSize = flag.Int("tile", 40, "tile size in pixels")

func main() {
	flag.Parse()
	wctxt, err := x11.NewWindow()
	if wctxt == nil {
		log.Exitf("no window: %v", err)
	}
	screen := wctxt.Screen()

	ctxt := &context{}
	ctxt.iterations = *iterations

	bg := canvas.NewBackground(screen.(*image.RGBA), draw.White, flushFunc(wctxt))
	ctxt.cvs = canvas.NewCanvas(nil, bg.Rect())
	bg.SetItem(ctxt.cvs)

	ctxt.cache = NewTileTable()
	ctxt.setFractal(NewMandelbrot(topArea, ctxt.cvs.Rect(), false, 0, ctxt.iterations), draw.ZP)

	qc := wctxt.QuitChan()
	kc := wctxt.KeyboardChan()
	mc := wctxt.MouseChan()
	ctxt.cvs.Flush()
	for {
		select {
		case <-qc:
			log.Exit("quitting")
			return
		case m0 := <-mc:
			// b1 drag - drag mandel around.
			// double-b1 drag - zoom in to rectangle
			// douible-b1 click - zoom in a little
			// b2 click - julia
			// b2 drag - interactive julia
			// b3 zoom out
			if m0.Buttons == 0 {
				break
			}
			nclick := 0
			clicks, finalm := clicker(m0, mc)
			for _ = range clicks {
				nclick++
			}
			m := <-finalm
			dragging := m.Buttons != 0
			switch {
			case m0.Buttons&1 != 0:
				switch nclick {
				case 1:
					if dragging {
						ctxt.cvs.HandleMouse(ctxt.cvs, m, mc)
					}
				case 2:
					if dragging {
						ctxt.zoomRect(m, mc)
					} else {
						ctxt.zoomABit(m, mc)
					}
				}
			case m0.Buttons&2 != 0:
				switch nclick {
				case 1:
					if dragging {
						ctxt.interactiveJulia(m, mc)
					} else {
						ctxt.julia(m.Point)
					}
				}
			case m0.Buttons&4 != 0:
				ctxt.pop()
			}
		case <-kc:
		}
	}
}

func (ctxt *context) zoomRect(m draw.Mouse, mc <-chan draw.Mouse) {
	r := dragRect(ctxt.cvs, m, mc).Add(ctxt.mouseDelta())
	ctxt.push(ctxt.f.Zoom(r))
}

func (ctxt *context) zoomABit(m draw.Mouse, mc <-chan draw.Mouse) {
	// TODO: zoom into a rectangle centred on the mouse position,
	// but half the size of the current canvas rectangle
	log.Stdoutf("zoom a bit")
}

func (ctxt *context) julia(p draw.Point) {
	if f := ctxt.f.Associated(p.Add(ctxt.mouseDelta())); f != nil {
		ctxt.push(f)
	}
}

const ThumbSize = 150

func (ctxt *context) interactiveJulia(m draw.Mouse, mc <-chan draw.Mouse) {
	var i canvas.ImageItem
	i.IsOpaque = true
	i.R = draw.Rect(0, 0, ThumbSize, ThumbSize).Add(ctxt.cvs.Rect().Max).Sub(draw.Pt(ThumbSize, ThumbSize))
	i.Image = image.NewRGBA(image.Rect(0, 0, ThumbSize, ThumbSize))
	ctxt.cvs.AddItem(&i)
	defer func() {
		ctxt.cvs.Delete(&i)
		ctxt.cvs.Flush()
	}()
	delta := ctxt.mouseDelta()
	for {
		f := ctxt.f.Associated(m.Point.Add(delta))
		if f == nil {
			for m.Buttons != 0 {
				m = <-mc
			}
			return
		}
		r := draw.Rect(0, 0, ThumbSize, ThumbSize)
		f = f.Resize(r)
		ctxt.cvs.Atomically(func(flush canvas.FlushFunc) {
			NewTile(r, f, i.Image.(*image.RGBA), true)
			flush(i.Bbox(), nil)
		})
		ctxt.cvs.Flush()
		if m.Buttons == 0 {
			ctxt.julia(m.Point)
			return
		}
		m = <-mc
	}
}

func (ctxt *context) setFractal(f Fractal, centre draw.Point) {
	if ctxt.item != nil {
		ctxt.tiler.Stop()
		ctxt.cvs.Delete(ctxt.item)
	}
	ctxt.f = f
	ctxt.tiler = NewTiler(f, ctxt.cache, *tileSize)
	ctxt.item = canvas.Draggable(canvas.Moveable(ctxt.tiler))
	ctxt.item.SetCentre(centre)
	ctxt.cvs.AddItem(ctxt.item)
	ctxt.cvs.Flush()
}

// push pushes a new item onto the zoom stack.
func (ctxt *context) push(f Fractal) {
	ctxt.pushed = &stack{ctxt.f, centre(ctxt.item.Bbox()), ctxt.iterations, ctxt.pushed}
	ctxt.setFractal(f, draw.ZP)
}

// pop pops one level off the current zoom stack
func (ctxt *context) pop() {
	if ctxt.pushed == nil {
		return
	}
	ctxt.setFractal(ctxt.pushed.f, ctxt.pushed.centre)
	ctxt.iterations = ctxt.pushed.iterations
	ctxt.pushed = ctxt.pushed.next
}

// mouseDelta returns the vector from mouse coords to coords inside
// the fractal item.
func (ctxt *context) mouseDelta() draw.Point {
	return draw.ZP.Sub(centre(ctxt.item.Bbox()))
}

type ColorRange struct {
	start, end color.Color
	p          float64
}

var (
	DarkYellow   = image.Uniform{color.RGBA{0xee, 0xee, 0x9e, 0xff}}
	DarkGreen    = image.Uniform{color.RGBA{0x44, 0x88, 0x44, 0xff}}
	PaleGreyBlue = image.Uniform{color.RGBA{0x49, 0x93, 0xDD, 0xFF}}
)

var range0 = []ColorRange{
	ColorRange{DarkYellow, DarkGreen, 0.25},
	ColorRange{DarkGreen, image.Cyan, 0.25},
	ColorRange{image.Cyan, image.Red, 0.25},
	ColorRange{image.Red, image.White, 0.125},
	ColorRange{image.White, PaleGreyBlue, 0.125},
}

func interpolateColor(c1, c2 color.Color, where float64) color.Color {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()

	var c color.RGBA64
	c.R = uint16(float64(r2-r1)*where + float64(r1) + 0.5)
	c.G = uint16(float64(g2-g1)*where + float64(g1) + 0.5)
	c.B = uint16(float64(b2-b1)*where + float64(b1) + 0.5)
	c.A = uint16(float64(a2-a1)*where + float64(a1) + 0.5)
	return c
}

func makePalette(spec []ColorRange, nsteps int) []color.RGBA {
	palette := make([]color.RGBA, nsteps+1)
	p := 0
	for _, r := range spec {
		n := int(r.p*float64(nsteps) + 0.5)
		for j := 0; j < n && p < nsteps; j++ {
			c := interpolateColor(r.start, r.end, float64(j)/float64(n))
			palette[p] = color.RGBAModel.Convert(c).(color.RGBA)
			p++
		}
	}
	palette[nsteps] = color.RGBAModel.Convert(image.Black).(color.RGBA)
	return palette
}

type Mandelbrot struct {
	iterations    int
	palette       []color.RGBA
	origin, delta complex128
	cr            crect
	r             draw.Rectangle
	jpoint        complex128 // number characterising julia set.
	julia         bool
}

type crect struct {
	min, max complex128
}

// NewMandelbrot returns a mandelbrot-set calculator
// that shows at least the area r within wr.
//
func NewMandelbrot(r crect, wr draw.Rectangle, julia bool, jpoint complex128, iterations int) *Mandelbrot {
	btall := float64(wr.Dy()) / float64(wr.Dx())
	atall := (imag(r.min) - imag(r.min)) / (real(r.max) - real(r.min))
	if btall > atall {
		// bitmap is taller than area, so expand area vertically
		excess := (real(r.max)-real(r.min))*btall - (imag(r.max) - imag(r.min))
		r.min -= cmplx(0, excess/2.0)
		r.max += cmplx(0, excess/2.0)
	} else {
		// area is taller than bitmap, so expand area horizontally
		excess := (imag(r.max)-imag(r.min))/btall - (real(r.max) - real(r.min))
		r.min -= cmplx(excess/2.0, 0)
		r.max += cmplx(excess/2.0, 0)
	}
	var m Mandelbrot
	m.iterations = iterations
	m.palette = makePalette(range0, iterations)
	m.origin = r.min
	m.julia = julia
	m.jpoint = jpoint
	m.cr = r
	m.delta = cmplx(
		(real(r.max)-real(r.min))/float64(wr.Dx()),
		(imag(r.max)-imag(r.min))/float64(wr.Dy()),
	)
	m.r = wr
	return &m
}

func (m *Mandelbrot) translate(p draw.Point) complex128 {
	return m.origin + cmplx(float64(p.X)*real(m.delta), float64(p.Y)*imag(m.delta))
}

func (m *Mandelbrot) Zoom(r draw.Rectangle) Fractal {
	return NewMandelbrot(crect{m.translate(r.Min), m.translate(r.Max)}, m.r, m.julia, m.jpoint, m.iterations)
}

func (m *Mandelbrot) Resize(r draw.Rectangle) Fractal {
	return NewMandelbrot(m.cr, r, m.julia, m.jpoint, m.iterations)
}

func (m *Mandelbrot) At(pt draw.Point) (col color.RGBA) {
	p := cmplx(float64(pt.X)*real(m.delta), float64(pt.Y)*imag(m.delta)) + m.origin

	const max = 4

	z := p
	c := p
	if m.julia {
		c = m.jpoint
	}
	n := m.iterations
	for i := 0; i < n; i++ {
		z = z*z + c
		if real(z)*real(z)+imag(z)*imag(z) > max {
			return m.palette[i]
		}
	}
	return m.palette[n]
}

func (m *Mandelbrot) Associated(pt draw.Point) Fractal {
	if m.julia {
		return nil
	}
	return NewMandelbrot(topArea, m.r, true, m.translate(pt), m.iterations)
}

type Tile struct {
	stop chan<- (chan<- int)
	done <-chan int
	r    draw.Rectangle // rectangle in pixels covered by tile
	//	lastDrawn int	// time stamp last drawn at.
	nrows int // number of rows calculated so far, from top.
	image *image.RGBA
	calc  Fractal
	next  *Tile
}

func NewTile(r draw.Rectangle, calc Fractal, img *image.RGBA, wait bool) *Tile {
	t := new(Tile)
	t.r = r
	t.nrows = 0
	if img == nil {
		img = image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	}
	t.calc = calc
	t.image = img
	if wait {
		t.calculate(nil, nil)
		t.nrows = img.Height()
	} else {
		// choose some vaguely appropriate colour
		col := calc.At(centre(r))
		draw.Draw(t.image, draw.Rect(0, 0, r.Dx(), r.Dy()), image.Uniform{col}, draw.ZP)
	}
	return t
}

func (t *Tile) Go(updatec chan<- draw.Rectangle) {
	if t.nrows >= t.image.Height() || t.stop != nil {
		return
	}
	if t.done != nil {
		t.nrows = <-t.done
		t.done = nil
	}
	stop := make(chan (chan<- int), 1)
	t.stop = stop
	go t.calculate(updatec, stop)
}

func (t *Tile) calculate(updatec chan<- draw.Rectangle, stop <-chan (chan<- int)) {
	y0 := t.nrows + t.r.Min.Y
	for y := t.r.Min.Y + t.nrows; y < t.r.Max.Y; {
		row := t.image.Pixel[y-t.r.Min.Y]
		for i := range row {
			row[i] = t.calc.At(draw.Point{i + t.r.Min.X, y})
		}
		y++
		if updatec != nil && y&3 == 0 {
			select {
			case updatec <- draw.Rect(t.r.Min.X, y0, t.r.Max.X, y):
				y0 = y
			case done := <-stop:
				done <- y - t.r.Min.Y
				return
			}
		}
	}
	if updatec != nil {
		updatec <- t.r
		<-stop <- t.image.Height()
	}
}

func (t *Tile) Stop() {
	if t.stop == nil {
		return
	}
	done := make(chan int, 1)
	t.done = done
	t.stop <- done
	t.stop = nil
}

func (t *Tile) Draw(dst draw.Image, clipr draw.Rectangle) {
	r := t.r.Clip(clipr)
	draw.DrawMask(dst, r, t.image, r.Min.Sub(t.r.Min), nil, draw.ZP, draw.Src)
}

type tileTable map[int64]*Tile

func NewTileTable() tileTable {
	return make(map[int64]*Tile)
}

func hash(x, y int) int64 {
	return int64(x) + int64(y)<<32
}

func (table tileTable) Get(x, y int, f Fractal) *Tile {
	h := hash(x, y)
	for t := table[h]; t != nil; t = t.next {
		if t.r.Min.X == x && t.r.Min.Y == y && t.calc == f {
			return t
		}
	}
	return nil
}

func (table tileTable) Set(x, y int, f Fractal, t *Tile) {
	h := hash(x, y)
	for t := table[h]; t != nil; t = t.next {
		if t.r.Min.X == x && t.r.Min.Y == y && t.calc == f {
			panic("cannot change tiletable")
		}
	}
	t.next = table[h]
	table[h] = t
}

type Tiler struct {
	backing    canvas.Backing
	all        tileTable
	current    map[*Tile]bool
	tileSize   int
	calc       Fractal
	updatec    chan draw.Rectangle
	drawerDone chan bool
}

// NewTiler creates a new object that tiles the
// results of calc across an arbitrarily large
// space. The centre of its bounding box is always
// (0, 0).
//
func NewTiler(calc Fractal, cache tileTable, tileSize int) *Tiler {
	t := &Tiler{}
	t.all = cache
	t.current = make(map[*Tile]bool)
	t.calc = calc
	t.tileSize = tileSize
	t.updatec = make(chan draw.Rectangle, 50)
	t.drawerDone = make(chan bool)
	go t.drawer()
	return t
}

func (t *Tiler) Stop() {
	// atomically?
	for tile := range t.current {
		tile.Stop()
	}
	close(t.updatec)
	<-t.drawerDone
	t.current = nil
}

func (t *Tiler) drawer() {
	for {
		r, ok := <-t.updatec
		if !ok {
			break
		}
		t.backing.Atomically(func(flush canvas.FlushFunc) {
			for ok {
				flush(r, nil)
				select {
				case r, ok = <-t.updatedc:
				default:
				}
			}
		})
		t.backing.Flush()
	}
	t.drawerDone <- false
}

func (t *Tiler) SetContainer(b canvas.Backing) {
	t.backing = b
}

func (t *Tiler) HitTest(p draw.Point) bool {
	return true
}

func (t *Tiler) Opaque() bool {
	return true
}

//problem:
//we want to be drawing independently here, but
//the canvas code does not want us to use an
//image or a backing after the backing has been
//changed.
//so there's a race: a Tiler.Draw request could
//be sending us a new image at this moment,
//just as we're about to enter Atomically.
//
//then we enter Atomically and we deadlock.
//
//perhaps this is fundamental problem with allowing
//items to move between containers...

func roundDown(x, size int) int {
	y := x / size
	if x < 0 {
		y--
	}
	return y * size
}

func (t *Tiler) Draw(dst draw.Image, clipr draw.Rectangle) {
	if t.current == nil {
		panic("no current")
	}
	min := draw.Point{
		roundDown(clipr.Min.X, t.tileSize),
		roundDown(clipr.Min.Y, t.tileSize),
	}
	var p draw.Point
	for p.Y = min.Y; p.Y < clipr.Max.Y; p.Y += t.tileSize {
		for p.X = min.X; p.X < clipr.Max.X; p.X += t.tileSize {
			tile := t.all.Get(p.X, p.Y, t.calc)
			if tile == nil {
				tile = NewTile(
					draw.Rect(p.X, p.Y, p.X+t.tileSize, p.Y+t.tileSize),
					t.calc,
					nil,
					false,
				)
				t.all.Set(p.X, p.Y, t.calc, tile)
				t.current[tile] = true
				tile.Go(t.updatec)
			} else if !t.current[tile] {
				tile.Go(t.updatec)
			}
			tile.Draw(dst, clipr)
		}
	}
}

func (t *Tiler) Bbox() draw.Rectangle {
	return draw.Rect(-100000000, -100000000, 100000000, 100000000)
}

// this will go.
type RectFlusherContext interface {
	draw.Context
	FlushImageRect(r draw.Rectangle)
}

func flushFunc(ctxt draw.Context) func(r draw.Rectangle) {
	if fctxt, ok := ctxt.(RectFlusherContext); ok {
		return func(r draw.Rectangle) {
			fctxt.FlushImageRect(r)
		}
	}
	return func(_ draw.Rectangle) {
		ctxt.FlushImage()
	}
}

func centre(r draw.Rectangle) draw.Point {
	return draw.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

type box struct {
	n, e, s, w *canvas.Line
}

func newBox(cvs *canvas.Canvas, r draw.Rectangle) *box {
	var b box
	red := image.Uniform{image.Red}
	b.n = canvas.NewLine(red, r.Min, draw.Pt(r.Max.X, r.Min.Y), 1)
	b.e = canvas.NewLine(red, draw.Pt(r.Max.X, r.Min.Y), r.Max, 1)
	b.s = canvas.NewLine(red, r.Max, draw.Pt(r.Min.X, r.Max.Y), 1)
	b.w = canvas.NewLine(red, draw.Pt(r.Min.X, r.Max.Y), r.Min, 1)
	cvs.AddItem(b.n)
	cvs.AddItem(b.e)
	cvs.AddItem(b.s)
	cvs.AddItem(b.w)
	return &b
}

func (b *box) setRect(r draw.Rectangle) {
	b.n.SetEndPoints(r.Min, draw.Pt(r.Max.X, r.Min.Y))
	b.e.SetEndPoints(draw.Pt(r.Max.X, r.Min.Y), r.Max)
	b.s.SetEndPoints(r.Max, draw.Pt(r.Min.X, r.Max.Y))
	b.w.SetEndPoints(draw.Pt(r.Min.X, r.Max.Y), r.Min)
}

func (b *box) delete(cvs *canvas.Canvas) {
	cvs.Delete(b.n)
	cvs.Delete(b.e)
	cvs.Delete(b.s)
	cvs.Delete(b.w)
}

func dragRect(cvs *canvas.Canvas, m draw.Mouse, mc <-chan draw.Mouse) draw.Rectangle {
	m0 := m
	r := draw.Rectangle{m0.Point, m0.Point}
	b := newBox(cvs, r)
	cvs.Flush()
	for m.Buttons != 0 {
		m = <-mc
		b.setRect(draw.Rectangle{m0.Point, m.Point}.Canon())
		cvs.Flush()
	}
	b.delete(cvs)
	cvs.Flush()
	return draw.Rectangle{m0.Point, m.Point}.Canon()
}

const ClickDist = 4
const ClickTime = 0.3e9

// clicker handles possibly multiple click mouse actions.
// It should be called with the first mouse event that triggered
// the action (which should have m.Buttons != 0), and the
// channel from which it will read mouse events.
// It sends a mouse event on c for each click;
// and closes c when no more clicks are available.
// If the last event has Buttons == 0, the mouse
// has been released, otherwise the user continues
// to drag the mouse. Only the last event may have Buttons==0.
//
func clicker(m0 draw.Mouse, mc <-chan draw.Mouse) (clicks, final <-chan draw.Mouse) {
	var t *time.Ticker
	c := make(chan draw.Mouse)
	fc := make(chan draw.Mouse, 1)
	go func() {
		c <- m0
		m := m0
	tracking:
		for !closed(mc) {
			// wait for button up or delta or time to move outside limit.
			for m = range mc {
				if m.Buttons == 0 {
					// does a long click with no intervening movement still count as a click?
					break
				}
				d := m.Sub(m0.Point)
				if m.Nsec-m0.Nsec > ClickTime || d.X*d.X+d.Y*d.Y > ClickDist {
					break tracking
				}
			}

			t = time.NewTicker(ClickTime)
			// wait for button down or delta or time to move outside limit.
		buttonDown:
			for {
				select {
				case m = <-mc:
					if closed(mc) {
						break tracking
					}
					d := m.Sub(m0.Point)
					if m.Nsec-m0.Nsec > ClickTime || d.X*d.X+d.Y*d.Y > ClickDist {
						break tracking
					}
					if m.Buttons != 0 {
						break buttonDown
					}
				case <-t.C:
					break tracking
				}
			}
			t.Stop()
			t = nil
			c <- m0
			m0 = m
		}
		if t != nil {
			t.Stop()
		}
		close(c)
		fc <- m
		return
	}()
	return c, fc
}
