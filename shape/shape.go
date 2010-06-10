package main

import (
	"rog-go.googlecode.com/hg/x11"
	"exp/draw"
	"log"
	"image"
	"rog-go.googlecode.com/hg/canvas"
	"freetype-go.googlecode.com/hg/freetype/raster"
)

var window *canvas.Canvas

func main() {
	ctxt, err := x11.NewWindow()
	if ctxt == nil {
		log.Exitf("no window: %v", err)
	}
	screen := ctxt.Screen()

	draw.Draw(screen, draw.Rect(100, 100, 200, 200), draw.Red, draw.ZP)

	bg := canvas.NewBackground(screen.(*image.RGBA), draw.White, flushFunc(ctxt))
	window = canvas.NewCanvas(bg, nil, draw.Rect(0, 0, bg.Width(), bg.Height()))
	bg.SetItem(window)
	qc := ctxt.QuitChan()
	kc := ctxt.KeyboardChan()
	mc := ctxt.MouseChan()
	window.Flush()

	for {
		select {
		case <-qc:
			log.Exit("quitting")
			return
		case m := <-mc:
			if m.Buttons == 0 {
				break
			}
			if window.HandleMouse(window, m, mc) {
				break
			}
			rasterMaker(m, mc)
		case <-kc:
		}
	}
}

//but-1 double-click anywhere to create a new raster item.
//then but-2 to add another point (twice for Add2, thrice for Add3)
//but-1 to start. but-3 to finish.
func rasterMaker(m draw.Mouse, mc <-chan draw.Mouse) {
	obj := newRasterPlay()
	obj.AddPoint(true, m.Point)
	window.AddItem(obj)
	window.Flush()

	for {
		// wait for button press to be released, if the user was dragging
		for m.Buttons != 0 {
			m = <-mc
		}
		// wait for button press
		for m.Buttons == 0 {
			if closed(mc) {
				return
			}
			m = <-mc
		}
		if m.Buttons&4 != 0{
			return
		}
		obj.AddPoint(m.Buttons&2 == 0, m.Point)
		window.Flush()
	}
}


const size = 4
type ControlPoint struct {
	backing canvas.Backing
	p draw.Point
	value canvas.Value		// Point value
}

type moveEvent struct {
	i int			// index of control point that's moved
	p draw.Point	// where it's moved to
}

type rpoint struct {
	new bool
	raster.Point
}

type rasterPlay struct {
	canvas.HandlerItem
	c *canvas.Canvas
	points []rpoint
	moved chan moveEvent
	raster canvas.RasterItem
}

func NewControlPoint(value canvas.Value) canvas.Item {
	cp := new(ControlPoint)
	cp.value = value
	cp.backing = (*canvas.Canvas)(nil)
	go cp.listener()
	return canvas.Draggable(cp)
}

func (cp *ControlPoint) SetCentre(p draw.Point) {
	cp.value.Set(p)
}

func (cp *ControlPoint) listener() {
	for xp := range cp.value.Iter() {
		cp.backing.Atomically(func(flush canvas.FlushFunc){
			r := cp.Bbox()
			cp.p = xp.(draw.Point)
			flush(r, nil)
			flush(cp.Bbox(), nil)
		})
		cp.backing.Flush()
	}
}

func (cp *ControlPoint) Bbox() draw.Rectangle {
	return draw.Rectangle{cp.p, cp.p}.Inset(-size)
}

func (cp *ControlPoint) HitTest(p draw.Point) bool {
	return p.In(cp.Bbox())
}

func (cp *ControlPoint) Opaque() bool {
	return true
}

func (cp *ControlPoint) SetContainer(b canvas.Backing) {
	// XXX race with listener - should inform listener synchronously
	cp.backing = b
}

func (cp *ControlPoint) Draw(dst *image.RGBA, clipr draw.Rectangle) {
	r  := clipr.Clip(cp.Bbox())
	draw.Draw(dst, r, draw.Black, draw.ZP)
}

func (obj *rasterPlay) AddPoint(new bool, p draw.Point) {
	value := canvas.NewValue(p)
	cp := NewControlPoint(value)
	n := len(obj.points)
	obj.points = obj.points[0:n+1]
	obj.points[n] = rpoint{new, pixel2fixPoint(p)}
	go func() {
		for xp := range value.Iter() {
			obj.moved <- moveEvent{n, xp.(draw.Point)}
		}
	}()
	obj.c.Atomically(func(flush canvas.FlushFunc){
		r := obj.raster.Bbox()
		obj.rasterize()
		flush(r, nil)
		flush(obj.raster.Bbox(), nil)
	})
	obj.c.AddItem(cp)
}

func addSome(raster *canvas.RasterItem, p []raster.Point) {
	switch len(p) {
	case 1:
		raster.Add1(p[0])
	case 2:
		raster.Add2(p[0], p[1])
	case 3:
		raster.Add3(p[0], p[1], p[2])
	}
}

func (obj *rasterPlay) rasterize() {
	obj.raster.Clear()
	if len(obj.points) > 0 {
		var accum [3]raster.Point
		n := 0
		obj.raster.Start(obj.points[0].Point)
		for i, p := range obj.points[1:] {
			if p.new || n == len(accum) || i + 1 == len(obj.points) - 1 {
				addSome(&obj.raster, accum[0:n])
				n = 0
			}
			accum[n] = p.Point
			n++
		}
		addSome(&obj.raster, accum[0:n])
	}
	obj.raster.CalcBbox()
}

func (obj *rasterPlay) listener() {
	for m := range obj.moved {
		obj.c.Atomically(func(flush canvas.FlushFunc){
			r := obj.raster.Bbox()
			obj.points[m.i].Point = pixel2fixPoint(m.p)
			obj.rasterize()
			flush(r, nil)
			flush(obj.raster.Bbox(), nil)
		})
		obj.c.Flush()
	}
}

func (obj *rasterPlay) SetContainer(b canvas.Backing) {
	if b != nil {
		obj.raster.SetBounds(b.Width(), b.Height())
		obj.rasterize()
	}
	obj.HandlerItem.SetContainer(b)
}


func newRasterPlay() *rasterPlay {
	obj := new(rasterPlay)
	obj.points = make([]rpoint, 0, 100)		// expansion later
	obj.moved = make(chan moveEvent)
	obj.raster.SetColor(draw.Color(0x808080ff).SetAlpha(0x80))
	obj.c = canvas.NewCanvas(window, nil, window.Bbox())
	obj.HandlerItem = obj.c
	obj.c.AddItem(&obj.raster)
	go obj.listener()
	return obj
}

const (
	fixBits  = 8
	fixScale = 1 << fixBits // matches raster.Fixed
)

func pixel2fixPoint(p draw.Point) raster.Point {
	return raster.Point{raster.Fixed(p.X << fixBits), raster.Fixed(p.Y << fixBits)}
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
