// The canvas package provides some a facility
// for managing independently updating objects
// inside a graphics window.
//
// The principal type is Canvas, which displays a
// z-ordered set of objects. New objects may be added,
// deleted and change their appearance: the Canvas 
// manages any necessary re-drawing.
//
// Any object that implements the Item interface
// may be placed onto a Canvas, including a Canvas
// itself, allowing more complex objects to be built
// relatively easily.
//
package canvas

import (
	"code.google.com/p/x-go-binding/ui"
	"container/list"
	"image"
	"image/color"
	"image/draw"
	"log"
)

// The Flush method is used to flush any pending changes
// to the underlying image, usually the screen.
//
type Flusher interface {
	Flush()
}

// A Backer represents a graphical area containing
// a number of Drawer objects.
// To change its appearance, one of those objects
// may call Atomically, passing it a function which
// will be called once to make the changes.
// The function will be passed a FlushFunc that
// can be used to inform the Backer of any changes
// that are made.
// Rect should return the rectangle that is available
// for items within the Backing to draw into.
//
type Backing interface {
	Flusher
	Atomically(func(f FlushFunc))
	Rect() image.Rectangle
}

// A FlushFunc can be used to inform a Backing object
// of a changed area of pixels. r specifies the rectangle that has changed;
// if drawn is non-nil, it indicates that the rectangle has already
// been redrawn (only appropriate if all pixels in the
// rectangle have been non-transparently overwritten);
// its value gives the item that has changed,
// which must be a direct child of the Backing object.
//
type FlushFunc func(r image.Rectangle, drawn Drawer)

// The Drawer interface represents the basic level
// of functionality for a drawn object.
// SetContainer is called when the object is placed
// inside another; b will be nil if the object is removed
// from its container. SetContainer can be called twice
// in a row with the same object if some ancestor has
// changed.
//
// The Draw method should draw a representation of
// the object onto dst. No pixels outside clipr should
// be changed. It is legitimate for the Drawer object
// to retain the dst image until SetContainer is
// called (for instance, to perform its own direct
// manipulation of the image).
//
// Neither method in Drawer should interact with any object
// outside its direct control (for example by modifying
// the appearance of another object using the same Backer)
//
type Drawer interface {
	Draw(dst draw.Image, clipr image.Rectangle)
	SetContainer(b Backing)
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
	Bbox() image.Rectangle
	HitTest(p image.Point) bool
	Opaque() bool
}

