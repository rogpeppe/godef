// Package canvas layers a set of movable objects onto
// a draw.Image. Objects in the canvas may be created,
// moved and deleted; the Canvas manages any necessary
// re-drawing.
package canvas

import (
	"container/list"
	"rog-go.googlecode.com/hg/draw"
	"image"
	"log"
)

type Flusher interface {
	Flush()
}

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
	Flusher
	Atomically(func(f FlushFunc))
	Width() int
	Height() int
}

// A FlushFunc can be used to inform a Backing object
// of a changed area of pixels. r specifies the rectangle that has changed;
// if drawn is non-nil, it indicates that the rectangle has already
// been redrawn (only appropriate if all pixels in the
// rectangle have been non-transparently overwritten);
// its value gives the item that has changed,
// which must be a direct child of the Backing object.
//
type FlushFunc func(r draw.Rectangle, drawn Drawer)

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
	SetContainer(Backing)
}

// HandleMouse can be implemented by any object
// that might wish to handle mouse events. It is
// called with an initial mouse event, and a channel
// from which further mouse events can be read.
// It returns true if the initial mouse event was absorbed.
// mc should not be used after HandleMouse returns.
//
type HandleMouser interface {
	HandleMouse(f Flusher, m draw.Mouse, mc <-chan draw.Mouse) bool
}

type HandlerItem interface {
	Item
	HandleMouser
}

// static interface checks:
var _ Backing = (*Canvas)(nil)
var _ HandlerItem = (*Canvas)(nil)

// A Canvas represents a z-ordered set of drawable Items.
// As a Canvas itself implements Item and Backing, Canvas's can
// be nested indefinitely.
type Canvas struct {
	r          draw.Rectangle // the bounding rectangle of the canvas.
	img        *image.RGBA    // image we were last drawn onto
	backing    Backing
	opaque     bool
	background image.Image
	items      list.List // foreground objects are at the end of the list
}

// NewCanvas returns a new Canvas object that is inside
// backing. The background image, if non-nil, must
// be opaque, and will used to draw the background.
// r gives the extent of the canvas.
// TODO: if background==nil and backing==nil, then
// r is irrelevant, and we can just calculate our bbox
// from the items we hold
//
func NewCanvas(backing Backing, background image.Color, r draw.Rectangle) *Canvas {
	c := new(Canvas)
	c.backing = backing
	if background != nil {
		c.opaque = opaqueColor(background)
		c.background = image.ColorImage{background}
	}
	c.r = r
	return c
}

func (c *Canvas) Opaque() bool {
	// could do better by seeing if any opaque sub-item
	// covers the whole area.
	return c.opaque
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

func (c *Canvas) SetContainer(b Backing) {
	// XXX
}

func (c *Canvas) Bbox() draw.Rectangle {
	return c.r
}

func (c *Canvas) Flush() {
	if c.backing != nil {
		c.backing.Flush()
	}
}

// HandleMouse delivers the mouse events to the top-most
// item that that is hit by the mouse point.
//
func (c *Canvas) HandleMouse(_ Flusher, m draw.Mouse, mc <-chan draw.Mouse) bool {
	var chosen HandlerItem
	c.Atomically(func(_ FlushFunc) {
		for e := c.items.Back(); e != nil; e = e.Prev() {
			if h, ok := e.Value.(HandlerItem); ok {
				if h.HitTest(m.Point) {
					chosen = h
					break
				}
			}
		}
	})
	if chosen != nil {
		return chosen.HandleMouse(c, m, mc)
	}
	return false
}

func (c *Canvas) HitTest(p draw.Point) (hit bool) {
	c.Atomically(func(_ FlushFunc) {
		for e := c.items.Back(); e != nil; e = e.Prev() {
			if e.Value.(Item).HitTest(p) {
				hit = true
				break
			}
		}
	})
	return
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
		if e.Value == nil {
			continue
		}
		item := e.Value.(Item)
		if drawing && item.Bbox().Overlaps(clipr) {
			item.Draw(c.img, clipr)
		} else if item == it {
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
	c.Atomically(func(flush FlushFunc) {
		var next *list.Element
		for e := c.items.Front(); e != nil; e = next {
			next = e.Next()
			if e.Value.(Item) == it {
				c.items.Remove(e)
				flush(it.Bbox(), nil)
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
	c.Atomically(func(flush FlushFunc) {
		var next *list.Element
		for e := c.items.Front(); e != nil; e = next {
			next = e.Next()
			if e.Value.(Item) == it {
				r := it.Bbox()
				e.Value = it1
				flush(r, nil)
				flush(it1.Bbox(), nil)
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
	if c == nil || c.backing == nil {
		// if an object isn't inside a canvas, then
		// just perform the action anyway,
		// as atomicity doesn't matter then.
		f(func(_ draw.Rectangle, _ Drawer) {})
		return
	}
	c.backing.Atomically(func(bflush FlushFunc) {
		f(func(r draw.Rectangle, drawn Drawer) {
			if drawn != nil {
				c.drawAbove(drawn.(Item), r)
				drawn = c
			} else if c.img != nil && c.opaque {
				// if we're opaque, then we can just redraw ourselves
				// without worrying about what might be underneath.
				c.Draw(c.img, r)
				drawn = c
			}
			bflush(r, drawn)
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
			flush(r, item)
		} else {
			flush(r, nil)
		}
	})
}

func debugp(f string, a ...interface{}) {
	log.Stdoutf(f, a)
}

type sizer interface {
	Width() int
	Height() int
}

func size(x sizer) draw.Point {
	return draw.Point{x.Width(), x.Height()}
}
