package main

import (
	"code.google.com/p/freetype-go/freetype/raster"
	"code.google.com/p/rog-go/canvas"
	"code.google.com/p/rog-go/values"
	"exp/draw"
	"exp/draw/x11"
	"fmt"
	"image"
	"image/color"
	"log"
)

var cvs *canvas.Canvas

func main() {
	ctxt, err := x11.NewWindow()
	if ctxt == nil {
		log.Fatalf("no window: %v", err)
	}
	screen := ctxt.Screen()

	bg := canvas.NewBackground(screen.(*image.RGBA), image.White, flushFunc(ctxt))
	cvs = canvas.NewCanvas(nil, bg.Rect())
	bg.SetItem(cvs)
	ec := ctxt.EventChan()
	cvs.Flush()

	for {
		select {
		case e := <-ec:
			switch e := e.(type) {
			case nil:
				if closed(ec) {
					log.Fatal("quitting")
					return
				}
			case draw.MouseEvent:
				if e.Buttons == 0 {
					break
				}
				if cvs.HandleMouse(cvs, e, ec) {
					fmt.Printf("handled mouse\n")
					break
				}
				fmt.Printf("raster maker\n")
				rasterMaker(e, ec)
			}
		}
	}
}

func nextMouse(ec <-chan interface{}) (m draw.MouseEvent) {
	for {
		e := <-ec
		switch e := (e).(type) {
		case draw.MouseEvent:
			return e
		case nil:
			if closed(ec) {
				return
			}
		}
	}
	return
}

func color_(r, g, b, a uint8) *image.Uniform {
	return &image.Uniform{color.RGBA{r, g, b, a}}
}

// click anywhere to create a new raster item.
// then but-2 to add another point (twice for Add2, thrice for Add3)
// but-3 to finish.
func rasterMaker(m draw.MouseEvent, ec <-chan interface{}) {
	obj := newRasterPlay()
	defer obj.SetControlPointColor(color_(0x80, 0x80, 0x80, 0xff))
	obj.AddPoint(true, m.Loc)
	cvs.AddItem(obj)
	cvs.Flush()

	for {
		// wait for button press to be released, if the user was dragging
		for m.Buttons != 0 {
			m = nextMouse(ec)
		}
		// wait for button press
		for m.Buttons == 0 {
			if closed(ec) {
				return
			}
			m = nextMouse(ec)
		}
		if m.Buttons&4 != 0 {
			return
		}
		fmt.Printf("add point %v\n", m.Loc)
		obj.AddPoint(m.Buttons&2 == 0, m.Loc)
		cvs.Flush()
	}
}

const size = 4

type ControlPoint struct {
	backing  canvas.Backing
	p        image.Point
	col      *image.Uniform
	value    values.Value // Point Value
	colValue values.Value // draw.Color Value
}

type moveEvent struct {
	i int         // index of control point that's moved
	p image.Point // where it's moved to
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
	cp.col = image.Black
	cp.colValue = colValue
	cp.backing = (*canvas.Canvas)(nil)
	go cp.listener()
	return canvas.Draggable(cp)
}

func (cp *ControlPoint) SetCentre(p image.Point) {
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
				cp.p = p.(image.Point)
				flush(r, nil)
				flush(cp.Bbox(), nil)
			})
		case col := <-colc:
			if closed(colc) {
				colc = nil
				break
			}
			cp.backing.Atomically(func(flush canvas.FlushFunc) {
				cp.col = col.(*image.Uniform)
				flush(cp.Bbox(), nil)
			})
		}
		cp.backing.Flush()
	}
}

func (cp *ControlPoint) Bbox() image.Rectangle {
	return image.Rectangle{cp.p, cp.p}.Inset(-size)
}

func (cp *ControlPoint) HitTest(p image.Point) bool {
	return cp.Bbox().Contains(p)
}

func (cp *ControlPoint) Opaque() bool {
	return opaqueColor(cp.col)
}

func (cp *ControlPoint) SetContainer(b canvas.Backing) {
	// XXX race with listener - should inform listener synchronously
	cp.backing = b
}

func (cp *ControlPoint) Draw(dst draw.Image, clipr image.Rectangle) {
	r := clipr.Intersect(cp.Bbox())
	draw.Draw(dst, r, cp.col, image.ZP)
}

func (obj *rasterPlay) AddPoint(new bool, p image.Point) {
	value := values.NewValue(p)
	cp := NewControlPoint(value, obj.colValue)
	n := len(obj.points)
	obj.points = obj.points[0 : n+1]
	obj.points[n] = rpoint{new, pixel2fixPoint(p)}
	go func() {
		for xp := range value.Iter() {
			obj.moved <- moveEvent{n, xp.(image.Point)}
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
	obj.raster.SetContainer(b)
	if b != nil {
		obj.makeOutline()
	}
	obj.HandlerItem.SetContainer(b)
}

func (obj *rasterPlay) SetControlPointColor(col *image.Uniform) {
	obj.colValue.Set(col)
}

var blue = &image.Uniform{color.RGBA{0, 0, 0xff, 0xff}}

func newRasterPlay() *rasterPlay {
	obj := new(rasterPlay)
	obj.points = make([]rpoint, 0, 100) // expansion later
	obj.moved = make(chan moveEvent)
	obj.raster.SetFill(&image.Uniform{image.AlphaMultiply(blue, 0x8000)})
	obj.c = canvas.NewCanvas(nil, cvs.Bbox())
	obj.colValue = values.NewValue(nil)
	obj.colValue.Set(image.Black)
	obj.HandlerItem = obj.c
	obj.c.AddItem(&obj.raster)
	go obj.listener()
	return obj
}

const (
	fixBits  = 8
	fixScale = 1 << fixBits // matches raster.Fixed
)

func pixel2fixPoint(p image.Point) raster.Point {
	return raster.Point{raster.Fix32(p.X << fixBits), raster.Fix32(p.Y << fixBits)}
}

// this will go.
type RectFlusherWindow interface {
	draw.Window
	FlushImageRect(r image.Rectangle)
}

func flushFunc(ctxt draw.Window) func(r image.Rectangle) {
	if fctxt, ok := ctxt.(RectFlusherWindow); ok {
		return func(r image.Rectangle) {
			fctxt.FlushImageRect(r)
		}
	}
	return func(_ image.Rectangle) {
		ctxt.FlushImage()
	}
}

func opaqueColor(col color.Color) bool {
	_, _, _, a := col.RGBA()
	return a == 0xffff
}
