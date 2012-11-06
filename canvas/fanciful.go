package canvas

import (
	"code.google.com/p/x-go-binding/ui"
	"image"
	"image/color"
	"image/draw"
)

// A MoveableItem is an item that may be
// moved by calling SetCentre, where the
// centre is the central point of the item's
// bounding box.
//
type MoveableItem interface {
	Item
	SetCentre(p image.Point)
}

type dragger struct {
	MoveableItem
}

// Draggable makes any MoveableItem into
// an object that may be dragged by the
// mouse.
//
func Draggable(it MoveableItem) MoveableItem {
	return &dragger{it}
}

var _ HandlerItem = &dragger{}

func (d *dragger) HandleMouse(f Flusher, m ui.MouseEvent, ec <-chan interface{}) bool {
	if m.Buttons&1 == 0 {
		if h, ok := d.MoveableItem.(HandleMouser); ok {
			return h.HandleMouse(f, m, ec)
		}
		return false
	}
	delta := centre(d.Bbox()).Sub(m.Loc)
	but := m.Buttons
	for {
		if m, ok := (<-ec).(ui.MouseEvent); ok {
			d.SetCentre(m.Loc.Add(delta))
			f.Flush()
			if (m.Buttons & but) != but {
				break
			}
		}
	}
	return true
}

type mover struct {
	item    Item
	backing Backing
	delta   image.Point // vector from backing to item coords
}

func Moveable(item Item) MoveableItem {
	m := &mover{item, NullBacking(), image.ZP}
	item.SetContainer(m)
	return m
}

// func (mov *mover) HandleMouse(m ui.MouseEvent, ec <-chan interface{}) bool
// we'd like to be able to pass the mouse events on to our item
// here, but we can't do so without possibly consuming one
// mouse event too many, as we'd have to impose an intermediate
// process to do coord translation, which adds a buffer size of one.

func (m *mover) SetCentre(p image.Point) {
	m.backing.Atomically(func(flush FlushFunc) {
		bbox := m.item.Bbox()
		oldr := bbox.Sub(m.delta)
		m.delta = centre(bbox).Sub(p)
		flush(oldr, nil)
		flush(bbox.Sub(m.delta), nil)
	})
	m.backing.Flush()
}

func (m *mover) SetContainer(b Backing) {
	m.backing = b
	m.item.SetContainer(m)
}

func (m *mover) Atomically(f func(FlushFunc)) {
	m.backing.Atomically(func(flush FlushFunc) {
		f(func(r image.Rectangle, it Drawer) {
			if it != nil {
				it = m
			}
			flush(r.Sub(m.delta), it)
		})
	})
}

// TODO: it's entirely possible for m.delta to be more
// negative the backing width, but that width be 
func (m *mover) Rect() image.Rectangle {
	return m.backing.Rect().Add(m.delta)
}

func (m *mover) Flush() {
	m.backing.Flush()
}

func (m *mover) Draw(img draw.Image, clipr image.Rectangle) {
	//debugp("mover draw clipr %v; centre %v; delta %v\n", clipr, centre(m.item.Bbox()), m.delta)
	clipr = clipr.Add(m.delta)
	i := SliceImage(clipr.Max.X, clipr.Max.Y, clipr, img, m.delta)
	//debugp("item draw clipr %v; delta %v; bbox %v\n", clipr, m.delta, m.item.Bbox())
	m.item.Draw(i, clipr)
	//debugp("item drawn\n")
}

func (m *mover) Bbox() image.Rectangle {
	return m.item.Bbox().Sub(m.delta)
}

func (m *mover) HitTest(p image.Point) bool {
	return m.item.HitTest(p.Add(m.delta))
}

func (m *mover) Opaque() bool {
	return m.item.Opaque()
}

type rectBacking struct {
	Backing
	r image.Rectangle
}

func (b rectBacking) Rect() image.Rectangle {
	return b.r
}

func ImageOf(it Item) *image.RGBA {
	r := it.Bbox()
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	b := Backing(rectBacking{NullBacking(), image.Rect(0, 0, r.Dx(), r.Dy())})
	b.Atomically(func(_ FlushFunc) {
		it.SetContainer(b)
		it.Draw(SliceImage(r.Max.X, r.Max.Y, r, img, r.Min), r)
		it.SetContainer(nil)
	})
	return img
}

type imageSlice struct {
	img draw.Image
	r   image.Rectangle // rect actually backed with an image.
	p   image.Point     // origin of img.
}

var _ draw.Image = (*imageSlice)(nil)

// SliceImage returns an image which is a view onto a portion of img.
// The returned image has the specified width and height,
// but all draw operations are clipped to r.
// The origin of img is aligned with p. Where img
// overlaps with r, it will be used for drawing operations.
//
func SliceImage(width, height int, r image.Rectangle, img draw.Image, p image.Point) draw.Image {
	// TODO: detect when img is itself a SliceImage and
	// use the underlying image directly.
	i := new(imageSlice)
	i.img = img
	i.r = r.Intersect(image.Rectangle{p, p.Add(img.Bounds().Size())})
	//debugp("actual sliced rectangle %v\n", i.r)
	i.p = p
	return i
}

func (i *imageSlice) DrawMask(r image.Rectangle, src image.Image, sp image.Point, mask image.Image, mp image.Point, op draw.Op) bool {
	//debugp("imageslice draw %v; sp %v\n", r, sp)
	dr := r.Intersect(i.r)
	if dr.Empty() {
		//debugp("-> clipped empty (r %v)\n", i.r)
		return true
	}
	delta := dr.Min.Sub(r.Min) // realignment because of clipping.
	sp = sp.Add(delta)
	mp = mp.Add(delta)
	dr = dr.Sub(i.p)
	//debugp("-> draw %v; sp %v\n", dr, sp)
	draw.DrawMask(i.img, dr, src, sp, mask, mp, op)
	return true
}

func (i *imageSlice) ColorModel() color.Model {
	return i.img.ColorModel()
}

func (i *imageSlice) Bounds() image.Rectangle {
	return i.r
}

func (i *imageSlice) At(x, y int) color.Color {
	p := image.Point{x, y}
	if p.In(i.r) {
		p = p.Add(i.p)
		return i.img.At(p.X, p.Y)
	}
	return color.RGBA{0, 0, 0, 0}
}

func (i *imageSlice) Set(x, y int, c color.Color) {
	p := image.Point{x, y}
	if p.In(i.r) {
		p = p.Add(i.p)
		i.img.Set(p.X, p.Y, c)
	}
}
