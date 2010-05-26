package canvas
import (
	"exp/draw"
	"image"
	"sync"
)

type Background struct {
	lock sync.Mutex
	r draw.Rectangle	// overall rectangle (always origin 0, 0)
	img *image.RGBA
	bg image.Image
	item Drawer
	imgflush func(r draw.Rectangle)

	flushrect draw.Rectangle
	waste int
}

func NewBackground(img *image.RGBA, bg image.Image, flush func(r draw.Rectangle)) *Background {
	return &Background{
		img: img,
		bg: bg,
		r: draw.Rect(0, 0, img.Width(), img.Height()),
		imgflush: flush,
	}
}

func (b *Background) SetItem(item Drawer) {
	b.lock.Lock()
	b.item = item
	b.flushrect = b.r
	b.waste = 0
	b.lock.Unlock()
}

func (b *Background) Width() int {
	return b.img.Width()
}

func (b *Background) Height() int {
	return b.img.Height()
}

func (b *Background) Atomically(f func(FlushFunc)) {
	// could pre-allocate inside b if we cared.
	flush := func(r draw.Rectangle, drawn bool, draw Drawer){
		if draw != b.item {
			panic("flushed object not directly inside Background")
		}
		b.addFlush(r, drawn)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	f(FlushFunc(flush))
}

// stolen from inferno's devdraw
func (b *Background) addFlush(r draw.Rectangle, drawn bool) {
	r = r.Clip(b.r)
	if b.flushrect.Empty() {
		if !drawn {
			b.flushrect = r
			b.waste = 0
		}
		return
	}

	// if the new segment doesn't overlap with the
	// old segment and it has already been drawn,
	// do nothing.
	overlaps := b.flushrect.Overlaps(r)
	if !overlaps && drawn {
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
	draw.DrawMask(b.img, b.flushrect, b.bg, b.flushrect.Min, nil, draw.ZP, draw.Src)
	b.item.Draw(b.img, b.flushrect)
	if b.imgflush != nil {
		b.imgflush(b.flushrect)
	}
	b.flushrect = draw.ZR
}

func (b *Background) Flush() {
	b.lock.Lock()
	b.flush()
	b.lock.Unlock()
}
