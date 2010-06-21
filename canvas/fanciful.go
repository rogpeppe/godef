package canvas

import (
	"exp/draw"
	"image"
)

// A MoveableItem is an item that may be
// moved by calling SetCentre, where the
// centre is the central point of the item's
// bounding box.
//
type MoveableItem interface {
	Item
	SetCentre(p draw.Point)
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

func (d *dragger) HandleMouse(f Flusher, m draw.Mouse, mc <-chan draw.Mouse) bool {
	if m.Buttons&1 == 0 {
		if h, ok := d.MoveableItem.(HandleMouser); ok {
			return h.HandleMouse(f, m, mc)
		}
		return false
	}
	delta := centre(d.Bbox()).Sub(m.Point)
	but := m.Buttons
	for {
		m = <-mc
		d.SetCentre(m.Add(delta))
		f.Flush()
		if (m.Buttons & but) != but {
			break
		}
	}
	return true
}

type mover struct {
	item Item
	backing Backing
	delta draw.Point		// vector from backing to item coords
}

func Moveable(item Item) MoveableItem {
	m := &mover{item, NullBacking(), draw.ZP}
	item.SetContainer(m)
	return m
}

// func (mov *mover) HandleMouse(m draw.Mouse, mc <-chan draw.Mouse) bool
// we'd like to be able to pass the mouse events on to our item
// here, but we can't do so without possibly consuming one
// mouse event too many, as we'd have to impose an intermediate
// process to do coord translation, which adds a buffer size of one.

func (m *mover) SetCentre(p draw.Point) {
	m.backing.Atomically(func(flush FlushFunc){
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
	m.backing.Atomically(func(flush FlushFunc){
		f(func(r draw.Rectangle, it Drawer){
			if it != nil {
				it = m
			}
			flush(r.Sub(m.delta), it)
		})
	})
}

// TODO: it's entirely possible for m.delta to be more
// negative the backing width, but that width be 
func (m *mover) Rect() draw.Rectangle {
	return m.backing.Rect().Add(m.delta)
}

func (m *mover) Flush() {
	m.backing.Flush()
}

func (m *mover) Draw(img draw.Image, clipr draw.Rectangle) {
//debugp("mover draw clipr %v; centre %v; delta %v\n", clipr, centre(m.item.Bbox()), m.delta)
	clipr = clipr.Add(m.delta)
	i := SliceImage(clipr.Max.X, clipr.Max.Y, clipr, img, m.delta)
//debugp("item draw clipr %v; delta %v; bbox %v\n", clipr, m.delta, m.item.Bbox())
	m.item.Draw(i, clipr)
//debugp("item drawn\n")
}

func (m *mover) Bbox() draw.Rectangle {
	return m.item.Bbox().Sub(m.delta)
}

func (m *mover) HitTest(p draw.Point) bool {
	return m.item.HitTest(p.Add(m.delta))
}

func (m *mover) Opaque() bool {
	return m.item.Opaque()
}

type rectBacking struct {
	Backing
	r draw.Rectangle
}
func (b rectBacking) Rect() draw.Rectangle {
	return b.r
}

func ImageOf(it Item) *image.RGBA {
	r := it.Bbox()
	img := image.NewRGBA(r.Dx(), r.Dy())
	b := Backing(rectBacking{NullBacking(), draw.Rect(0, 0, r.Dx(), r.Dy())})
	b.Atomically(func(_ FlushFunc){
		it.SetContainer(b)
		it.Draw(SliceImage(r.Max.X, r.Max.Y, r, img, r.Min), r)
		it.SetContainer(nil)
	})
	return img
}

type imageSlice struct {
	img draw.Image
	r draw.Rectangle		// rect actually backed with an image.
	p draw.Point			// origin of img.
}

var _ draw.Image = (*imageSlice)(nil)

// SliceImage returns an image which is a view onto a portion of img.
// The returned image has the specified width and height,
// but all draw operations are clipped to r.
// The origin of img is aligned with p. Where img
// overlaps with r, it will be used for drawing operations.
//
func SliceImage(width, height int, r draw.Rectangle, img draw.Image, p draw.Point) draw.Image {
	// TODO: detect when img is itself a SliceImage and
	// use the underlying image directly.
	i := new(imageSlice)
	i.img = img
	i.r = r.Clip(draw.Rectangle{p, p.Add(draw.Pt(img.Width(), img.Height()))})
//debugp("actual sliced rectangle %v\n", i.r)
	i.p = p
	return i
}

func (i *imageSlice) DrawMask(r draw.Rectangle, src image.Image, sp draw.Point, mask image.Image, mp draw.Point, op draw.Op) bool {
//debugp("imageslice draw %v; sp %v\n", r, sp)
	dr := r.Clip(i.r)
	if dr.Empty() {
//debugp("-> clipped empty (r %v)\n", i.r)
		return true
	}
	delta := dr.Min.Sub(r.Min)			// realignment because of clipping.
	sp = sp.Add(delta)
	mp = mp.Add(delta)
	dr = dr.Sub(i.p)
//debugp("-> draw %v; sp %v\n", dr, sp)
	draw.DrawMask(i.img, dr, src, sp, mask, mp, op)
	return true
}

func (i *imageSlice) ColorModel() image.ColorModel {
	return i.img.ColorModel()
}

func (i *imageSlice) Width() int {
	return i.r.Max.X
}

func (i *imageSlice) Height() int {
	return i.r.Max.Y
}

func (i *imageSlice) At(x, y int) image.Color {
	p := draw.Point{x, y}
	if p.In(i.r) {
		p = p.Add(i.p)
		return i.img.At(p.X, p.Y)
	}
	return image.RGBAColor{0, 0, 0, 0}
}

func (i *imageSlice) Set(x, y int, c image.Color) {
	p := draw.Point{x, y}
	if p.In(i.r) {
		p = p.Add(i.p)
		i.img.Set(p.X, p.Y, c)
	}
}
