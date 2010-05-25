// Package canvas layers a set of movable objects onto
// a draw.Image. Objects in the canvas may be created,
// moved and deleted; the Canvas manages any necessary
// re-drawing.
package canvas

import (
	"container/list"
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
	items    list.List 	// foreground objects are at the end of the list
	flushFunc  func(r draw.Rectangle)
}

type canvasItem struct {
	item	Item
	object Object				// object associated with the item.
}

// Values that implement the Item interface may be added
// to the canvas. They should adhere to the following rules:
// - No calls to the canvas should be made while in the Draw, Bbox
// or HitTest methods.
// - All changes to the appearance of the object should be made using
// the Atomically function.
//
type Item interface {
	Draw(img *image.RGBA, clip draw.Rectangle)
	Bbox() draw.Rectangle
	HitTest(p draw.Point) bool
}

// Each Item may be associated with an handler Object.
// More than one Item may be associated with the same
// Object. If the canvas's HandleMouse method is
// called, and HitTest succeeds on an item, then
// HandleMouse will be invoked on its associated Object.
//
type Object interface {
	HandleMouse(item Item, m draw.Mouse, mc <-chan draw.Mouse) bool
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
//	defer func() {
//		if !c.flushrect.Eq(c.flushrect.Canon() {
//			panic("setting non-canonical flushrect")
//		}
//	}()
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
	for e := c.items.Front(); e != nil; e = e.Next() {
		ci := e.Value.(canvasItem)
		r := ci.item.Bbox()
		if r.Overlaps(c.flushrect) {
			ci.item.Draw(c.dst, c.flushrect)
		}
	}
	if c.flushFunc != nil {
		c.flushFunc(c.flushrect)
	}
	c.flushrect = draw.ZR
}

// DeleteItem deletes a single item from the canvas.
//
func (c *Canvas) DeleteItem(item Item) {
	c.lock.Lock()
	var next *list.Element
	for e := c.items.Front(); e != nil; e = next {
		next = e.Next()
		ci := e.Value.(canvasItem)
		if ci.item == item {
			c.items.Remove(e)
			c.addFlush(item.Bbox())
		}
	}
	c.lock.Unlock()
}

// Delete deletes all the items associated with obj from the canvas.
//
func (c *Canvas) Delete(obj Object) {
	c.lock.Lock()
	var next *list.Element
	for e := c.items.Front(); e != nil; e = next {
		next = e.Next()
		ci := e.Value.(canvasItem)
		if ci.object == obj {
			c.items.Remove(e)
			c.addFlush(ci.item.Bbox())
		}
	}
	c.lock.Unlock()
}


// Atomically calls f while the canvas's lock is held,
// allowing objects to adjust their appearance without
// risk of drawing anomalies. Flush can be called
// to flush dirty areas of the canvas.
// An object should not change its appearance outside
// of this call.
//
func (c *Canvas) Atomically(f func(flush func(r draw.Rectangle))) {
	// could pre-allocate inside c if we cared.
	flush := func(r draw.Rectangle) {
		c.addFlush(r)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	f(flush)
}

func (c *Canvas) AddItem(item Item, obj Object) {
	c.lock.Lock()
	c.items.PushBack(canvasItem{item, obj})
	r := item.Bbox()
	if c.flushrect.Empty() {
		item.Draw(c.dst, r)
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
