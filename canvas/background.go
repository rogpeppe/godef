package canvas

import (
	"rog-go.googlecode.com/hg/draw"
	"image"
	"sync"
)

// A Background is the base layer on which other
// objects can be layered. It implements the Backing
// interface and displays a single object only.
type Background struct {
	lock     sync.Mutex
	r        draw.Rectangle // overall rectangle (always origin 0, 0)
	img      draw.Image
	bg       image.Image
	item     Drawer
	imgflush func(r draw.Rectangle)

	flushrect draw.Rectangle
	waste     int
}

// NewBackground creates a new Background object that
// draws to img, and draws the actual background with bg.
// The flush function, if non-nil, will be called to
// whenever changes are to be made visible externally
// (for example when Flush() is called.
//
// Note that bg is drawn with the draw.Src operation,
// so it is possible to create images with a transparent
// background.
//
func NewBackground(img draw.Image, bg image.Image, flush func(r draw.Rectangle)) *Background {
	r := draw.Rect(0, 0, img.Width(), img.Height())
	return &Background{
		img:      img,
		bg:       bg,
		r:        r,
		flushrect: r,
		imgflush: flush,
	}
}

// SetItem sets the item to draw on top of the background.
//
func (b *Background) SetItem(item Drawer) {
	b.lock.Lock()
	b.item = item
	b.flushrect = b.r
	b.waste = 0
	if item != nil {
		b.item.SetContainer(b)
	}
	b.lock.Unlock()
}

func (b *Background) Rect() draw.Rectangle {
	return draw.Rect(0, 0, b.img.Width(), b.img.Height())
}

func (b *Background) Atomically(f func(FlushFunc)) {
	// could pre-allocate inside b if we cared.
	flush := func(r draw.Rectangle, drawn Drawer) {
		if drawn != nil && drawn != b.item {
			panic("flushed object not directly inside Background")
		}
		if !r.Canon().Eq(r) {
			debugp("non canonical flushrect %v", r)
			panic("oops background")
		}
		b.addFlush(r, drawn != nil)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	f(flush)
}

// stolen from inferno's devdraw
func (b *Background) addFlush(r draw.Rectangle, drawn bool) {
	r = r.Clip(b.r)
	if b.flushrect.Empty() {
		if drawn {
			if b.imgflush != nil {
				b.imgflush(r)
			}
		} else {
			b.flushrect = r
			b.waste = 0
		}
		return
	}

	// if the new segment doesn't overlap with the
	// old segment and it has already been drawn,
	// do nothing except possible call the external flush.
	overlaps := b.flushrect.Overlaps(r)
	if !overlaps && drawn {
		if b.imgflush != nil {
			b.imgflush(r)
		}
		return
	}
	nbb := b.flushrect.Combine(r)
	ar := r.Dx() * r.Dy()
	abb := b.flushrect.Dx() * b.flushrect.Dy()
	anbb := nbb.Dx() * nbb.Dy()

	// Area of new waste is area of new bb minus area of old bb,
	// less the area of the new segment, which we assume is not waste.
	// This could be negative, but that's OK.
	b.waste += anbb - abb - ar
	if b.waste < 0 {
		b.waste = 0
	}

	//absorb if:
	//	total area is small
	//	waste is less than half total area
	// 	rectangles touch
	if anbb <= 1024 || b.waste*2 < anbb || b.flushrect.Overlaps(r) {
		b.flushrect = nbb
		return
	}
	//  emit current state
	if !b.flushrect.Empty() {
		b.flush()
	}
	b.flushrect = r
}

func (b *Background) flush() {
	if !b.flushrect.Empty() {
		draw.DrawMask(b.img, b.flushrect, b.bg, b.flushrect.Min, nil, draw.ZP, draw.Src)
		b.item.Draw(b.img, b.flushrect)
		if b.imgflush != nil {
			b.imgflush(b.flushrect)
		}
		b.flushrect = draw.ZR
	}
}

// Flush flushes all pending changes, and makes them visible.
//
func (b *Background) Flush() {
	b.lock.Lock()
	b.flush()
	b.lock.Unlock()
}

type nullBacking bool

// NullBacking returns an object that satisfies the
// Backing interface but has no actual image associated
// with it.
//
func NullBacking() Backing {
	return nullBacking(false)
}

var globalLock sync.Mutex

func (_ nullBacking) Flush() { }

func (_ nullBacking) Atomically(f func(f FlushFunc)) {
	globalLock.Lock()
	defer globalLock.Unlock()
	f(func(_ draw.Rectangle, _ Drawer) {})
}

func (b nullBacking) Rect() draw.Rectangle {
	return draw.ZR
}