// HandleMouse can be implemented by any object
// that might wish to handle mouse events. It is
// called with an initial mouse event, and a channel
// from which further events can be read.
// It returns true if the initial mouse event was absorbed.
// mc should not be used after HandleMouse returns.
//
type HandleMouser interface {
	HandleMouse(f Flusher, m ui.MouseEvent, ec <-chan interface{}) bool
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
//
type Canvas struct {
	r          image.Rectangle // the bounding rectangle of the canvas.
	img        draw.Image      // image we were last drawn onto
	backing    Backing
	opaque     bool
	background image.Image
	items      list.List // foreground objects are at the end of the list
}

// NewCanvas returns a new Canvas object that is inside
// backing. The background image, if non-nil, must
// be opaque, and will used to draw the background.
// r gives the extent of the canvas.
//
func NewCanvas(background color.Color, r image.Rectangle) *Canvas {
	// TODO: if background==nil, then
	// r is irrelevant, and we can just calculate our bbox
	// from the items we hold
	c := new(Canvas)
	if background != nil {
		c.opaque = opaqueColor(background)
		c.background = &image.Uniform{background}
	}
	c.backing = NullBacking()
	c.r = r
	return c
}

func (c *Canvas) Opaque() bool {
	// could do better by seeing if any opaque sub-item
	// covers the whole area.
	return c.opaque
}

// Rect returns the rectangle that is available
// for items to draw into.
//
func (c *Canvas) Rect() image.Rectangle {
	return c.r
}

func (c *Canvas) SetContainer(b Backing) {
	c.img = nil
	c.backing = b
	for e := c.items.Front(); e != nil; e = e.Next() {
		e.Value.(Item).SetContainer(c)
	}
}

func (c *Canvas) Bbox() image.Rectangle {
	return c.r
}

func (c *Canvas) Flush() {
	if c != nil && c.backing != nil {
		c.backing.Flush()
	}
}

// HandleMouse delivers the mouse events to the top-most
// item that that is hit by the mouse point.
//
func (c *Canvas) HandleMouse(_ Flusher, m ui.MouseEvent, ec <-chan interface{}) bool {
	var chosen HandlerItem
	c.Atomically(func(_ FlushFunc) {
		for e := c.items.Back(); e != nil; e = e.Prev() {
			if h, ok := e.Value.(HandlerItem); ok {
				if h.HitTest(m.Loc) {
					chosen = h
					break
				}
			}
		}
	})
	if chosen != nil {
		return chosen.HandleMouse(c, m, ec)
	}
	return false
}

func (c *Canvas) HitTest(p image.Point) (hit bool) {
	for e := c.items.Back(); e != nil; e = e.Prev() {
		if e.Value.(Item).HitTest(p) {
			return true
		}
	}
	return false
}

func (c *Canvas) Draw(dst draw.Image, clipr image.Rectangle) {
	clipr = clipr.Intersect(c.r)
	c.img = dst
	if c.background != nil {
		draw.Draw(dst, clipr, c.background, clipr.Min, draw.Over)
	}
	clipr = clipr.Intersect(c.r)
	for e := c.items.Front(); e != nil; e = e.Next() {
		it := e.Value.(Item)
		if it.Bbox().Overlaps(clipr) {
			it.Draw(dst, clipr)
		}
	}
}

// Raise moves the Item it adjacent to nextto in the canvas z-ordering.
// If above is true, the item will be placed just above nextto,
// otherwise it will be placed just below.
// If nextto is nil, then the item will be placed at
// the very top (above==true) or the very bottom (above==false).
//
func (c *Canvas) Raise(it, nextto Item, above bool) {
	if it == nextto {
		return
	}
	c.Atomically(func(flush FlushFunc) {
		var ie, ae *list.Element
		for e := c.items.Front(); e != nil; e = e.Next() {
			switch e.Value.(Item) {
			case it:
				ie = e
			case nextto:
				ae = e
			}
		}
		if ie != nil && (nextto == nil || ae != nil) {
			c.items.Remove(ie)
			if ae != nil {
				if above {
					c.items.InsertAfter(it, ae)
				} else {
					c.items.InsertBefore(it, ae)
				}
			} else {
				if above {
					c.items.PushBack(it)
				} else {
					c.items.PushFront(it)
				}
			}
			flush(it.Bbox(), nil)
		}
	})
}

// drawAbove draws only those items above it.
//
func (c *Canvas) drawAbove(it Item, clipr image.Rectangle) {
	clipr = clipr.Intersect(c.r)
	drawing := false
	for e := c.items.Front(); e != nil; e = e.Next() {
		if e.Value == nil {
			panic("nil value - can't happen?")
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
		if removed {
			it.SetContainer(NullBacking())
		} else {
			log.Printf("item %T not removed", it)
		}
	})
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
		panic("nil c or backing")
	}
	c.backing.Atomically(func(bflush FlushFunc) {
		f(func(r image.Rectangle, drawn Drawer) {
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
	c.Atomically(func(flush FlushFunc) {
		item.SetContainer(c)
		c.items.PushBack(item)
		r := item.Bbox()
		if item.Opaque() && c.img != nil {
			item.Draw(c.img, r.Intersect(c.r))
			flush(r, item)
		} else {
			flush(r, nil)
		}
	})
}

func debugp(f string, a ...interface{}) {
	log.Printf(f, a...)
}
