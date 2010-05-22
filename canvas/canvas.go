// Package canvas layers a set of movable objects onto
// a draw.Image. Objects in the canvas may be created,
// moved and deleted; the Canvas manages any necessary
// re-drawing.
package canvas

import (
	"exp/draw"
	"image"
	//	"fmt"
	"sync"
)

type Canvas struct {
	lock       sync.Mutex
	dst        *image.RGBA
	flushrect  draw.Rectangle
	r          draw.Rectangle // for convenience, the bounding rectangle of the dst
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
// risk of drawing anomalies. Flush can be called
// to flush dirty areas of the canvas.
//
func (c *Canvas) Atomic(f func(flush func(r draw.Rectangle))) {
	// could pre-allocate inside c if we cared.
	flush := func(r draw.Rectangle) {
		c.addFlush(r)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	f(flush)
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


type sizer interface {
	Width() int
	Height() int
}

func size(x sizer) draw.Point {
	return draw.Point{x.Width(), x.Height()}
}

func eqrect(r0, r1 draw.Rectangle) bool {
	return r0.Min.Eq(r1.Min) && r0.Max.Eq(r1.Max)
}
