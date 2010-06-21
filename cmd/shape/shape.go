package main

import (
	"rog-go.googlecode.com/hg/x11"
	"exp/draw"
	"log"
	"image"
	"rog-go.googlecode.com/hg/canvas"
	"rog-go.googlecode.com/hg/values"
	"freetype-go.googlecode.com/hg/freetype/raster"
)

var cvs *canvas.Canvas

func main() {
	ctxt, err := x11.NewWindow()
	if ctxt == nil {
		log.Exitf("no window: %v", err)
	}
	screen := ctxt.Screen()

	bg := canvas.NewBackground(screen.(*image.RGBA), draw.White, flushFunc(ctxt))
	cvs = canvas.NewCanvas(nil, bg.Rect())
	bg.SetItem(cvs)
	qc := ctxt.QuitChan()
	kc := ctxt.KeyboardChan()
	mc := ctxt.MouseChan()
	cvs.Flush()

	for {
		select {
		case <-qc:
			log.Exit("quitting")
			return
		case m := <-mc:
			if m.Buttons == 0 {
				break
			}
			if cvs.HandleMouse(cvs, m, mc) {
				break
			}
			rasterMaker(m, mc)
		case <-kc:
		}
	}
}

// click anywhere to create a new raster item.
// then but-2 to add another point (twice for Add2, thrice for Add3)
// but-3 to finish.
func rasterMaker(m draw.Mouse, mc <-chan draw.Mouse) {
	obj := newRasterPlay()
	defer obj.SetControlPointColor(0x808080ff)
	obj.AddPoint(true, m.Point)
	cvs.AddItem(obj)
	cvs.Flush()

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
		if m.Buttons&4 != 0 {
			return
		}
		obj.AddPoint(m.Buttons&2 == 0, m.Point)
		cvs.Flush()
	}
}


const size = 4

type ControlPoint struct {
	backing  canvas.Backing
	p        draw.Point
	col      draw.Color
	value    values.Value // Point Value
	colValue values.Value // draw.Color Value
}

type moveEvent struct {
	i int        // index of control point that's moved
	p draw.Point // where it's moved to
}

type rpoint struct {
	new bool
	raster.Point
}

type rasterPlay struct {
	colValue values.Value
	canvas.HandlerItem
	c      *canvas.Canvas
	points []rpoint
	moved  chan moveEvent
	raster canvas.RasterItem
}

func NewControlPoint(value, colValue values.Value) canvas.Item {
	cp := new(ControlPoint)
	cp.value = value
	cp.colValue = colValue
	cp.backing = (*canvas.Canvas)(nil)
	go cp.listener()
	return canvas.Draggable(cp)
}

func (cp *ControlPoint) SetCentre(p draw.Point) {
	cp.value.Set(p)
}

func (cp *ControlPoint) listener() {
	pc := cp.value.Iter()
	colc := cp.colValue.Iter()
	for pc != nil && colc != nil {
		select {
		case p := <-pc:
			if closed(pc) {
				pc = nil
				break
			}
			cp.backing.Atomically(func(flush canvas.FlushFunc) {
				r := cp.Bbox()
				cp.p = p.(draw.Point)
				flush(r, nil)
				flush(cp.Bbox(), nil)
			})
		case col := <-colc:
			if closed(colc) {
				colc = nil
				break
			}
			cp.backing.Atomically(func(flush canvas.FlushFunc) {
				cp.col = col.(draw.Color)
				flush(cp.Bbox(), nil)
			})
		}
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
	return opaqueColor(cp.col)
}

func (cp *ControlPoint) SetContainer(b canvas.Backing) {
	// XXX race with listener - should inform listener synchronously
	cp.backing = b
}

func (cp *ControlPoint) Draw(dst draw.Image, clipr draw.Rectangle) {
	r := clipr.Clip(cp.Bbox())
	draw.Draw(dst, r, cp.col, draw.ZP)
}

func (obj *rasterPlay) AddPoint(new bool, p draw.Point) {
	value := values.NewValue(p)
	cp := NewControlPoint(value, obj.colValue)
	n := len(obj.points)
	obj.points = obj.points[0 : n+1]
	obj.points[n] = rpoint{new, pixel2fixPoint(p)}
	go func() {
		for xp := range value.Iter() {
			obj.moved <- moveEvent{n, xp.(draw.Point)}
		}
	}()
	obj.c.Atomically(func(flush canvas.FlushFunc) {
		r := obj.raster.Bbox()
		obj.makeOutline()
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

func (obj *rasterPlay) makeOutline() {
	obj.raster.Clear()
	if len(obj.points) > 0 {
		var accum [3]raster.Point
		n := 0
		obj.raster.Start(obj.points[0].Point)
		for i, p := range obj.points[1:] {
			if p.new || n == len(accum) || i+1 == len(obj.points)-1 {
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
		obj.c.Atomically(func(flush canvas.FlushFunc) {
			r := obj.raster.Bbox()
			obj.points[m.i].Point = pixel2fixPoint(m.p)
			obj.makeOutline()
			flush(r, nil)
			flush(obj.raster.Bbox(), nil)
		})
		obj.c.Flush()
	}
}

func (obj *rasterPlay) SetContainer(b canvas.Backing) {
	if b != nil {
		size := b.Rect().Max
		obj.raster.SetBounds(size.X, size.Y)
		obj.makeOutline()
	}
	obj.HandlerItem.SetContainer(b)
}

func (obj *rasterPlay) SetControlPointColor(col draw.Color) {
	obj.colValue.Set(col)
}

func newRasterPlay() *rasterPlay {
	obj := new(rasterPlay)
	obj.points = make([]rpoint, 0, 100) // expansion later
	obj.moved = make(chan moveEvent)
	obj.raster.SetFill(image.ColorImage{draw.Color(0x808080ff).SetAlpha(0x80)})
	obj.c = canvas.NewCanvas(nil, cvs.Bbox())
	obj.colValue = values.NewValue(draw.Black)
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

func opaqueColor(col image.Color) bool {
	_, _, _, a := col.RGBA()
	return a == 0xffff
}
