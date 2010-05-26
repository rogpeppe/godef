// Package canvas layers a set of movable objects onto
// a draw.Image. Objects in the canvas may be created,
// moved and deleted; the Canvas manages any necessary
// re-drawing.
package canvas

import (
	"container/list"
	"exp/draw"
	"image"
)

// A Backer represents a graphical area containing
// a number of Drawer objects.
// To change its appearance, one of those objects
// may call Atomically, passing it a function which
// will be called (once) to make the changes.
// The function will be passed a FlushFunc that
// can be used to inform the Backer of any changes
// that are made.
//
type Backing interface {
	Atomically(func(f FlushFunc))
	Flush()
	Width() int
	Height() int
}

// A FlushFunc can be used to inform a Backing object
// of a changed area of pixels. r specifies the rectangle that has changed;
// drawn indicates whether the rectangle has already been redrawn
// (only appropriate if the all pixels in the rectangle have
// been non-transparently overwritten); and
// draw gives the item that has changed, which must
// be directly inside the Backer.
//
type FlushFunc func(r draw.Rectangle, drawn bool, draw Drawer)

// The Draw method should draw a representation of
// the object onto dst. No pixels outside clipr should
// be changed. It should not interact with any object
// outside its direct control (for example by modifying
// the appearance of another object using the same Backer)
//
type Drawer interface {
	Draw(dst *image.RGBA, clipr draw.Rectangle)
}

// Values that implement the Item interface may be added
// to the canvas. They should adhere to the following rules:
// - No calls to the canvas should be made while in the Draw, Bbox
// or HitTest methods.
// - All changes to the appearance of the object should be made using
// the Atomically function.
//
type Item interface {
	Drawer
	Bbox() draw.Rectangle
	HitTest(p draw.Point) bool
	Opaque() bool
	SetContainer(c *Canvas)
}

// Each Item may be associated with an handler Object.
// More than one Item may be associated with the same
// Object. If the canvas's HandleMouse method is
// called, and HitTest succeeds on an item, then
// HandleMouse will be invoked on its associated Object.
//
type MouseHandler interface {
	HandleMouse(item Item, m draw.Mouse, mc <-chan draw.Mouse) bool
}

// A Canvas represents a z-ordered set of drawable Items.
// As a Canvas itself implements Item and Backing, Canvas's can
// be nested indefinitely.
type Canvas struct {
	r draw.Rectangle		// the bounding rectangle of the canvas.
	img *image.RGBA		// image we were last drawn onto
	backing Backing
	opaque bool
	background image.Image
	items    list.List 	// foreground objects are at the end of the list
}

// NewCanvas returns a new Canvas object that is inside
// backing. The background image, if non-nil, must
// be opaque, and will used to draw the background.
// r gives the extent of the canvas.
//
func NewCanvas(backing Backing, background image.Color, r draw.Rectangle) *Canvas {
	c := new(Canvas)
	c.backing = backing
	if background != nil {
		_, _, _, a := background.RGBA()
		c.opaque = a == 0xffffffff
		c.background = image.ColorImage{background}
	}
	c.r = r
	return c
}

// Width returns the width of the canvas, which is
// the width of its underlying image.
//
func (c *Canvas) Width() int {
	return c.r.Dx()
}

// Width returns the height of the canvas, which is
// the height of its underlying image.
//
func (c *Canvas) Height() int {
	return c.r.Dy()
}

func (c *Canvas) Bbox() draw.Rectangle {
	return c.r
}

func (c *Canvas) Flush() {
	c.backing.Flush()
}

func (c *Canvas) Draw(dst *image.RGBA, clipr draw.Rectangle) {
	clipr = clipr.Clip(c.r)
	c.img = dst
	if c.background != nil {
		draw.DrawMask(dst, clipr, c.background, clipr.Min, nil, draw.ZP, draw.Src)
	}
	clipr = clipr.Clip(c.r)
	for e := c.items.Front(); e != nil; e = e.Next() {
		it := e.Value.(Item)
		if it.Bbox().Overlaps(clipr) {
			it.Draw(dst, clipr)
		}
	}
}

// drawAbove draws only those items above it.
//
func (c *Canvas) drawAbove(it Item, clipr draw.Rectangle) {
	clipr = clipr.Clip(c.r)
	drawing := false
	for e := c.items.Front(); e != nil; e = e.Next() {
		item := e.Value.(Item)
		if drawing && item.Bbox().Overlaps(clipr) {
			item.Draw(c.img, clipr)
		}else if item == it {
			drawing = true
		}
	}
}

// DeleteItem deletes a single item from the canvas.
//
func (c *Canvas) Delete(it Item) {
	if c == nil {
		return
	}
	removed := false
	c.Atomically(func (flush FlushFunc) {
		var next *list.Element
		for e := c.items.Front(); e != nil; e = next {
			next = e.Next()
			if e.Value.(Item) == it {
				c.items.Remove(e)
				flush(it.Bbox(), false, it)
				removed = true
				break
			}
		}
	})
	if removed {
		it.SetContainer(nil)
	}
}

func (c *Canvas) Replace(it, it1 Item) (replaced bool) {
	c.Atomically(func (flush FlushFunc) {
		var next *list.Element
		for e := c.items.Front(); e != nil; e = next {
			next = e.Next()
			if e.Value.(Item) == it {
				r := it.Bbox()
				e.Value = it1
				flush(r, false, it)
				flush(it1.Bbox(), false, it1)
				replaced = true
				break
			}
		}
	})
	return
}


// Atomically calls f, which can then make changes to
// the appearance of items in the canvas.
// See the Backing interface for details
//
func (c *Canvas) Atomically(f func(FlushFunc)) {
	if c == nil {
		// if an object isn't inside a canvas, then
		// just perform the action anyway,
		// as atomicity doesn't matter then.
		f(func(r draw.Rectangle, drawn bool, it Drawer) { })
		return
	}
	c.backing.Atomically(func(bflush FlushFunc) {
		f(func(r draw.Rectangle, drawn bool, it Drawer) {
			if drawn {
				c.drawAbove(it.(Item), r)
			}else if c.img != nil && c.opaque {
				// if we're opaque, then we can just redraw ourselves
				// without worrying about what might be underneath.
				c.Draw(c.img, r)
				drawn = true
			}
			bflush(r, drawn, c)
		})
	})
}

func (c *Canvas) AddItem(item Item) {
	item.SetContainer(c)
	c.Atomically(func(flush FlushFunc) {
		c.items.PushBack(item)
		r := item.Bbox()
		if item.Opaque() && c.img != nil {
			item.Draw(c.img, r)
			flush(r, true, item)
		}else{
			flush(r, false, item)
		}
	})
}

type sizer interface {
	Width() int
	Height() int
}

func size(x sizer) draw.Point {
	return draw.Point{x.Width(), x.Height()}
}
